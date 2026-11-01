package server

import (
	"context"
	"github.com/githubflyideas/unagikuma/internal/app"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
)

func TestNodeDiscoveryRetainsOfflineMembers(t *testing.T) {
	e := newEnv(t, testBind+"$SET cluster.discovery-interval=1h\neast IN NODE East\nwest IN NODE West\n", nil)
	if code := e.do("GET", "/api/v1/nodes", nil, nil); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
	e.setup()
	var out struct {
		Layout                        string
		Nodes                         []nodeView
		Discovery                     *engine.DiscoverySnapshot
		Fingerprint, Server, Instance string
		Monitors                      int
	}
	read := func() {
		t.Helper()
		if code := e.do("GET", "/api/v1/nodes", nil, &out); code != 200 {
			t.Fatalf("status %d", code)
		}
	}
	read()
	if out.Discovery == nil {
		t.Fatal("missing discovery metadata")
	}
	before := *out.Discovery
	if before.Source != "configured_registry" || before.IntervalS != 3600 || !reflect.DeepEqual(before.Members, []string{"east", "west"}) {
		t.Fatalf("discovery %+v", before)
	}
	if before.LastRefresh.IsZero() || before.NextRefresh.Sub(before.LastRefresh) != time.Hour {
		t.Fatalf("refresh times %+v", before)
	}
	if out.Layout != "cluster" || len(out.Nodes) != 3 || !out.Nodes[0].Self || out.Fingerprint == "" || out.Server == "" || out.Instance != "test" || out.Monitors != 3 {
		t.Fatalf("legacy contract %+v", out)
	}
	e.app.Engine().NodeState(engine.NodeStatus{Node: "east", Online: true, LastSeen: time.Now().Add(-2 * time.Hour)})
	e.app.Engine().NodeState(engine.NodeStatus{Node: "east", Online: false, LastSeen: time.Now().Add(-2 * time.Hour)})
	e.app.Engine().NodeState(engine.NodeStatus{Node: "unknown", Online: true})
	read()
	if !reflect.DeepEqual(before, *out.Discovery) {
		t.Fatalf("session churn changed membership: %+v", out.Discovery)
	}
	for _, n := range out.Nodes {
		if !n.Self && n.Online {
			t.Fatalf("expected offline member: %+v", n)
		}
	}
}

func TestNodeDiscoveryBeforeRunIsBounded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(testBind), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := app.New(app.Options{DataDir: dir, Probe: okProbe})
	if err != nil {
		t.Fatal(err)
	}
	s := New(a, Options{})
	defer s.Close()
	// Start and stop the real application afterwards to release its resources
	// and the one legacy engine query waiting for Run.
	defer func() {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { _ = a.Run(ctx); close(done) }()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("app did not stop")
		}
		deadline := time.Now().Add(time.Second)
		for len(nodeListQueryGate) != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(nodeListQueryGate) != 0 {
			t.Error("query worker not released")
		}
	}()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		req := httptest.NewRequest("GET", "/api/v1/nodes", nil).WithContext(ctx)
		start := time.Now()
		_, err := s.nodesList(httptest.NewRecorder(), req)
		cancel()
		if err == nil || time.Since(start) > time.Second {
			t.Fatalf("pre-run query was not bounded: %v", err)
		}
		if len(nodeListQueryGate) != 1 {
			t.Fatalf("expected only one outstanding legacy query")
		}
	}
}

func TestNodeDiscoveryDefaultEmptyRegistry(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	var out struct {
		Discovery engine.DiscoverySnapshot
		Nodes     []nodeView
		Layout    string
	}
	if code := e.do("GET", "/api/v1/nodes", nil, &out); code != 200 {
		t.Fatalf("status %d", code)
	}
	if out.Discovery.Source != "configured_registry" || out.Discovery.IntervalS != 300 || out.Discovery.Members == nil || len(out.Discovery.Members) != 0 {
		t.Fatalf("empty registry %+v", out.Discovery)
	}
	if out.Layout != "single" || len(out.Nodes) != 1 || !out.Nodes[0].Self {
		t.Fatalf("local identity must remain separate: %+v", out)
	}
}

func TestNodeDiscoveryCanceledRequest(t *testing.T) {
	e := newEnv(t, testBind, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil).WithContext(ctx)
	_, err := e.srv.nodesList(httptest.NewRecorder(), req)
	if err == nil {
		t.Fatal("canceled metadata query must return an error")
	}
}
