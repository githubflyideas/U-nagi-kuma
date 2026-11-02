package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/app"
	"github.com/githubflyideas/unagikuma/internal/store"
)

const scheduleHeader = "@ IN SOA local. admin.localhost. ( 1 200ms 100ms 100ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off\n"

func scheduleOpen(t *testing.T, dir string) *app.App {
	t.Helper()
	a, err := app.New(app.Options{DataDir: dir, Log: slog.New(slog.NewTextHandler(&zoneLog{}, nil))})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func scheduleRun(t *testing.T, a *app.App) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(2 * time.Second):
				t.Error("App.Run did not join before Store close")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}
func scheduleScopes(t *testing.T, a *app.App) []store.WorkspaceContext {
	t.Helper()
	uid, err := a.Store().CreateUser("owner", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := a.Store().DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	owner := store.TenantContext{TenantID: tenant.ID, UserID: uid}
	var out []store.WorkspaceContext
	for _, name := range []string{"department-a", "department-b"} {
		w, err := a.Store().CreateWorkspace(owner, name)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, store.WorkspaceContext{TenantContext: owner, WorkspaceID: w.ID})
	}
	return out
}
func scheduleWait(t *testing.T, f func() bool) {
	t.Helper()
	end := time.Now().Add(4 * time.Second)
	for time.Now().Before(end) {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("autonomous workspace checks did not recur")
}
func scheduleFixture(t *testing.T) (*app.App, []store.WorkspaceContext, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(scheduleHeader), 0600); err != nil {
		t.Fatal(err)
	}
	a := scheduleOpen(t, dir)
	return a, scheduleScopes(t, a), dir
}

