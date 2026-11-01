package e2e

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/app"
)

// Real applications, independent SQLite/PKIs, authenticated pinned loopback
// transport, real scheduler/probes and log delivery. No preseeded snapshot.
func TestZoneSyncSourceLossAndSecondaryRestart(t *testing.T) {
	var shared, legacy, base, primary atomic.Int64
	var failed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shared":
			shared.Add(1)
		case "/legacy":
			legacy.Add(1)
		case "/base":
			base.Add(1)
		case "/primary":
			primary.Add(1)
		}
		if failed.Load() {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer target.Close()
	header := func(node string) string {
		return fmt.Sprintf("@ IN SOA %s. admin.localhost. ( 1 200ms 100ms 100ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off\n", node)
	}
	makeDir := func(node, extra string) string {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "monitors.bind"), []byte(header(node)+extra), 0600); err != nil {
			t.Fatal(err)
		}
		return d
	}
	logs := &zoneLog{}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("isolated zone sync runtime log:\n%s", logs.text())
		}
	})
	start := func(dir string) (*app.App, func()) {
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
	source, stopSource := start(makeDir("source", ""))
	defer func() {
		if stopSource != nil {
			stopSource()
		}
	}()
	body := header("source") + "source IN NODE\nsecondary IN NODE\nout IN CHANNEL log\ndefault IN POLICY chain=out\nshared IN HTTP " + target.URL + "/shared nodes=secondary\nlegacy IN HTTP " + target.URL + "/legacy\nprimary IN HTTP " + target.URL + "/primary nodes=source\n"
	z, err := source.ImportZone([]byte(body), app.ZoneImportOptions{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	z, err = source.RebindZone(z.Header.ZoneID, z.Revision(), app.ZoneBinding{SourceNode: "source"})
	if err != nil {
		t.Fatal(err)
	}
	z, err = source.ApplyZone(z.Header.ZoneID, z.Revision())
	if err != nil {
		t.Fatal(err)
	}
	token, err := source.GrantZoneSync(z.Header.ZoneID, "secondary")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(source.ZoneSyncHandler())
	srv.TLS = source.PKI().TLSConfig(nil)
	srv.StartTLS()
	defer srv.Close()
	dir := makeDir("secondary", "out IN CHANNEL log\ndefault IN POLICY chain=out\nbase IN HTTP "+target.URL+"/base\n")
	secondary, stopSecondary := start(dir)
	defer func() {
		if stopSecondary != nil {
			stopSecondary()
		}
	}()
	local, err := secondary.ImportZone([]byte(header("secondary")), app.ZoneImportOptions{Name: "subscribed"})
	if err != nil {
		t.Fatal(err)
	}
	local, err = secondary.RebindZone(local.Header.ZoneID, local.Revision(), app.ZoneBinding{SourceNode: "source", Fingerprint: strings.TrimPrefix(source.PKI().Fingerprint(), "sha256:"), SourceZoneID: z.Header.ZoneID, SourceGeneration: z.Header.Generation})
	if err != nil {
		t.Fatal(err)
	}
	startSync := func(a *app.App) func() {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- a.RunZoneSync(ctx, local.Header.ZoneID, app.ZoneSyncOptions{Endpoint: srv.URL, Token: token, Interval: time.Second})
		}()
		return func() {
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("sync stop: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Error("sync cancellation did not join")
			}
		}
	}
	stopSync := startSync(secondary)
	defer func() {
		if stopSync != nil {
			stopSync()
		}
	}()
	zoneEventually(t, func() bool {
		zs, e := secondary.ListZones()
		return e == nil && len(zs) == 1 && zs[0].Record.Synced != nil && shared.Load() > 1 && base.Load() > 1 && primary.Load() > 0
	})
	if len(secondary.RuntimeConfig().Monitors) != 3 {
		t.Fatal("secondary ran unauthorized assignment or missed legacy-local monitor")
	}
	srv.Close()
	stopSource()
	stopSource = nil
	time.Sleep(250 * time.Millisecond)
	primaryStopped := primary.Load()
	beforeShared, beforeBase := shared.Load(), base.Load()
	failed.Store(true)
	channel := "channel=zone-" + local.Header.ZoneID + ".out kind="
	zoneEventually(t, func() bool {
		return shared.Load() > beforeShared && base.Load() > beforeBase && strings.Contains(logs.text(), channel+"PROBLEM") && strings.Contains(logs.text(), "channel=out kind=PROBLEM")
	})
	failed.Store(false)
	zoneEventually(t, func() bool { return strings.Contains(logs.text(), channel+"RECOVERY") })
	stopSync()
	stopSync = nil
	stopSecondary()
	stopSecondary = nil
	beforeShared, beforeBase = shared.Load(), base.Load()
	beforeLegacy := legacy.Load()
	secondary, stopSecondary = start(dir)
	stopSync = startSync(secondary)
	zoneEventually(t, func() bool {
		return shared.Load() > beforeShared && base.Load() > beforeBase && legacy.Load() > beforeLegacy
	})
	// A failure shortly after recovery reopens the persisted incident;
	// restarting must not reset it to a new PROBLEM notification.
	reopensBefore := strings.Count(logs.text(), channel+"REOPEN")
	failed.Store(true)
	zoneEventually(t, func() bool { return strings.Count(logs.text(), channel+"REOPEN") > reopensBefore })
	if primary.Load() != primaryStopped {
		t.Fatal("unauthorized primary-only assignment ran on secondary")
	}
	zs, err := secondary.ListZones()
	if err != nil || zs[0].AppliedRevision == "" || zs[0].Record.Synced == nil {
		t.Fatal("lost authenticated last-good snapshot on restart")
	}
	t.Logf("authenticated sync; source stopped; secondary restarted offline: shared=%d legacy=%d base=%d; problem/recovery log delivery; forbidden assignment unchanged=%d", shared.Load(), legacy.Load(), base.Load(), primary.Load())
}
