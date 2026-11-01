package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/tunables"
)

// Read the additive public snapshot through JSON so the initial RED is a
// behavior assertion (missing discovery API), not a compiler failure.
func discoveryJSON(t *testing.T, e *Engine) map[string]any {
	t.Helper()
	method := reflect.ValueOf(e).MethodByName("Discovery")
	if !method.IsValid() {
		t.Fatal("engine has no retained membership discovery snapshot")
	}
	b, err := json.Marshal(method.Call(nil)[0].Interface())
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDiscoveryPeriodicRegistryRefresh(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	ts, err := tunables.New(map[string]string{"cluster.discovery-interval": "1h", "localnet.enabled": "off"})
	if err != nil {
		t.Fatal(err)
	}
	var clockMu sync.Mutex
	e := New(Deps{Sink: newMemSink(), Clock: func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }})
	advance := func(d time.Duration) { clockMu.Lock(); now = now.Add(d); clockMu.Unlock(); e.do(func() { e.onTick() }) }
	e.Apply(Config{Self: "local", Nodes: []NodeSpec{{Name: "b"}, {Name: "a"}}, Tunables: ts})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("engine did not stop")
		}
	})
	first := discoveryJSON(t, e)
	if first["source"] != "configured_registry" || first["interval_s"] != float64(3600) {
		t.Fatalf("source/cadence: %+v", first)
	}
	if first["last_refresh"] != now.Format(time.RFC3339) || first["next_refresh"] != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("refresh metadata: %+v", first)
	}
	if !reflect.DeepEqual(first["members"], []any{"a", "b"}) {
		t.Fatalf("members: %+v", first)
	}
	advance(time.Hour - time.Nanosecond)
	if got := discoveryJSON(t, e); !reflect.DeepEqual(first, got) {
		t.Fatalf("refreshed before deadline: %+v", got)
	}
	advance(time.Nanosecond)
	got := discoveryJSON(t, e)
	if got["last_refresh"] != now.Format(time.RFC3339) || got["next_refresh"] != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("periodic refresh not run by engine tick: %+v", got)
	}
	if !reflect.DeepEqual(got["members"], first["members"]) {
		t.Fatalf("offline configured members disappeared: %+v", got)
	}
	// Session churn is not registry membership, and unknown peers cannot enroll.
	e.do(func() {
		e.onNodeState(NodeStatus{Node: "a", Online: true})
		e.onNodeState(NodeStatus{Node: "stranger", Online: true})
		e.onNodeState(NodeStatus{Node: "a", Online: false})
	})
	if churn := discoveryJSON(t, e); !reflect.DeepEqual(got, churn) {
		t.Fatalf("session churn changed discovery: %+v", churn)
	}
	detached := e.Discovery()
	detached.Members[0] = "mutated"
	if e.Discovery().Members[0] != "a" {
		t.Fatal("snapshot aliases engine state")
	}
	advance(3 * time.Hour)
	if snap := e.Discovery(); snap.LastRefresh != now || snap.NextRefresh != now.Add(time.Hour) {
		t.Fatalf("delayed tick replayed missed intervals: %+v", snap)
	}
	ts, err = tunables.New(map[string]string{"cluster.discovery-interval": "5m", "localnet.enabled": "off"})
	if err != nil {
		t.Fatal(err)
	}
	e.Apply(Config{Self: "local", Nodes: []NodeSpec{{Name: "c"}}, Tunables: ts})
	if snap := e.Discovery(); !reflect.DeepEqual(snap.Members, []string{"c"}) || snap.IntervalS != 300 || snap.NextRefresh != now.Add(5*time.Minute) {
		t.Fatalf("reload not immediate: %+v", snap)
	}
	e.Apply(Config{Self: "local", Tunables: ts})
	if snap := e.Discovery(); len(snap.Members) != 0 {
		t.Fatalf("removed nodes retained: %+v", snap)
	}
	cancel()
	<-done
	if snap := e.Discovery(); len(snap.Members) != 0 {
		t.Fatal("stopped engine returned stale members")
	}
}