func scheduleStatus(t *testing.T, a *app.App, w store.WorkspaceContext) app.WorkspaceRuntimeStatus {
	t.Helper()
	s, e := a.WorkspaceRuntimeStatus(w)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestWorkspaceScheduleBlockedAndErrorBackoff(t *testing.T) {
	a, scopes, dir := scheduleFixture(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer target.Close()
	blocked := []byte(scheduleHeader + fmt.Sprintf("check IN HTTP %s timeout=6s\n", target.URL))
	good := []byte(scheduleHeader + fmt.Sprintf("check IN HTTP %s timeout=1s\n", target.URL))
	first, e := a.ApplyWorkspaceConfig(scopes[0], blocked, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.ApplyWorkspaceConfig(scopes[1], good, 0); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", filepath.ToSlash(filepath.Join(dir, "core.db")))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	// A pooled-connection-safe persistent trigger injects secret-bearing SQL text.
	_, e = db.Exec(`CREATE TRIGGER lifecycle_failure BEFORE INSERT ON monitors WHEN NEW.workspace_id='` + string(scopes[1].WorkspaceID) + `' BEGIN SELECT RAISE(ABORT,'PRIVATE-target-token'); END`)
	if e != nil {
		t.Fatal(e)
	}
	stop := scheduleRun(t, a)
	defer stop()
	for i, w := range scopes {
		scheduleWait(t, func() bool { return scheduleStatus(t, a, w).ConsecutiveErrors == 1 })
		s := scheduleStatus(t, a, w)
		want := "error"
		code := "execution_failed"
		if i == 0 {
			want = "blocked"
			code = "unsupported_execution"
		}
		if s.State != want || s.Code != code || s.NextAttempt.Sub(s.LastCompleted) != time.Second {
			t.Fatalf("first backoff %+v", s)
		}
		raw, _ := json.Marshal(s)
		if strings.Contains(string(raw), "PRIVATE") {
			t.Fatalf("unsafe error: %s", raw)
		}
	}
	scheduleWait(t, func() bool {
		return scheduleStatus(t, a, scopes[0]).ConsecutiveErrors == 2 && scheduleStatus(t, a, scopes[1]).ConsecutiveErrors == 2
	})
	for _, w := range scopes {
		s := scheduleStatus(t, a, w)
		if s.NextAttempt.Sub(s.LastCompleted) != 2*time.Second {
			t.Fatalf("second backoff %+v", s)
		}
	}
	if _, e = db.Exec(`DROP TRIGGER lifecycle_failure`); e != nil {
		t.Fatal(e)
	}
	next, e := a.ApplyWorkspaceConfig(scopes[0], good, first.Revision)
	if e != nil {
		t.Fatal(e)
	}
	scheduleWait(t, func() bool {
		s := scheduleStatus(t, a, scopes[0])
		return s.Revision == next.Revision && s.State == "scheduled" && s.ConsecutiveErrors == 0
	})
	scheduleWait(t, func() bool {
		s := scheduleStatus(t, a, scopes[1])
		return s.State == "scheduled" && s.ConsecutiveErrors == 0
	})
	if s := scheduleStatus(t, a, scopes[1]); s.NotificationStatus != "ready" {
		t.Fatalf("scheduler discarded executor notification status: %+v", s)
	}
}

func TestWorkspaceScheduleUnsupportedNotificationsVisible(t *testing.T) {
	a, scopes, _ := scheduleFixture(t)
	body := []byte(scheduleHeader + "one IN CHANNEL log\ntwo IN CHANNEL log\ndefault IN POLICY chain=one,two\n")
	if _, e := a.ApplyWorkspaceConfig(scopes[0], body, 0); e != nil {
		t.Fatal(e)
	}
	stop := scheduleRun(t, a)
	defer stop()
	scheduleWait(t, func() bool { return scheduleStatus(t, a, scopes[0]).ConsecutiveErrors > 0 })
	s := scheduleStatus(t, a, scopes[0])
	if s.State != "blocked" || s.Code != "unsupported_notifications" || s.NotificationStatus != "blocked_unsupported_notifications" {
		t.Fatalf("notification blocker not explicit: %+v", s)
	}
}

func TestWorkspaceScheduleDiscoveryFailureAndRecovery(t *testing.T) {
	a, scopes, dir := scheduleFixture(t)
	db, e := sql.Open("sqlite", filepath.ToSlash(filepath.Join(dir, "core.db")))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(`ALTER TABLE workspace_applied_configs RENAME TO lifecycle_hidden_configs`); e != nil {
		t.Fatal(e)
	}
	stop := scheduleRun(t, a)
	defer stop()
	scheduleWait(t, func() bool {
		s, e := a.WorkspaceDiscoveryStatus(scopes[0].UserID)
		return e == nil && s.State == "error"
	})
	d, e := a.WorkspaceDiscoveryStatus(scopes[0].UserID)
	if e != nil {
		t.Fatal(e)
	}
	if d.Code != "discovery_failed" || d.ConsecutiveErrors != 1 || d.NextAttempt.Sub(d.LastAttempt) != time.Second {
		t.Fatalf("discovery %+v", d)
	}
	time.Sleep(250 * time.Millisecond)
	d, e = a.WorkspaceDiscoveryStatus(scopes[0].UserID)
	if e != nil || d.ConsecutiveErrors != 1 {
		t.Fatalf("unbounded discovery retry %+v %v", d, e)
	}
	if _, e = db.Exec(`ALTER TABLE lifecycle_hidden_configs RENAME TO workspace_applied_configs`); e != nil {
		t.Fatal(e)
	}
	scheduleWait(t, func() bool {
		s, e := a.WorkspaceDiscoveryStatus(scopes[0].UserID)
		return e == nil && s.State == "ready" && s.ConsecutiveErrors == 0 && !s.LastSuccess.IsZero()
	})
}

func TestWorkspaceScheduleCapacityAndFairness(t *testing.T) {
	a, scopes, _ := scheduleFixture(t)
	for i := 2; i < 12; i++ {
		w, e := a.Store().CreateWorkspace(scopes[0].TenantContext, fmt.Sprintf("fair-%02d", i))
		if e != nil {
			t.Fatal(e)
		}
		scopes = append(scopes, store.WorkspaceContext{TenantContext: scopes[0].TenantContext, WorkspaceID: w.ID})
	}
	var mu sync.Mutex
	counts := map[string]int{}
	active, maxActive := 0, 0
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		select {
		case <-release:
		case <-r.Context().Done():
		}
		mu.Lock()
		active--
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer target.Close()
	defer unblock()
	for i, w := range scopes {
		if _, e := a.ApplyWorkspaceConfig(w, []byte(scheduleHeader+fmt.Sprintf("check 200ms IN HTTP %s/%d timeout=5s\n", target.URL, i)), 0); e != nil {
			t.Fatal(e)
		}
	}
	stop := scheduleRun(t, a)
	defer stop()
	scheduleWait(t, func() bool { mu.Lock(); defer mu.Unlock(); return active == 8 })
	// Hold all eight slots: the remaining four scopes must be visibly queued.
	queued, running := 0, 0
	for _, w := range scopes {
		s := scheduleStatus(t, a, w)
		if s.State == "queued" {
			queued++
		}
		if s.State == "running" {
			running++
		}
	}
	if queued != 4 || running != 8 {
		t.Fatalf("capacity status queued=%d running=%d", queued, running)
	}
	d, e := a.WorkspaceDiscoveryStatus(scopes[0].UserID)
	if e != nil || d.ActiveWorkers != 8 || d.Capacity != 8 {
		t.Fatalf("capacity %+v %v", d, e)
	}
	unblock()
	scheduleWait(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		if len(counts) != 12 {
			return false
		}
		for _, n := range counts {
			if n < 2 {
				return false
			}
		}
		return true
	})
	mu.Lock()
	defer mu.Unlock()
	if maxActive > 8 {
		t.Fatalf("worker cap violated: %d", maxActive)
	}
	t.Logf("12 scopes each executed at least twice; peak concurrent localhost probes=%d", maxActive)
}

func TestWorkspaceScheduleRevocationCancelsNoLateHistory(t *testing.T) {
	for _, action := range []string{"revoke", "disable", "replace"} {
		t.Run(action, func(t *testing.T) {
			a, scopes, _ := scheduleFixture(t)
			started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var startedOnce, canceledOnce sync.Once
			var mu sync.Mutex
			healthy := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/blocked" {
					startedOnce.Do(func() { close(started) })
					select {
					case <-r.Context().Done():
						canceledOnce.Do(func() { close(canceled) })
					case <-release:
					}
					return
				}
				mu.Lock()
				healthy++
				mu.Unlock()
				w.WriteHeader(200)
			}))
			defer target.Close()
			defer close(release)
			first, e := a.ApplyWorkspaceConfig(scopes[0], []byte(scheduleHeader+fmt.Sprintf("check IN HTTP %s/blocked timeout=5s\n", target.URL)), 0)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = a.ApplyWorkspaceConfig(scopes[1], []byte(scheduleHeader+fmt.Sprintf("other 200ms IN HTTP %s/healthy timeout=1s\n", target.URL)), 0); e != nil {
				t.Fatal(e)
			}
			stop := scheduleRun(t, a)
			defer stop()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("slow probe not started")
			}
			repo, e := a.Store().WorkspaceMonitors(scopes[0])
			if e != nil {
				t.Fatal(e)
			}
			ids, e := repo.MonitorIDs()
			if e != nil || len(ids) != 1 {
				t.Fatalf("registry %v %v", ids, e)
			}
			switch action {
			case "revoke":
				e = a.Store().RevokeWorkspaceSecretReader(scopes[0])
			case "disable":
				e = a.Store().SetWorkspaceRuntimeActive(scopes[0], false)
			case "replace":
				_, e = a.ApplyWorkspaceConfig(scopes[0], []byte(scheduleHeader+fmt.Sprintf("check IN HTTP %s/blocked timeout=5s disabled=yes\n", target.URL)), first.Revision)
			}
			if e != nil {
				t.Fatal(e)
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("revocation did not cancel probe before its 5s timeout")
			}
			scheduleWait(t, func() bool { mu.Lock(); defer mu.Unlock(); return healthy >= 3 })
			for _, id := range ids {
				rounds, e := repo.Rounds(id, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 100)
				if e != nil || len(rounds) != 0 {
					t.Fatalf("late history %+v %v", rounds, e)
				}
			}
			states, e := repo.States()
			if e != nil {
				t.Fatal(e)
			}
			if action == "replace" {
				if len(states) != 1 || states[0].Status != "paused" {
					t.Fatalf("replacement did not remain paused: %+v", states)
				}
			} else if len(states) != 0 {
				t.Fatalf("late states %+v", states)
			}
			if action != "replace" {
				s := scheduleStatus(t, a, scopes[0])
				if s.State != "inactive" || s.Code != "authority_inactive" {
					t.Fatalf("inactive %+v", s)
				}
			}
			t.Log("canceled in-flight 5s localhost probe within 1s; no late history; other Workspace continued")
		})
	}
}

