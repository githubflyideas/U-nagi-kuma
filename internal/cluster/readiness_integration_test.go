package cluster_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/app"
	"github.com/githubflyideas/unagikuma/internal/backup"
	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/server"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// These are real application engines, SQLite stores, pinned TLS sessions and
// loopback HTTP probes, not mocked protocol replies. No external targets or
// notification channels are configured. Run serially: app has process signals.
type readinessApp struct {
	a        *app.App
	url, dir string
	stop     func()
}

func readinessStart(t *testing.T, dir, text string) readinessApp {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	if text != "" {
		if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.DiscardHandler)
	a, err := app.New(app.Options{DataDir: dir, Log: log, Version: "readiness-test"})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(a, server.Options{Log: log})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: srv.Handler()}
	go hs.Serve(cluster.SniffListener(ln, a.PKI().TLSConfig(nil), time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	var once sync.Once
	stop := func() { once.Do(func() { hs.Close(); cancel(); <-done; srv.Close() }) }
	t.Cleanup(stop)
	return readinessApp{a: a, dir: dir, url: "https://" + ln.Addr().String(), stop: stop}
}

func readinessWait(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("timeout: " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const readinessSOA = "@ IN SOA %s. admin.localhost. ( 2026102801 1s 500ms 300ms 1 )\n$SET engine.first-round-delay=0s localnet.enabled=off cluster.ping-interval=200ms api.login-rate=100/s\n"

func TestReadinessJoinToFirstResult(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer target.Close()
	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			master := readinessStart(t, "", fmt.Sprintf(readinessSOA, "master")+"web IN HTTP "+target.URL+"\n")
			joiner := readinessStart(t, "", fmt.Sprintf(readinessSOA, "box"))
			code, _, err := master.a.CreateJoinCode(master.url)
			if err != nil {
				t.Fatal(err)
			}
			info, err := cluster.DecodeJoin(code)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			_, err = joiner.a.JoinCluster(t.Context(), app.JoinOptions{Code: code, Node: "edge"})
			if err != nil {
				t.Fatal(err)
			}
			joined := time.Since(start)
			readinessWait(t, "configuration acknowledged", func() bool {
				for _, n := range master.a.Engine().Nodes() {
					if n.Node == "edge" && n.Online && n.Serial == master.a.Config().Serial {
						return true
					}
				}
				return false
			})
			ack := time.Since(start)
			readinessWait(t, "real remote HTTP result", func() bool {
				row, ok := master.a.Engine().Row("web")
				if !ok {
					return false
				}
				for _, v := range row.Votes {
					if v.Node == "edge" && v.Vote == "ok" {
						return true
					}
				}
				return false
			})
			result := time.Since(start)
			b, _ := json.Marshal(map[string]any{"sample": i, "join_ms": float64(joined) / float64(time.Millisecond), "ack_ms": float64(ack) / float64(time.Millisecond), "first_result_ms": float64(result) / float64(time.Millisecond)})
			t.Logf("READINESS_TIMING %s", b)
			if _, err := cluster.Join(t.Context(), info, cluster.JoinRequest{Code: info.Code, Node: "replay"}); err == nil {
				t.Fatal("join code replay accepted")
			}
			// Ordinary probe nodes cannot download the controller's database/key archive.
			token, err := os.ReadFile(filepath.Join(joiner.dir, "agent.token"))
			if err != nil {
				t.Fatal(err)
			}
			ag, err := cluster.NewAgent(cluster.AgentOptions{Server: master.url, Token: string(bytes.TrimSpace(token)), Fingerprint: master.a.PKI().Fingerprint()})
			if err != nil {
				t.Fatal(err)
			}
			var state bytes.Buffer
			if _, _, err := ag.FetchState(t.Context(), &state); err == nil {
				t.Fatal("probe-only node fetched controller state")
			}
		})
	}
}

func TestReadinessStandbyArchiveAndRestart(t *testing.T) {
	master := readinessStart(t, "", fmt.Sprintf(readinessSOA, "master"))
	joiner := readinessStart(t, "", fmt.Sprintf(readinessSOA, "box"))
	if err := master.a.Store().SetSetting("readiness-marker", "replicated-core"); err != nil {
		t.Fatal(err)
	}
	code, _, err := master.a.CreateJoinCode(master.url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = joiner.a.JoinCluster(t.Context(), app.JoinOptions{Code: code, Node: "edge"}); err != nil {
		t.Fatal(err)
	}
	readinessWait(t, "joined", func() bool { s := joiner.a.Agent(); return s.Status != nil && s.Status.Connected })
	_, err = master.a.EditConfig("readiness", "enable standby", func(f *config.File) error {
		return f.UpsertRecord(config.RecordSpec{Name: "edge", Type: "NODE", Opts: []config.Opt{{Key: "standby", Value: joiner.url}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	readinessWait(t, "downloaded archive", func() bool { return joiner.a.Standby().Size > 0 })
	meta := joiner.a.Standby()
	f, err := os.Open(filepath.Join(joiner.dir, "standby", "state.nkb"))
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	manifest, err := backup.Restore(f, dst, filepath.Join(dst, "monitors.bind"), meta.Key, true)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"core.db", "monitors.bind", "unagikuma.key", "cluster-ca.crt", "cluster-ca.key"} {
		if !slices.Contains(manifest.Files, name) {
			t.Fatalf("archive missing %s", name)
		}
	}
	// Characterization, NOT an endorsement: current standby snapshots omit history.
	if slices.Contains(manifest.Files, "history.db") {
		t.Fatal("history behavior changed: update readiness report")
	}
	t.Logf("READINESS_ARCHIVE %s", mustJSON(manifest.Files))
	master.stop()
	term, err := joiner.a.Promote(t.Context(), 0, "readiness")
	if err != nil {
		t.Fatal(err)
	}
	if term != 1 {
		t.Fatalf("promotion term %d", term)
	}
	select {
	case <-joiner.a.RestartRequested():
	case <-time.After(time.Second):
		t.Fatal("restart not requested")
	}
	joiner.stop()
	// Explicit supervisor restart is essential on Windows (reexec exits 3).
	promoted := readinessStart(t, joiner.dir, "")
	if promoted.a.Term() != 1 || promoted.a.Agent().Member != nil {
		t.Fatal("restart did not restore controller role")
	}
	marker, err := promoted.a.Store().Setting("readiness-marker")
	if err != nil || marker != "replicated-core" {
		t.Fatalf("restored core marker: %q %v", marker, err)
	}
	if promoted.a.PKI().Fingerprint() != master.a.PKI().Fingerprint() {
		t.Fatal("cluster CA changed")
	}
	t.Log("READINESS_PROMOTION explicit restart restored core setting, CA, membership and term; no automatic election")
}

func TestReadinessEnrollmentRaceAndExpiry(t *testing.T) {
	var now atomic.Int64
	now.Store(time.Now().UnixMilli())
	st, err := store.Open(t.TempDir(), store.Options{Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return time.UnixMilli(now.Load()) }})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	code, _, err := st.CreateJoinCode(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := st.UseJoinCode(code, fmt.Sprint(i))
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, store.ErrJoinCode) {
				t.Errorf("consume: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("single-use winners=%d", winners.Load())
	}
	code, exp, err := st.CreateJoinCode(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now.Store(exp.UnixMilli())
	if err := st.UseJoinCode(code, "expired"); !errors.Is(err, store.ErrJoinCode) {
		t.Fatalf("expiry boundary accepted: %v", err)
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
