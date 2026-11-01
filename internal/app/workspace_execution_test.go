package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func TestWorkspaceExecutionRevocationCancels(t *testing.T) {
	for _, action := range []string{"revoke", "disable", "replace"} {
		t.Run(action, func(t *testing.T) {
			a, ca, _ := workspaceConfigFixture(t)
			started := make(chan struct{})
			cancelled := make(chan struct{})
			release := make(chan struct{})
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-r.Context().Done():
					close(cancelled)
				case <-release:
				}
			}))
			defer target.Close()
			defer close(release)
			raw := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s timeout=5s header.X-Token=${secret:same}\n", target.URL))
			cfg, err := a.ApplyWorkspaceConfig(ca, raw, 0)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, e := a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation)
				done <- e
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("probe not started")
			}
			switch action {
			case "revoke":
				err = a.store.RevokeWorkspaceSecretReader(ca)
			case "disable":
				err = a.store.SetWorkspaceRuntimeActive(ca, false)
			case "replace":
				_, err = a.ApplyWorkspaceConfig(ca, raw, cfg.Revision)
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Error("revocation did not cancel in-flight HTTP")
			}
			select {
			case e := <-done:
				if e == nil {
					t.Error("revoked execution succeeded")
				}
			case <-time.After(time.Second):
				t.Error("execution did not join after revoke")
			}
			if _, e := a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation); e == nil {
				t.Fatal("old generation reused plaintext")
			}
			repo, e := a.store.WorkspaceMonitors(ca)
			if e != nil {
				t.Fatal(e)
			}
			states, e := repo.States()
			if e != nil || len(states) != 0 {
				t.Fatalf("cancelled result persisted: %+v %v", states, e)
			}
			ids, e := repo.MonitorIDs()
			if e != nil {
				t.Fatal(e)
			}
			for _, id := range ids {
				rounds, e := repo.Rounds(id, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 10)
				if e != nil || len(rounds) != 0 {
					t.Fatalf("cancelled history: %+v %v", rounds, e)
				}
			}
			t.Logf("%s cancelled real HTTP, joined executor, old generation denied; state/history empty", action)
		})
	}
}

func TestWorkspaceExecutionIsolation(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	var mu sync.Mutex
	seen := map[string]int{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Token")
		mu.Lock()
		seen[token]++
		mu.Unlock()
		if token == "department-A" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(503)
		}
	}))
	defer target.Close()
	before := a.store.Stats()
	events := 0
	a.SetPublisher(func(string, any) { events++ })
	ids := map[int64]bool{}
	for _, scope := range []store.WorkspaceContext{ca, cb} {
		raw := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s header.X-Token=${secret:same}\nblocked IN CHANNEL webhook url=%s/notify\ndefault IN POLICY chain=blocked\n", target.URL, target.URL))
		cfg, err := a.ApplyWorkspaceConfig(scope, raw, 0)
		if err != nil {
			t.Fatal(err)
		}
		got, err := a.RunWorkspaceChecks(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation)
		if err != nil {
			t.Fatal(err)
		}
		if got.NotificationStatus != "blocked_unscoped_notifications" || len(got.Checks) != 1 {
			t.Fatalf("result: %+v", got)
		}
		check := got.Checks[0]
		if check.Name != "check" || check.ID <= 0 || ids[check.ID] {
			t.Fatalf("identity: %+v", check)
		}
		ids[check.ID] = true
		want := "ok"
		if scope == cb {
			want = "critical"
		}
		if check.Status != want {
			t.Fatalf("check: %+v", check)
		}
		repo, err := a.store.WorkspaceMonitors(scope)
		if err != nil {
			t.Fatal(err)
		}
		states, err := repo.States()
		if err != nil || len(states) != 1 || states[0].Status != want || states[0].IncidentID != 0 {
			t.Fatalf("state: %+v %v", states, err)
		}
		rounds, err := repo.Rounds(check.ID, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 10)
		if err != nil || len(rounds) != 1 || rounds[0].Status != want {
			t.Fatalf("history: %+v %v", rounds, err)
		}
		t.Logf("scope=%s display=%s id=%d state=%s rounds=%d notification=%s", scope.WorkspaceID, check.Name, check.ID, states[0].Status, len(rounds), got.NotificationStatus)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["department-A"] != 1 || seen["department-B"] != 1 || len(seen) != 2 {
		t.Fatalf("actual HTTP headers: %v", seen)
	}
	t.Logf("localhost HTTP requests by credential: %v", seen)
	after := a.store.Stats()
	for _, table := range []string{"incidents", "events", "deliveries", "evidence"} {
		if before.Rows[table] != after.Rows[table] {
			t.Fatalf("unscoped %s changed", table)
		}
	}
	if events != 0 || len(a.RuntimeConfig().Monitors) != 0 || len(a.store.MonitorIDs()) != 0 {
		t.Fatal("workspace escaped to default runtime/events")
	}
}