func TestWorkspaceScheduleRecursAndRestarts(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(503)
	}))
	defer target.Close()
	count := func(path string) int { mu.Lock(); defer mu.Unlock(); return calls[path] }
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(scheduleHeader), 0600); err != nil {
		t.Fatal(err)
	}
	a := scheduleOpen(t, dir)
	scopes := scheduleScopes(t, a)
	body := func(dept string) []byte {
		return []byte(scheduleHeader + fmt.Sprintf("fast 200ms IN HTTP %s/%s\nslow 900ms IN HTTP %s/slow-%s\n", target.URL, dept, target.URL, dept))
	}
	states := make([]store.WorkspaceConfigState, 2)
	for i, w := range scopes {
		var err error
		states[i], err = a.ApplyWorkspaceConfig(w, body(fmt.Sprint(i)), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	stop := scheduleRun(t, a)
	scheduleWait(t, func() bool { return count("/0") >= 7 && count("/1") >= 7 })
	if fast, slow := count("/0"), count("/slow-0"); slow < 1 || fast < slow*2 {
		t.Fatalf("intervals not separated fast=%d slow=%d", fast, slow)
	}
	ids := make([]map[string]int64, 2)
	for i, w := range scopes {
		repo, err := a.Store().WorkspaceMonitors(w)
		if err != nil {
			t.Fatal(err)
		}
		ids[i], err = repo.MonitorIDs()
		if err != nil || len(ids[i]) != 2 {
			t.Fatalf("due subset retired full registry: %v %v", ids[i], err)
		}
	}
	if err := a.Store().SetWorkspaceRuntimeActive(scopes[0], false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	before := count("/0")
	b := count("/1")
	scheduleWait(t, func() bool { return count("/1") >= b+3 })
	if count("/0") != before {
		t.Fatal("disabled scope kept checking")
	}
	if _, err := a.ApplyWorkspaceConfig(scopes[1], body("updated"), states[1].Revision); err != nil {
		t.Fatal(err)
	}
	scheduleWait(t, func() bool { return count("/updated") >= 2 })
	stop()
	disabledCount := count("/0")
	before = count("/updated")
	time.Sleep(300 * time.Millisecond)
	if count("/updated") != before {
		t.Fatal("HTTP after App.Run returned")
	}
	a = scheduleOpen(t, dir)
	if s := scheduleStatus(t, a, scopes[1]); s.State != "pending" || s.Revision != states[1].Revision+1 || s.NotificationStatus != "not_started" {
		t.Fatalf("restart snapshot must await execution: %+v", s)
	}
	if s := scheduleStatus(t, a, scopes[0]); s.State != "inactive" {
		t.Fatalf("disabled publication after restart: %+v", s)
	}
	stop = scheduleRun(t, a)
	scheduleWait(t, func() bool { return count("/updated") >= before+3 })
	scheduleWait(t, func() bool {
		s := scheduleStatus(t, a, scopes[1])
		return s.State == "scheduled" && !s.LastCompleted.IsZero() && s.Revision == states[1].Revision+1
	})
	if count("/0") != disabledCount {
		t.Fatal("restart reactivated disabled scope")
	}
	for i, w := range scopes {
		repo, err := a.Store().WorkspaceMonitors(w)
		if err != nil {
			t.Fatal(err)
		}
		got, err := repo.MonitorIDs()
		if err != nil {
			t.Fatal(err)
		}
		for name, id := range ids[i] {
			if got[name] != id {
				t.Fatalf("ID changed across due subset/update/restart: %s", name)
			}
		}
	}
	if err := a.Store().RevokeWorkspaceSecretReader(scopes[1]); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	before = count("/updated")
	time.Sleep(500 * time.Millisecond)
	if count("/updated") != before {
		t.Fatal("revoked scope kept checking")
	}
	stop()
	if count("/notify") != 0 {
		t.Fatal("unscoped notification dispatched or replayed")
	}
	t.Logf("autonomous loopback counts: fast A=%d slow A=%d B=%d updated=%d; restart IDs stable; notifications=0", count("/0"), count("/slow-0"), count("/1"), count("/updated"))
}
