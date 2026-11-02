package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func TestWorkspaceNotificationVerticalSlice(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	var mu sync.Mutex
	healthy := false
	seen := map[string][]string{}
	probes := map[string]bool{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/check" {
			probes[r.Header.Get("X-Token")] = true
			if !healthy {
				w.WriteHeader(503)
			}
			return
		}
		var m struct {
			Kind string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Error(err)
		}
		seen[r.URL.Path+":"+r.Header.Get("X-Token")] = append(seen[r.URL.Path+":"+r.Header.Get("X-Token")], m.Kind)
	}))
	defer target.Close()
	gens := map[store.WorkspaceID]int64{}
	for _, s := range []store.WorkspaceContext{ca, cb} {
		raw := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s/check header.X-Token=${secret:same}\nroute IN CHANNEL webhook url=%s/%s header.X-Token=${secret:same}\ndefault IN POLICY chain=route\n", target.URL, target.URL, s.WorkspaceID))
		cfg, err := a.ApplyWorkspaceConfig(s, raw, 0)
		if err != nil {
			t.Fatal(err)
		}
		gens[s.WorkspaceID] = cfg.Generation
		for i := 0; i < 2; i++ {
			out, err := a.RunWorkspaceChecks(context.Background(), s.TenantID, s.WorkspaceID, cfg.Generation)
			if err != nil {
				t.Fatal(err)
			}
			if out.NotificationStatus != "ready" {
				t.Fatalf("notification status %s", out.NotificationStatus)
			}
		}
	}
	mu.Lock()
	healthy = true
	mu.Unlock()
	for _, s := range []store.WorkspaceContext{ca, cb} {
		for i := 0; i < 2; i++ {
			if _, err := a.RunWorkspaceChecks(context.Background(), s.TenantID, s.WorkspaceID, gens[s.WorkspaceID]); err != nil {
				t.Fatal(err)
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for i, s := range []store.WorkspaceContext{ca, cb} {
		key := fmt.Sprintf("/%s:department-%c", s.WorkspaceID, 'A'+i)
		got := seen[key]
		if len(got) != 2 || got[0] != "PROBLEM" || got[1] != "RECOVERY" {
			t.Fatalf("route %s: %v", key, got)
		}
	}
	if len(probes) != 2 || !probes["department-A"] || !probes["department-B"] {
		t.Fatalf("shared target credentials %v", probes)
	}
	if len(seen) != 2 {
		t.Fatalf("unexpected routes %v", seen)
	}
}

func TestWorkspaceNotificationRevocationCancels(t *testing.T) {
	a, scope, _ := workspaceConfigFixture(t)
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/check" {
			w.WriteHeader(503)
			return
		}
		var body any
		_ = json.NewDecoder(r.Body).Decode(&body)
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	cfg, e := a.ApplyWorkspaceConfig(scope, []byte(runtimeSOA+fmt.Sprintf("check IN HTTP %s/check\nroute IN CHANNEL webhook url=%s/notify\ndefault IN POLICY chain=route\n", server.URL, server.URL)), 0)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		_, e := a.RunWorkspaceChecks(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation)
		done <- e
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("notification not started")
	}
	if e = a.store.RevokeWorkspaceSecretReader(scope); e != nil {
		t.Fatal(e)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("notification not cancelled")
	}
	select {
	case e = <-done:
		if !errors.Is(e, store.ErrTenantAccess) && !errors.Is(e, context.Canceled) {
			t.Fatalf("revoke error %v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch did not join")
	}
	if _, e = a.DispatchWorkspaceNotifications(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("stale dispatch %v", e)
	}
	events, e := a.WorkspaceEvents(scope, 0, 100)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range events {
		if v.Kind == "delivered_PROBLEM" {
			t.Fatal("revoked delivery acknowledged")
		}
	}
}
