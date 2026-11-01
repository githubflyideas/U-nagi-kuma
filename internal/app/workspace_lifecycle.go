package app

import (
	"context"
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
}
type workspaceCompletion struct {
	job *workspaceJob
	err error
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
	defer workers.Wait()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	active := 0
	reconcile := func() {
		states, err := a.store.PublishedWorkspaceRuntimes()
		if err != nil {
			return
		}
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
				jobs[key] = &workspaceJob{state: s, due: map[string]time.Time{}}
			}
		}
		now := time.Now()
		var ready []*workspaceJob
		for key, j := range jobs {
			if j.cancel == nil && desired[key] == j.state && !now.Before(j.next) {
				ready = append(ready, j)
			}
		}
		sort.Slice(ready, func(i, j int) bool { return ready[i].next.Before(ready[j].next) })
		for _, j := range ready {
			if active >= 8 || ctx.Err() != nil {
				break
			}
			work, cancel := context.WithCancel(ctx)
			j.cancel = cancel
			active++
			workers.Add(1)
			go func(j *workspaceJob) {
				defer workers.Done()
				_, err := a.runWorkspaceChecks(work, j.state.TenantID, j.state.WorkspaceID, j.state.Generation, j.due)
				done <- workspaceCompletion{j, err}
			}(j)
		}
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
			if c.err != nil {
				if j.backoff == 0 {
					j.backoff = time.Second
				} else {
					j.backoff *= 2
					if j.backoff > 30*time.Second {
						j.backoff = 30 * time.Second
					}
				}
				j.next = now.Add(j.backoff)
			} else {
				j.backoff = 0
				j.next = now.Add(30 * time.Second)
				for _, at := range j.due {
					if at.Before(j.next) {
						j.next = at
					}
				}
			}
		case <-ticker.C:
			reconcile()
		}
	}
}
