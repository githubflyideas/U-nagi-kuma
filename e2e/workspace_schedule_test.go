package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		return []byte(scheduleHeader + fmt.Sprintf("fast 200ms IN HTTP %s/%s\nslow 900ms IN HTTP %s/slow-%s\nout IN CHANNEL webhook url=%s/notify\ndefault IN POLICY chain=out\n", target.URL, dept, target.URL, dept, target.URL))
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
	stop = scheduleRun(t, a)
	scheduleWait(t, func() bool { return count("/updated") >= before+3 })
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
