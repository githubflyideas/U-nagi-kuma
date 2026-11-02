package app

import (
	"context"
	"errors"
	"github.com/githubflyideas/unagikuma/internal/store"
	"testing"
	"time"
)

func TestWorkspaceLifecycleStatusSanitizationAndBackoff(t *testing.T) {
	for _, v := range []string{"PRIVATE http://secret:password@localhost", "", "delivered"} {
		if got := sanitizedWorkspaceNotificationStatus(v); got != "unknown" {
			t.Fatalf("unsafe status %q", got)
		}
	}
	for _, v := range []string{"ready", "pending_delivery", "error", "blocked_unscoped_notifications", "blocked_unsupported_notifications"} {
		if got := sanitizedWorkspaceNotificationStatus(v); got != v {
			t.Fatalf("status %q became %q", v, got)
		}
	}
	var delay time.Duration
	for _, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second} {
		delay = workspaceRetryDelay(delay)
		if delay != want {
			t.Fatalf("backoff %v want %v", delay, want)
		}
	}
}

func TestWorkspaceLifecycleStatusAuthorization(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	got, err := a.WorkspaceRuntimeStatus(ca)
	if err != nil || got.State != "not_published" || got.NotificationStatus != "not_started" {
		t.Fatalf("initial status=%+v err=%v", got, err)
	}
	if _, err = a.WorkspaceRuntimeStatus(store.WorkspaceContext{}); !errors.Is(err, store.ErrTenantAccess) {
		t.Fatalf("anonymous: %v", err)
	}
	uid, err := a.store.CreateUserRole("status-viewer", "password", store.RoleViewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.store.SetWorkspaceMember(ca, uid, store.RoleViewer, false); err != nil {
		t.Fatal(err)
	}
	viewer := ca
	viewer.UserID = uid
	if _, err = a.WorkspaceRuntimeStatus(viewer); err != nil {
		t.Fatal(err)
	}
	viewer.WorkspaceID = cb.WorkspaceID
	if _, err = a.WorkspaceRuntimeStatus(viewer); !errors.Is(err, store.ErrTenantAccess) {
		t.Fatalf("foreign scope: %v", err)
	}
	if _, err = a.WorkspaceDiscoveryStatus(uid); !errors.Is(err, store.ErrTenantAccess) {
		t.Fatalf("viewer discovery: %v", err)
	}
	if _, err = a.WorkspaceDiscoveryStatus(ca.UserID); err != nil {
		t.Fatal(err)
	}
	viewer.WorkspaceID = ca.WorkspaceID
	if err = a.store.RemoveWorkspaceMember(ca, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = a.WorkspaceRuntimeStatus(viewer); !errors.Is(err, store.ErrTenantAccess) {
		t.Fatalf("revoked membership: %v", err)
	}
}

func TestWorkspaceLifecycleDiscoveryFailureVisible(t *testing.T) {
	a, ca, _ := workspaceConfigFixture(t)
	// A closed core database deterministically exercises discovery failure without
	// injecting database error text into any public snapshot.
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); a.runWorkspaceSchedules(ctx) }()
	defer func() { cancel(); <-done }()
	end := time.Now().Add(time.Second)
	for time.Now().Before(end) {
		a.workspaceLifecycle.mu.RLock()
		d := a.workspaceLifecycle.discovery
		a.workspaceLifecycle.mu.RUnlock()
		if d.State == "error" {
			if d.Code != "discovery_failed" || d.ConsecutiveErrors != 1 || d.NextAttempt.IsZero() {
				t.Fatalf("discovery=%+v", d)
			}
			time.Sleep(200 * time.Millisecond)
			a.workspaceLifecycle.mu.RLock()
			again := a.workspaceLifecycle.discovery
			a.workspaceLifecycle.mu.RUnlock()
			if again.ConsecutiveErrors != 1 {
				t.Fatalf("discovery retry ignored backoff: %+v", again)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no discovery failure state for %s", ca.WorkspaceID)
}
