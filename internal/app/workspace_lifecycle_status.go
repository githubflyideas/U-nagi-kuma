package app

import (
	"github.com/githubflyideas/unagikuma/internal/store"
	"sync"
	"time"
)

// WorkspaceRuntimeStatus is a secret-free snapshot, not proof of target health.
// State and Code are fixed machine-readable values, never raw error strings.
type WorkspaceRuntimeStatus struct {
	TenantID           store.TenantID    `json:"tenant_id"`
	WorkspaceID        store.WorkspaceID `json:"workspace_id"`
	Revision           int64             `json:"revision"`
	Generation         int64             `json:"generation"`
	State              string            `json:"state"`
	Code               string            `json:"code,omitempty"`
	NotificationStatus string            `json:"notification_status"`
	LastAttempt        time.Time         `json:"last_attempt"`
	LastCompleted      time.Time         `json:"last_completed"`
	NextAttempt        time.Time         `json:"next_attempt"`
	ConsecutiveErrors  int               `json:"consecutive_errors"`
}

type WorkspaceDiscoveryStatus struct {
	State             string    `json:"state"`
	Code              string    `json:"code,omitempty"`
	LastAttempt       time.Time `json:"last_attempt"`
	LastSuccess       time.Time `json:"last_success"`
	NextAttempt       time.Time `json:"next_attempt"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
	ActiveWorkers     int       `json:"active_workers"`
	Capacity          int       `json:"capacity"`
}

type workspaceLifecycleStatus struct {
	mu        sync.RWMutex
	scopes    map[workspaceScope]WorkspaceRuntimeStatus
	discovery WorkspaceDiscoveryStatus
}

// WorkspaceRuntimeStatus requires a caller identity authenticated by the caller;
// every invocation rechecks current membership in the exact requested scope.
func (a *App) WorkspaceRuntimeStatus(ctx store.WorkspaceContext) (WorkspaceRuntimeStatus, error) {
	state, active, err := a.store.WorkspaceRuntimePublication(ctx)
	if err != nil {
		return WorkspaceRuntimeStatus{}, err
	}
	a.workspaceLifecycle.mu.RLock()
	out, ok := a.workspaceLifecycle.scopes[workspaceScope{ctx.TenantID, ctx.WorkspaceID}]
	discovery := a.workspaceLifecycle.discovery
	a.workspaceLifecycle.mu.RUnlock()
	if !ok || out.Revision != state.Revision || out.Generation != state.Generation {
		out = WorkspaceRuntimeStatus{TenantID: ctx.TenantID, WorkspaceID: ctx.WorkspaceID, Revision: state.Revision, Generation: state.Generation, State: "pending", NotificationStatus: "not_started"}
	}
	if state.Revision == 0 {
		out.State = "not_published"
		out.Code = ""
	} else if !active {
		out.State = "inactive"
		out.Code = "authority_inactive"
	} else if discovery.State == "error" {
		out.State = "error"
		out.Code = "discovery_failed"
	} else if discovery.State == "stopped" {
		out.State = "stopped"
		out.Code = ""
	}
	if out.State == "inactive" || out.State == "stopped" {
		out.NextAttempt = time.Time{}
	}
	return out, nil
}

// WorkspaceDiscoveryStatus is installation-wide and deliberately separate from
// scope reads. userID must come from the authenticated session/token, not input.
func (a *App) WorkspaceDiscoveryStatus(userID int64) (WorkspaceDiscoveryStatus, error) {
	if err := a.store.AuthorizeWorkspaceDiscovery(userID); err != nil {
		return WorkspaceDiscoveryStatus{}, err
	}
	a.workspaceLifecycle.mu.RLock()
	out := a.workspaceLifecycle.discovery
	a.workspaceLifecycle.mu.RUnlock()
	if out.State == "" {
		out.State = "not_started"
		out.Capacity = 8
	}
	return out, nil
}

func sanitizedWorkspaceNotificationStatus(status string) string {
	switch status {
	case "ready", "pending_delivery", "error", "blocked_unscoped_notifications", "blocked_unsupported_notifications":
		return status
	default:
		return "unknown"
	}
}

func workspaceRetryDelay(previous time.Duration) time.Duration {
	if previous == 0 {
		return time.Second
	}
	if previous >= 15*time.Second {
		return 30 * time.Second
	}
	return previous * 2
}
