package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/hostclock"
	"github.com/githubflyideas/unagikuma/internal/netscope"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/tunables"
)

// memSink keeps what the engine persists, for assertions.
type memSink struct {
	mu        sync.Mutex
	nextID    int64
	incidents map[int64]Incident
	events    []Event
	evidence  map[string]int // "monitor/node" → count
}

func newMemSink() *memSink {
	return &memSink{incidents: map[int64]Incident{}, evidence: map[string]int{}}
}

func (s *memSink) SaveState(StateRecord)   {}
func (s *memSink) DeleteState(string)      {}
func (s *memSink) Verdict(VerdictRecord)   {}
func (s *memSink) Delivery(DeliveryRecord) {}
func (s *memSink) Counter(string, int)     {}
func (s *memSink) SaveIntent(*Intent) error {
	return nil
}
func (s *memSink) Evidence(name, node string, _ time.Time, _ probe.Result) {
	s.mu.Lock()
	s.evidence[name+"/"+node]++
	s.mu.Unlock()
}
func (s *memSink) SaveIncident(inc *Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inc.ID == 0 {
		s.nextID++
		inc.ID = s.nextID
	}
	s.incidents[inc.ID] = *inc
	return nil
}
func (s *memSink) Event(ev Event) {
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()
}

func (s *memSink) open(kind, key string) (Incident, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inc := range s.incidents {
		if inc.Kind == kind && (inc.Monitor == key || inc.System == key) && inc.State.Open() {
			return inc, true
		}
	}
	return Incident{}, false
}

// fakeCluster answers probe requests for agents and the local probe from
// one table of per-node votes.
type fakeCluster struct {
	mu     sync.Mutex
	e      *Engine
	votes  map[string]probe.Vote // node → vote for every monitor
	silent map[string]bool       // node accepts requests but never answers
	asked  map[string]int
}

func (f *fakeCluster) set(node string, v probe.Vote) {
	f.mu.Lock()
	f.votes[node] = v
	f.mu.Unlock()
}

func (f *fakeCluster) result(node string) probe.Result {
	f.mu.Lock()
	v, ok := f.votes[node]
	f.mu.Unlock()
	ev := &probe.Evidence{Peer: "10.0.0.1:80"}
	if !ok || v == probe.VoteOK {
		return probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Latency: 2 * time.Millisecond, Evidence: ev}
	}
	return probe.Result{Vote: probe.VoteFail, Reason: probe.ReasonTimeout, Detail: "timeout", Evidence: ev}
}

func (f *fakeCluster) Probe(node string, req ProbeRequest) bool {
	f.mu.Lock()
	f.asked[node]++
	silent := f.silent[node]
	f.mu.Unlock()
	if silent {
		return true
	}
	go func() {
		r := f.result(node)
		f.e.RemoteVote(RemoteVote{Node: node, Round: req.Round, Monitor: req.Monitor, DefHash: req.DefHash, Result: r, Vote: r.Vote, Reason: r.Reason})
	}()
	return true
}

type clusterEnv struct {
	t    *testing.T
	e    *Engine
	sink *memSink
	fc   *fakeCluster
}

