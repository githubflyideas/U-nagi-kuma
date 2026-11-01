package e2e

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/app"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// This is deliberately a runtime admission boundary test, NOT a claim that
// nondefault workspace execution is enabled. It exercises real SQLite, the
// application scheduler, HTTP probes, and reopen against the same dataset.
func TestWorkspaceRuntimeRejectsLegacyActivation(t *testing.T) {
	var base, foreign atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/foreign" {
			foreign.Add(1)
		} else {
			base.Add(1)
		}
		w.WriteHeader(200)
	}))
	defer target.Close()
	dir := t.TempDir()
	header := "@ IN SOA local. admin.localhost. ( 1 200ms 100ms 100ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off\n"
	original := header + "base IN HTTP " + target.URL + "/base\n"
	path := filepath.Join(dir, "monitors.bind")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	logs := &zoneLog{}
	start := func() (*app.App, func()) {
		a, err := app.New(app.Options{DataDir: dir, Log: slog.New(slog.NewTextHandler(logs, nil))})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- a.Run(ctx) }()
		return a, func() {
			cancel()
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
	}
	a, stop := start()
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()
	uid, err := a.Store().CreateUser("owner", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := a.Store().DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	owner := store.TenantContext{TenantID: tenant.ID, UserID: uid}
	ids := map[string]int64{}
	contexts := map[string]store.WorkspaceContext{}
	for _, name := range []string{"department-a", "department-b"} {
		ws, err := a.Store().CreateWorkspace(owner, name)
		if err != nil {
			t.Fatal(err)
		}
		wc := store.WorkspaceContext{TenantContext: owner, WorkspaceID: ws.ID}
		repo, err := a.Store().WorkspaceMonitors(wc)
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.SyncMonitors([]store.MonitorDef{{Name: name, DefHash: "same-target"}}); err != nil {
			t.Fatal(err)
		}
		id, err := repo.MonitorID(name)
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = id
		contexts[name] = wc
	}
	zoneEventually(t, func() bool { return base.Load() > 1 })
	etag := a.Config().ETag()
	for name := range ids {
		_, err = a.SaveConfig([]byte(strings.Replace(original, "( 1 ", "( 2 ", 1)+name+" IN HTTP "+target.URL+"/foreign\n"), etag, "test", "must reject foreign runtime")
		if err == nil {
			t.Fatalf("legacy config activated workspace monitor %s", name)
		}
		t.Logf("rejected %s: %v", name, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != original {
		t.Fatalf("rejected edit changed durable base: %v", err)
	}
	if a.Config().ETag() != etag {
		t.Fatal("rejected edit changed base revision")
	}
	a.Reapply()
	before := base.Load()
	zoneEventually(t, func() bool { return base.Load() > before })
	if foreign.Load() != 0 {
		t.Fatal("foreign monitor reached real probe")
	}
	stop()
	stopped = true
	a, stop = start()
	stopped = false
	before = base.Load()
	zoneEventually(t, func() bool { return base.Load() > before })
	for name, wc := range contexts {
		repo, err := a.Store().WorkspaceMonitors(wc)
		if err != nil {
			t.Fatal(err)
		}
		id, err := repo.MonitorID(name)
		if err != nil || id != ids[name] {
			t.Fatalf("base apply retired workspace %s: %d %v", name, id, err)
		}
		states, err := repo.States()
		if err != nil || len(states) != 0 {
			t.Fatalf("foreign runtime state created: %v %v", states, err)
		}
	}
	if foreign.Load() != 0 {
		t.Fatal("restart activated rejected foreign configuration")
	}
	t.Logf("real base probes=%d; foreign probes=%d; both department identities survive reapply/restart", base.Load(), foreign.Load())
}
