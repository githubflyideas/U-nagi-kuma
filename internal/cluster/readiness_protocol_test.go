package cluster

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/tunables"
)

func readinessRawPeer(t *testing.T, c *clusterEnv) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := tls.Dial("tcp", strings.TrimPrefix(c.url, "https://"), &tls.Config{ServerName: AgentServerName, NextProtos: []string{"http/1.1"}, InsecureSkipVerify: true, VerifyConnection: pinnedVerifier(c.pki.Fingerprint())})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	req, _ := http.NewRequest("GET", c.url+ConnectPath, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", Upgrade)
	req.Header.Set("Authorization", "Bearer uka_bj")
	if err = req.Write(conn); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, req)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("upgrade: %v %v", resp, err)
	}
	if err = writeFrame(conn, msgHello, Hello{Proto: ProtoVersion, Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	f, err := readFrame(r, 1<<20)
	if err != nil || f.Type != msgWelcome {
		t.Fatalf("welcome: %v %v", f, err)
	}
	return conn, r
}

func TestReadinessReconnectLatestConfiguration(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer target.Close()
	c := newEnv(t)
	cfg := agentConfig(1)
	cfg["bj"].Monitors = []AgentMonitor{{Name: "web", DefHash: "h1", Probe: probe.Spec{Name: "web", Type: "HTTP", Target: target.URL, Timeout: time.Second}}}
	c.master.SetConfigs(cfg)
	a, _ := c.agent(func(o *AgentOptions) { o.Probe = probe.Run })
	waitFor(t, "initial ack", func() bool { st, _ := c.sink.last(); return st.Serial == 1 })
	c.master.mu.Lock()
	old := c.master.peers["bj"]
	c.master.mu.Unlock()
	old.close()
	waitFor(t, "offline", func() bool { st, _ := c.sink.last(); return !st.Online })
	latest := agentConfig(9)
	latest["bj"].Monitors = []AgentMonitor{{Name: "new", DefHash: "h9", Probe: probe.Spec{Name: "new", Type: "HTTP", Target: target.URL, Timeout: time.Second}}}
	c.master.SetConfigs(latest)
	waitFor(t, "latest config on reconnect", func() bool { st, _ := c.sink.last(); return st.Online && st.Serial == 9 && a.Status().Serial == 9 })
	if !c.master.Probe("bj", engine.ProbeRequest{Round: "reconnected", Monitor: "new", DefHash: "h9"}) {
		t.Fatal("probe not queued")
	}
	waitFor(t, "new monitor result", func() bool { v, ok := c.sink.vote("reconnected"); return ok && v.Vote == probe.VoteOK })
	if !c.master.Probe("bj", engine.ProbeRequest{Round: "removed", Monitor: "web", DefHash: "h1"}) {
		t.Fatal("probe not queued")
	}
	waitFor(t, "old monitor removed", func() bool {
		v, ok := c.sink.vote("removed")
		return ok && v.Vote == probe.VoteAbstain && v.Reason == probe.ReasonConfig
	})
}

func TestReadinessSilentPeerExpires(t *testing.T) {
	c := newEnv(t)
	_, _ = readinessRawPeer(t, c) // Deliberately never return pong or status.
	waitFor(t, "online", func() bool { st, ok := c.sink.last(); return ok && st.Online })
	start := time.Now()
	waitFor(t, "silent peer offline", func() bool { st, _ := c.sink.last(); return !st.Online })
	t.Logf("silent peer disconnected after %s (test ping=200ms; read deadline=3 intervals)", time.Since(start))
}

func TestReadinessConfigurationAckRequiresMatchingHash(t *testing.T) {
	c := newEnv(t)
	c.master.SetConfigs(agentConfig(1))
	conn, _ := readinessRawPeer(t, c)
	if err := writeFrame(conn, msgStatus, Status{Serial: 1, Hash: "wrong-hash", Health: &engine.Health{}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "status processed", func() bool { st, _ := c.sink.last(); return st.Health != nil })
	st, _ := c.sink.last()
	if st.Serial == 1 {
		t.Fatal("mismatched configuration hash acknowledged as synchronized serial 1")
	}
	if err := writeFrame(conn, msgStatus, Status{Serial: 99, Hash: "cfg1", Health: &engine.Health{}}); err != nil {
		t.Fatal(err)
	}
	// A pong acts as a processing barrier for the preceding status frame.
	if err := writeFrame(conn, msgPong, Ping{Seq: 1}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second status processed", func() bool { st, _ := c.sink.last(); return st.RTT > 0 })
	st, _ = c.sink.last()
	if st.Serial != 0 {
		t.Fatalf("incorrect serial acknowledged with matching hash: %d", st.Serial)
	}
	if err := writeFrame(conn, msgStatus, Status{Serial: 1, Hash: "cfg1"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "matching config acknowledged", func() bool { st, _ := c.sink.last(); return st.Serial == 1 })
}

type readinessRemote struct{ requests chan engine.ProbeRequest }

func (r *readinessRemote) Probe(_ string, p engine.ProbeRequest) bool { r.requests <- p; return true }

func TestReadinessDuplicateAndStaleVotes(t *testing.T) {
	ts, err := tunables.New(map[string]string{"engine.first-round-delay": "0s", "engine.round-grace": "0s", "cluster.vote-grace": "300ms", "localnet.enabled": "off"})
	if err != nil {
		t.Fatal(err)
	}
	remote := &readinessRemote{requests: make(chan engine.ProbeRequest, 16)}
	e := engine.New(engine.Deps{Remote: remote, Sink: newAgentSink(newEnv(t).master.o.Log)})
	e.Apply(engine.Config{Self: "master", Tunables: ts, Nodes: []engine.NodeSpec{{Name: "bj", Probe: true, Timeout: time.Hour}}, Monitors: []engine.MonitorSpec{{Name: "web", DefHash: "current", Probe: probe.Spec{Name: "web", Type: "TCP", Target: "127.0.0.1:1", Timeout: time.Second}, Nodes: []string{"bj", "other"}, Interval: time.Hour, Attempts: 1, RecoverAttempts: 1, Quorum: engine.QuorumRule{Spec: "majority"}}}})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	e.NodeState(engine.NodeStatus{Node: "bj", Online: true, Since: time.Now()})
	var req engine.ProbeRequest
	select {
	case req = <-remote.requests:
	case <-time.After(5 * time.Second):
		t.Fatal("no round")
	}
	vote := engine.RemoteVote{Node: "bj", Round: req.Round, Monitor: req.Monitor, DefHash: req.DefHash, Vote: probe.VoteOK, Reason: probe.ReasonOK}
	// Unsolicited/stale rounds and wrong monitor identities cannot poison the active round.
	wrong := vote
	wrong.Round = "unknown"
	wrong.Vote = probe.VoteFail
	e.RemoteVote(wrong)
	wrong = vote
	wrong.Monitor = "wrong"
	wrong.Vote = probe.VoteFail
	e.RemoteVote(wrong)
	e.RemoteVote(vote)
	waitFor(t, "valid vote wins", func() bool { r, _ := e.Row("web"); return len(r.Votes) == 2 && r.Votes[0].Vote == "ok" })
	vote.Vote = probe.VoteFail
	e.RemoteVote(vote) // conflicting duplicate after completion
	time.Sleep(100 * time.Millisecond)
	row, _ := e.Row("web")
	if len(row.Votes) != 2 || row.Votes[0].Vote != "ok" {
		t.Fatalf("duplicate rewrote verdict: %+v", row)
	}
}
