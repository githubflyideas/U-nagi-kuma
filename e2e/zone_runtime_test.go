package e2e

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/app"
)

type zoneLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *zoneLog) Write(p []byte) (int, error) { w.mu.Lock(); defer w.mu.Unlock(); return w.b.Write(p) }
func (w *zoneLog) text() string                { w.mu.Lock(); defer w.mu.Unlock(); return w.b.String() }
func zoneEventually(t *testing.T, f func() bool) {
	t.Helper()
	end := time.Now().Add(15 * time.Second)
	for time.Now().Before(end) {
		if f() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("local runtime condition not observed")
}

// This is a real in-process application, SQLite store, scheduler, HTTP probe
// and log sender. Rebinding to an unavailable source is NOT a sync proof.
func TestZoneRuntimeSurvivesRestart(t *testing.T) {
	var base, one, two atomic.Int64
	var failed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/base":
			base.Add(1)
		case "/one":
			one.Add(1)
		case "/two":
			two.Add(1)
		}
		if failed.Load() {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer target.Close()
	dir := t.TempDir()
	logs := &zoneLog{}
	header := "@ IN SOA local. admin.localhost. ( 1 200ms 100ms 100ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off\n"
	body := func(path string) string {
		return header + "out IN CHANNEL log\ndefault IN POLICY chain=out\ncheck IN HTTP " + target.URL + path + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(body("/base")), 0600); err != nil {
		t.Fatal(err)
	}
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
	z1, err := a.ImportZone([]byte(body("/one")), app.ZoneImportOptions{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	z2, err := a.ImportZone([]byte(body("/two")), app.ZoneImportOptions{Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z1.Header.ZoneID, z1.Revision()); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z2.Header.ZoneID, z2.Revision()); err != nil {
		t.Fatal(err)
	}
	zoneEventually(t, func() bool { return base.Load() > 1 && one.Load() > 1 && two.Load() > 1 })
	z1, err = a.RebindZone(z1.Header.ZoneID, z1.Revision(), app.ZoneBinding{SourceNode: "unavailable", Fingerprint: strings.Repeat("ab", 32)})
	if err != nil {
		t.Fatal(err)
	}
	// The trusted-source metadata must not switch global application role.
	a.Reapply()
	a.ChangesChanged()
	b, o, s := base.Load(), one.Load(), two.Load()
	zoneEventually(t, func() bool { return base.Load() > b && one.Load() > o && two.Load() > s })
	ids := a.Store().MonitorIDs()
	if len(ids) != 3 {
		t.Fatalf("persistent runtime IDs: %v", ids)
	}
	// A base-file edit must retain both applied zones.
	if _, err = a.SaveConfig([]byte(strings.Replace(body("/base"), "( 1 ", "( 2 ", 1)), a.Config().ETag(), "test", "base reapply"); err != nil {
		t.Fatal(err)
	}
	// A newer unapplied draft must not replace the persisted running body.
	draft := strings.Replace(body("/not-applied"), "( 1 ", "( 2 ", 1)
	if _, err = a.ImportZone([]byte(draft), app.ZoneImportOptions{Name: "second", TargetID: z2.Header.ZoneID, ExpectedRevision: z2.Revision()}); err != nil {
		t.Fatal(err)
	}
	stop()
	stopped = true
	b, o, s = base.Load(), one.Load(), two.Load()
	a, stop = start()
	stopped = false
	zoneEventually(t, func() bool { return base.Load() > b && one.Load() > o && two.Load() > s })
	for name, id := range ids {
		if a.Store().MonitorID(name) != id {
			t.Fatalf("runtime ID changed on restart: %s", name)
		}
	}
	failed.Store(true)
	zoneEventually(t, func() bool {
		text := logs.text()
		return strings.Contains(text, "channel=zone-"+z1.Header.ZoneID+".out kind=PROBLEM") && strings.Contains(text, "channel=zone-"+z2.Header.ZoneID+".out kind=PROBLEM") && strings.Contains(text, "channel=out kind=PROBLEM")
	})
	statuses, err := a.ListZones()
	if err != nil || len(statuses) != 2 {
		t.Fatalf("list: %v %v", statuses, err)
	}
	for _, z := range statuses {
		if z.AppliedRevision == "" {
			t.Fatal("lost applied snapshot")
		}
	}
	t.Logf("real HTTP checks after restart: base=%d first=%d second=%d; independent log deliveries observed", base.Load(), one.Load(), two.Load())
}

// Standalone, multi-zone/source-offline (above), and invalid/CAS/recovery
// (below) each allocate independent application datasets and loopback servers.
func TestZoneRuntimeStandalone(t *testing.T) {
	var calls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer target.Close()
	logs := &zoneLog{}
	dir := t.TempDir()
	text := "@ IN SOA local. admin.localhost. ( 1 200ms 100ms 100ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off\nout IN CHANNEL log\ndefault IN POLICY chain=out\nbase IN HTTP " + target.URL + "/\n"
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := app.New(app.Options{DataDir: dir, Log: slog.New(slog.NewTextHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	zoneEventually(t, func() bool { return calls.Load() > 1 && strings.Contains(logs.text(), "channel=out kind=PROBLEM") })
	zones, err := a.ListZones()
	if err != nil || len(zones) != 0 {
		t.Fatalf("standalone zones: %v %v", zones, err)
	}
	t.Logf("standalone real checks=%d; log problem delivered", calls.Load())
}

func TestZoneRuntimeInvalidCASAndRecovery(t *testing.T) {
	var calls, unauthorized atomic.Int64
	var failed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Token") != "local-fixture" {
			unauthorized.Add(1)
		}
		if failed.Load() {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer target.Close()
	const env = "UNAGIKUMA_ZONE_RUNTIME_TOKEN"
	t.Setenv(env, "local-fixture")
	logs := &zoneLog{}
	dir := t.TempDir()
	header := "@ IN SOA local. admin.localhost. ( 1 200ms 100ms 100ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off\n"
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(header), 0600); err != nil {
		t.Fatal(err)
	}
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
	raw := header + "out IN CHANNEL log\ndefault IN POLICY chain=out\ncheck IN HTTP " + target.URL + "/ header.X-Token=${env:" + env + "}\n"
	z, err := a.ImportZone([]byte(raw), app.ZoneImportOptions{Name: "safe"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(z.Header.ZoneID, z.Revision()); err != nil {
		t.Fatal(err)
	}
	zoneEventually(t, func() bool { return calls.Load() > 1 })
	if _, err = a.ApplyZone(z.Header.ZoneID, "stale"); err != app.ErrConflict {
		t.Fatalf("stale apply: %v", err)
	}
	if _, err = a.ImportZone([]byte("not configuration"), app.ZoneImportOptions{Name: "invalid"}); err == nil {
		t.Fatal("invalid import accepted")
	}
	bad := strings.Replace(strings.Replace(raw, "( 1 ", "( 2 ", 1), env, "UNAGIKUMA_ZONE_ABSENT", 1)
	t.Setenv("UNAGIKUMA_ZONE_ABSENT", "")
	draft, err := a.ImportZone([]byte(bad), app.ZoneImportOptions{Name: "safe", TargetID: z.Header.ZoneID, ExpectedRevision: z.Revision()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyZone(draft.Header.ZoneID, draft.Revision()); err == nil {
		t.Fatal("invalid dependency applied")
	}
	zs, err := a.ListZones()
	if err != nil || len(zs) != 1 || zs[0].AppliedRevision != z.Revision() {
		t.Fatal("failed apply replaced running snapshot")
	}
	failed.Store(true)
	channel := "channel=zone-" + z.Header.ZoneID + ".out kind="
	zoneEventually(t, func() bool { return strings.Contains(logs.text(), channel+"PROBLEM") })
	failed.Store(false)
	zoneEventually(t, func() bool { return strings.Contains(logs.text(), channel+"RECOVERY") })
	stop()
	stopped = true
	// Missing credentials after restart must abstain, not issue a request with
	// an empty substituted header. Other independent zones need not stop.
	t.Setenv(env, "")
	before := calls.Load()
	a, stop = start()
	stopped = false
	time.Sleep(700 * time.Millisecond)
	if unauthorized.Load() != 0 || calls.Load() != before {
		t.Fatalf("missing restart dependency still probed: unauthorized=%d calls=%d before=%d", unauthorized.Load(), calls.Load(), before)
	}
	t.Setenv(env, "local-fixture")
	a.Reapply()
	zoneEventually(t, func() bool { return calls.Load() > before })
	t.Logf("invalid import/CAS rejected; real PROBLEM and RECOVERY delivered; restart missing dependency abstained; restored local checks=%d", calls.Load())
}
