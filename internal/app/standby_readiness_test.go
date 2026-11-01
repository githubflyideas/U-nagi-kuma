package app

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"encoding/json"
	"github.com/githubflyideas/unagikuma/internal/backup"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
)

// The current wire protocol can enqueue Promote, but cannot acknowledge a
// verified snapshot or a successful restore. A connected socket is not ready.
func TestReadinessHandoverRefusesUnverifiedTarget(t *testing.T) {
	a, _, _, _ := newApp(t, testConfig+"\nspare IN NODE standby=https://127.0.0.1:1\n")
	defer a.Store().Close()
	m := cluster.NewMaster(cluster.MasterOptions{
		Auth:  func(string, string) (string, error) { return "spare", nil },
		Known: func(string) bool { return true },
	})
	defer m.Close()
	a.master.Close()
	a.master = m
	s := httptest.NewTLSServer(m)
	defer s.Close()
	promoted := make(chan struct{}, 1)
	ag, err := cluster.NewAgent(cluster.AgentOptions{Server: s.URL, Node: "spare", Token: "test", Insecure: true,
		OnPromote: func(uint64) { promoted <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _ = ag.Run(ctx) }()
	defer func() { cancel(); <-done }()
	waitFor(t, "standby connection", 3*time.Second, func() bool { return len(m.Connected()) == 1 })
	a.opts.HandoverWait = 10 * time.Millisecond
	term, err := a.HandOver("spare", "operator")
	if err == nil || !strings.Contains(err.Error(), "readiness") || term != 0 {
		t.Errorf("unverified handover must be refused, got term=%d err=%v", term, err)
	}
	select {
	case <-promoted:
		t.Error("unsafe promotion dispatched")
	case <-time.After(60 * time.Millisecond):
	}
	if a.SupersededBy() != nil {
		t.Error("source retired without readiness acknowledgement")
	}
}

func TestReadinessRefreshFailureDoesNotPromoteStaleSnapshot(t *testing.T) {
	a, _, _, dir := newApp(t, testConfig)
	defer a.Store().Close()
	m := cluster.NewMaster(cluster.MasterOptions{Auth: func(string, string) (string, error) { return "spare", nil }, Known: func(string) bool { return true }})
	defer m.Close()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == cluster.StatePath {
			http.Error(w, "snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		m.ServeHTTP(w, r)
	}))
	defer s.Close()
	ag, err := cluster.NewAgent(cluster.AgentOptions{Server: s.URL, Node: "spare", Token: "test", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = ag.Run(ctx) }()
	defer func() { cancel(); <-done }()
	waitFor(t, "connected", 3*time.Second, func() bool { return ag.Status().Connected })
	a.am.agent = ag
	if err := os.MkdirAll(filepath.Join(dir, standbyDir), 0700); err != nil {
		t.Fatal(err)
	}
	a.saveStandby(StandbyState{Key: "old-key", Term: 2, Fetched: time.Now().Add(-24 * time.Hour)})
	old := []byte("last known snapshot: preserve even on failed refresh")
	if err := os.WriteFile(filepath.Join(dir, standbyDir, stateFile), old, 0600); err != nil {
		t.Fatal(err)
	}
	term, err := a.Promote(context.Background(), 0, "operator")
	if err == nil || term != 0 {
		t.Errorf("failed refresh promoted stale state: term=%d err=%v", term, err)
	}
	select {
	case <-a.RestartRequested():
		t.Error("restart requested on failed refresh")
	default:
	}
	if _, err := os.Stat(filepath.Join(dir, promoteFile)); !os.IsNotExist(err) {
		t.Errorf("promotion marker exists: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, standbyDir, stateFile))
	if err != nil || !bytes.Equal(b, old) {
		t.Errorf("last copy lost: %v", err)
	}
}

func TestReadinessCoreSnapshotPreservesHistoryOutsideLiveDB(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "monitors.bind")
	if err := os.WriteFile(cfg, []byte(testConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, standbyDir), 0700); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, err := backup.Write(context.Background(), &buf, func(_ context.Context, _ string, w io.Writer) error { _, e := w.Write([]byte("new core")); return e }, backup.Options{DataDir: dir, ConfigPath: cfg, Passphrase: "key", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, standbyDir, stateFile), buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(StandbyState{Key: "key"})
	if err := os.WriteFile(filepath.Join(dir, standbyDir, stateMetaFile), b, 0600); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(filepath.Join(dir, "history.db"+s), []byte("local history"+s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := writePromote(dir, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	ok, err := applyPromote(dir, cfg)
	if err != nil || !ok {
		t.Fatalf("restore: %v %v", ok, err)
	}
	for _, s := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(filepath.Join(dir, "history.db"+s)); !os.IsNotExist(err) {
			t.Errorf("unreplicated history left as live master history: %v", err)
		}
		matches, err := filepath.Glob(filepath.Join(dir, standbyDir, "previous-history-*", "history.db"+s))
		if err != nil || len(matches) != 1 {
			t.Errorf("history was discarded instead of retained: %v %v", matches, err)
			continue
		}
		b, err := os.ReadFile(matches[0])
		if err != nil || string(b) != "local history"+s {
			t.Errorf("history changed: %q %v", b, err)
		}
	}
}

func TestReadinessSnapshotDeclaresCoreOnlyScope(t *testing.T) {
	source, _, _, _ := newApp(t, testConfig)
	defer source.Store().Close()
	target, _, _, _ := newApp(t, testConfig)
	defer target.Store().Close()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(cluster.StateTermHeader, "0")
		if err := source.writeState(w); err != nil {
			t.Errorf("write state: %v", err)
		}
	}))
	defer s.Close()
	ag, err := cluster.NewAgent(cluster.AgentOptions{Server: s.URL, Node: "spare", Token: "test", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := target.fetchState(context.Background(), ag)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(meta)
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["scope"] != "core-config-keys-no-history" {
		t.Errorf("snapshot scope not explicit: %s", encoded)
	}
	f, err := os.Open(target.dataPath(standbyDir, stateFile))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	restored := t.TempDir()
	manifest, err := backup.Restore(f, restored, filepath.Join(restored, "monitors.bind"), meta.Key, false)
	if err != nil {
		t.Fatal(err)
	}
	foundCore := false
	for _, name := range manifest.Files {
		if name == "history.db" {
			t.Error("unbounded history transfer enabled")
		}
		foundCore = foundCore || name == "core.db"
	}
	if !foundCore {
		t.Fatal("snapshot has no core database")
	}
}

func TestReadinessLegacyHandoverRequestRefused(t *testing.T) {
	a, _, _, dir := newApp(t, testConfig)
	defer a.Store().Close()
	if err := os.MkdirAll(filepath.Join(dir, standbyDir), 0700); err != nil {
		t.Fatal(err)
	}
	a.saveStandby(StandbyState{Key: "key", Term: 0})
	if err := os.WriteFile(filepath.Join(dir, standbyDir, stateFile), []byte("unverified snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	term, err := a.Promote(context.Background(), 1, "handover")
	if err == nil || !strings.Contains(err.Error(), "readiness") || term != 0 {
		t.Errorf("legacy handover accepted: term=%d err=%v", term, err)
	}
	select {
	case <-a.RestartRequested():
		t.Error("restart requested")
	default:
	}
	if _, err := os.Stat(filepath.Join(dir, promoteFile)); !os.IsNotExist(err) {
		t.Errorf("promotion marker exists: %v", err)
	}
}

func TestReadinessCorruptPromotionPreservesLocalState(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, standbyDir), 0700); err != nil {
		t.Fatal(err)
	}
	files := []string{"core.db", "core.db-wal", "core.db-shm", "history.db", "history.db-wal", "history.db-shm", "monitors.bind", agentFile, agentTokenFile}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep:"+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.Marshal(StandbyState{Key: "test"})
	if err := os.WriteFile(filepath.Join(dir, standbyDir, stateMetaFile), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, standbyDir, stateFile), []byte("corrupt archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writePromote(dir, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	ok, err := applyPromote(dir, filepath.Join(dir, "monitors.bind"))
	if err == nil || ok {
		t.Fatalf("corrupt promotion accepted: %v %v", ok, err)
	}
	for _, name := range files {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != "keep:"+name {
			t.Errorf("failed restore destroyed %s: %q %v", name, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, promoteFile)); err != nil {
		t.Errorf("lost promotion request: %v", err)
	}
}
