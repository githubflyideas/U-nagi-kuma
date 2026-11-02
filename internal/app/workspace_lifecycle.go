package app

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

type workspaceScope struct {
	tenant    store.TenantID
	workspace store.WorkspaceID
}
type workspaceJob struct {
	state   store.WorkspaceConfigState
	due     map[string]time.Time // accessed only by the worker while in flight
	next    time.Time
	backoff time.Duration
	cancel  context.CancelFunc
	status  WorkspaceRuntimeStatus
}
type workspaceCompletion struct {
	job                *workspaceJob
	err                error
	notificationStatus string
}

// One supervisor, at most eight admitted workers and no tick goroutines/queued
// passes. Monotonic wall time drives cadence; persisted observation clocks do not.
// Polling observes durable publication, including Store-side revoke/disable.
// The Store fences active probes synchronously with those mutations; discovery
// latency therefore cannot authorize a stale probe or late result.
func (a *App) runWorkspaceSchedules(ctx context.Context) {
	jobs := map[workspaceScope]*workspaceJob{}
	done := make(chan workspaceCompletion, 8)
	var workers sync.WaitGroup
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	active := 0
	discovery := WorkspaceDiscoveryStatus{State: "starting", Capacity: 8}
	var discoveryBackoff time.Duration
	publish := func() {
		a.workspaceLifecycle.mu.Lock()
		defer a.workspaceLifecycle.mu.Unlock()
		a.workspaceLifecycle.scopes = make(map[workspaceScope]WorkspaceRuntimeStatus, len(jobs))
		for key, j := range jobs {
			a.workspaceLifecycle.scopes[key] = j.status
		}
		discovery.ActiveWorkers = active
		a.workspaceLifecycle.discovery = discovery
	}
	defer func() {
		for _, j := range jobs {
			if j.cancel != nil {
				j.cancel()
			}
		}
		workers.Wait()
		active = 0
		discovery.State = "stopped"
		discovery.NextAttempt = time.Time{}
		for _, j := range jobs {
			j.status.State = "stopped"
			j.status.NextAttempt = time.Time{}
		}
		publish()
	}()
	reconcile := func() {
		now := time.Now()
		if now.Before(discovery.NextAttempt) {
			return
		}
		discovery.LastAttempt = now
		states, err := a.store.PublishedWorkspaceRuntimes()
		if err != nil {
			discoveryBackoff = workspaceRetryDelay(discoveryBackoff)
			discovery.State = "error"
			discovery.Code = "discovery_failed"
			discovery.ConsecutiveErrors++
			discovery.NextAttempt = now.Add(discoveryBackoff)
			for _, j := range jobs {
				if j.cancel != nil {
					j.cancel()
				}
			}
			publish()
			return
		}
		discoveryBackoff = 0
		discovery.State = "ready"
		discovery.Code = ""
		discovery.ConsecutiveErrors = 0
		discovery.LastSuccess = now
		discovery.NextAttempt = now.Add(100 * time.Millisecond)
		desired := map[workspaceScope]store.WorkspaceConfigState{}
		for _, s := range states {
			desired[workspaceScope{s.TenantID, s.WorkspaceID}] = s
		}
		for key, j := range jobs {
			if s, ok := desired[key]; !ok || s != j.state {
				if j.cancel != nil {
					j.cancel()
				} else {
					delete(jobs, key)
				}
			}
		}
		for key, s := range desired {
			if jobs[key] == nil {
				jobs[key] = &workspaceJob{state: s, due: map[string]time.Time{}, status: WorkspaceRuntimeStatus{TenantID: s.TenantID, WorkspaceID: s.WorkspaceID, Revision: s.Revision, Generation: s.Generation, State: "pending", NotificationStatus: "not_started"}}
			}
		}
		var ready []*workspaceJob
		for key, j := range jobs {
			if j.cancel == nil && desired[key] == j.state && !now.Before(j.next) {
				j.status.State = "queued"
				ready = append(ready, j)
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			if ready[i].next.Equal(ready[j].next) {
				if ready[i].state.TenantID == ready[j].state.TenantID {
					return ready[i].state.WorkspaceID < ready[j].state.WorkspaceID
				}
				return ready[i].state.TenantID < ready[j].state.TenantID
			}
			return ready[i].next.Before(ready[j].next)
		})
		for _, j := range ready {
			if active >= 8 || ctx.Err() != nil {
				break
			}
			work, cancel := context.WithCancel(ctx)
			j.cancel = cancel
			active++
			j.status.State = "running"
			j.status.LastAttempt = now
			j.status.NextAttempt = time.Time{}
			workers.Add(1)
			go func(j *workspaceJob) {
				defer workers.Done()
				batch, err := a.runWorkspaceChecks(work, j.state.TenantID, j.state.WorkspaceID, j.state.Generation, j.due)
				done <- workspaceCompletion{j, err, batch.NotificationStatus}
			}(j)
		}
		publish()
	}
	reconcile()
	for {
		select {
		case <-ctx.Done():
			for _, j := range jobs {
				if j.cancel != nil {
					j.cancel()
				}
			}
			return
		case c := <-done:
			active--
			j := c.job
			j.cancel()
			j.cancel = nil
			now := time.Now()
			j.status.LastCompleted = now
			j.status.NotificationStatus = sanitizedWorkspaceNotificationStatus(c.notificationStatus)
			if c.err != nil {
				j.backoff = workspaceRetryDelay(j.backoff)
				j.status.ConsecutiveErrors++
				j.status.State = "error"
				j.status.Code = "execution_failed"
				switch {
				case errors.Is(c.err, ErrWorkspaceNotificationUnsupported):
					j.status.State = "blocked"
					j.status.Code = "unsupported_notifications"
				case errors.Is(c.err, ErrWorkspaceExecutionUnsupported):
					j.status.State = "blocked"
					j.status.Code = "unsupported_execution"
				case errors.Is(c.err, store.ErrTenantAccess):
					j.status.State = "blocked"
					j.status.Code = "authority_unavailable"
				case errors.Is(c.err, context.Canceled):
					j.status.State = "canceled"
					j.status.Code = "execution_canceled"
				}
				j.next = now.Add(j.backoff)
			} else {
				j.status.ConsecutiveErrors = 0
				j.status.State = "scheduled"
				j.status.Code = ""
				j.backoff = 0
				j.next = now.Add(30 * time.Second)
				for _, at := range j.due {
					if at.Before(j.next) {
						j.next = at
					}
				}
			}
			j.status.NextAttempt = j.next
			publish()
		case <-ticker.C:
			reconcile()
		}
	}
}