func newClusterEnv(t *testing.T, cfg Config) *clusterEnv {
	t.Helper()
	sink := newMemSink()
	fc := &fakeCluster{votes: map[string]probe.Vote{}, silent: map[string]bool{}, asked: map[string]int{}}
	e := New(Deps{
		Probe: func(ctx context.Context, s probe.Spec) probe.Result { return fc.result(cfg.Self) },
		Send: func(context.Context, notify.Channel, *notify.Message) notify.Attempt {
			return notify.Attempt{OK: true}
		},
		Sink:      sink,
		HostClock: func() (hostclock.Reading, bool) { return hostclock.Reading{}, false },
		Gateways:  func() ([]netscope.Gateway, error) { return nil, nil },
		Remote:    fc,
	})
	fc.e = e
	ts, err := tunables.New(map[string]string{
		"engine.first-round-delay": "0s", "suspend.recheck-spread": "0s", "suspend.tick": "200ms",
		"engine.round-grace": "0s", "cluster.vote-grace": "300ms", "localnet.enabled": "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Tunables = ts
	e.Apply(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return &clusterEnv{t: t, e: e, sink: sink, fc: fc}
}

func (c *clusterEnv) online(nodes ...string) {
	for _, n := range nodes {
		c.e.NodeState(NodeStatus{Node: n, Online: true, Since: time.Now()})
	}
}

func (c *clusterEnv) waitRow(name string, what string, cond func(MonitorRow) bool) MonitorRow {
	c.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, ok := c.e.Row(name)
		if ok && cond(r) {
			return r
		}
		if time.Now().After(deadline) {
			c.t.Fatalf("%s: last row %+v votes %+v", what, r, r.Votes)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func clusterMonitor(nodes ...string) MonitorSpec {
	return MonitorSpec{
		Name: "web", Title: "web", Probe: probe.Spec{Name: "web", Type: "TCP", Target: "10.0.0.1:80", Timeout: 100 * time.Millisecond},
		Interval: 40 * time.Millisecond, Attempts: 1, RecoverAttempts: 1, DefHash: "d1", Nodes: nodes,
		Quorum: QuorumRule{Spec: "majority"},
	}
}

func clusterCfg(m MonitorSpec, agents ...string) Config {
	cfg := Config{Self: "m", Monitors: []MonitorSpec{m}}
	for _, a := range agents {
		cfg.Nodes = append(cfg.Nodes, NodeSpec{Name: a, Probe: true, Timeout: time.Hour})
	}
	return cfg
}

func TestClusterMajority(t *testing.T) {
	c := newClusterEnv(t, clusterCfg(clusterMonitor("m", "a", "b"), "a", "b"))
	c.online("a", "b")
	r := c.waitRow("web", "all ok", func(r MonitorRow) bool { return r.Status == StatusOK && len(r.Votes) == 3 && r.Votes[1].Vote == "ok" })
	if r.Quorum == nil || r.Quorum.N != 3 || r.Quorum.QEff != 2 {
		t.Fatalf("quorum %+v", r.Quorum)
	}
	// One node failing is a partial failure, not an outage.
	c.fc.set("a", probe.VoteFail)
	c.waitRow("web", "partial", func(r MonitorRow) bool { return r.Partial && r.Status == StatusOK })
	// Two of three failing is.
	c.fc.set("b", probe.VoteFail)
	r = c.waitRow("web", "critical", func(r MonitorRow) bool { return r.Status == StatusCritical })
	if r.Quorum.Fail != 2 {
		t.Fatalf("fail votes %+v", r.Quorum)
	}
	if _, ok := c.sink.open(IncidentNormal, "web"); !ok {
		t.Fatal("no incident for a quorum failure")
	}
	c.sink.mu.Lock()
	ev := c.sink.evidence["web/a"]
	c.sink.mu.Unlock()
	if ev == 0 {
		t.Fatal("agent evidence not recorded")
	}
}

func TestClusterNodeLossDegrades(t *testing.T) {
	// b never connects: with two healthy nodes the quorum stays 2, so one
	// failing node is still only partial…
	c := newClusterEnv(t, clusterCfg(clusterMonitor("m", "a", "b"), "a", "b"))
	c.online("a")
	c.fc.set("a", probe.VoteFail)
	r := c.waitRow("web", "partial with b offline", func(r MonitorRow) bool { return r.Partial })
	if r.Votes[2].Vote != "offline" || !r.Quorum.Degraded || r.Quorum.Healthy != 2 {
		t.Fatalf("votes %+v quorum %+v", r.Votes, r.Quorum)
	}
	// …and when a goes away too, the master judges alone.
	c.e.NodeState(NodeStatus{Node: "a", Online: false})
	c.fc.set("m", probe.VoteFail)
	r = c.waitRow("web", "critical alone", func(r MonitorRow) bool { return r.Status == StatusCritical })
	if r.Quorum.QEff != 1 || r.Quorum.Healthy != 1 {
		t.Fatalf("quorum %+v", r.Quorum)
	}
}

func TestClusterDeadlineAndSilentAgent(t *testing.T) {
	c := newClusterEnv(t, clusterCfg(clusterMonitor("m", "a", "b"), "a", "b"))
	c.fc.mu.Lock()
	c.fc.silent["b"] = true
	c.fc.mu.Unlock()
	c.online("a", "b")
	r := c.waitRow("web", "judged at the deadline", func(r MonitorRow) bool { return len(r.Votes) == 3 && r.Votes[1].Vote == "ok" })
	if r.Votes[2].Vote != "absent" || r.Status != StatusOK {
		t.Fatalf("votes %+v status %s", r.Votes, r.Status)
	}
}

func TestClusterAgentsOnly(t *testing.T) {
	c := newClusterEnv(t, clusterCfg(clusterMonitor("a", "b"), "a", "b"))
	c.online("a", "b")
	c.fc.set("m", probe.VoteFail) // the master does not take part
	r := c.waitRow("web", "agents only", func(r MonitorRow) bool { return len(r.Votes) == 2 && r.Votes[0].Vote == "ok" })
	if r.Status != StatusOK {
		t.Fatalf("status %s votes %+v", r.Status, r.Votes)
	}
}

func TestClusterClockSkew(t *testing.T) {
	m := clusterMonitor("m", "a", "b")
	m.TimeSensitive = true
	c := newClusterEnv(t, clusterCfg(m, "a", "b"))
	c.online("b")
	c.e.NodeState(NodeStatus{Node: "a", Online: true, Skew: 5 * time.Second})
	c.fc.set("a", probe.VoteFail)
	r := c.waitRow("web", "skewed vote abstains", func(r MonitorRow) bool { return len(r.Votes) == 3 && r.Votes[1].Vote == "abstain" })
	if r.Votes[1].Reason != probe.ReasonClockSkew || r.Partial {
		t.Fatalf("votes %+v partial %v", r.Votes, r.Partial)
	}
}

func TestClusterRegionalEscalation(t *testing.T) {
	m := clusterMonitor("m", "a", "b")
	m.PartialEscalate = 300 * time.Millisecond
	c := newClusterEnv(t, clusterCfg(m, "a", "b"))
	c.online("a", "b")
	c.fc.set("a", probe.VoteFail)
	r := c.waitRow("web", "regional incident", func(r MonitorRow) bool { return r.RegionalID != 0 })
	inc, ok := c.sink.open(IncidentRegional, "web")
	if !ok || inc.Detail != "a" || !inc.State.Low || r.Status != StatusOK {
		t.Fatalf("regional %+v ok=%v status %s", inc, ok, r.Status)
	}
	c.fc.set("a", probe.VoteOK)
	c.waitRow("web", "regional resolved", func(r MonitorRow) bool { return r.RegionalID == 0 && !r.Partial })
	if _, ok := c.sink.open(IncidentRegional, "web"); ok {
		t.Fatal("regional incident still open")
	}
}

func TestClusterNodeOffline(t *testing.T) {
	cfg := clusterCfg(clusterMonitor("m", "a"), "a")
	cfg.Nodes[0].Timeout = 300 * time.Millisecond
	c := newClusterEnv(t, cfg)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := c.sink.open(IncidentSystem, nodeOfflineKey("a")); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no offline incident for a node that never connected")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.online("a")
	deadline = time.Now().Add(5 * time.Second)
	for {
		if _, ok := c.sink.open(IncidentSystem, nodeOfflineKey("a")); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("offline incident not resolved after reconnect")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ns := c.e.Nodes(); len(ns) != 1 || !ns[0].Online {
		t.Fatalf("nodes %+v", ns)
	}
}

func TestSingleNodeUnchanged(t *testing.T) {
	m := clusterMonitor()
	c := newClusterEnv(t, Config{Monitors: []MonitorSpec{m}})
	c.fc.set("", probe.VoteFail)
	r := c.waitRow("web", "single-node failure", func(r MonitorRow) bool { return r.Status == StatusCritical })
	if len(r.Votes) != 0 || r.Quorum != nil {
		t.Fatalf("single node rows carry no votes: %+v", r)
	}
	c.fc.mu.Lock()
	asked := len(c.fc.asked)
	c.fc.mu.Unlock()
	if asked != 0 {
		t.Fatal("single node must not ask agents")
	}
}

func TestProbeForAgentMode(t *testing.T) {
	c := newClusterEnv(t, Config{})
	c.fc.set("", probe.VoteFail)
	v := c.e.ProbeFor(context.Background(), "x", probe.Spec{Name: "x", Type: "TCP", Target: "10.0.0.9:22", Timeout: time.Second}, "")
	if v.Vote != probe.VoteFail || v.Reason != probe.ReasonTimeout || v.Excluded {
		t.Fatalf("%+v", v)
	}
}
