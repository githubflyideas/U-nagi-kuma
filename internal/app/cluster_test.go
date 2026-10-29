package app

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/hostclock"
	"github.com/githubflyideas/unagikuma/internal/netscope"
	"github.com/githubflyideas/unagikuma/internal/probe"
)

func TestAgentEndToEnd(t *testing.T) {
	a, _, _, _ := newApp(t, testConfig+`bj IN NODE "北京"
`)
	tok, err := a.Store().CreateAgentToken("bj")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET "+cluster.ConnectPath, a.Cluster())
	hs := &http.Server{Handler: mux}
	go hs.Serve(cluster.SniffListener(raw, a.PKI().TLSConfig(nil), time.Second))
	defer hs.Close()

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- a.Run(ctx) }()
	defer func() { cancel(); <-runDone }()

	var agentFails atomic.Bool
	ag, err := cluster.NewAgent(cluster.AgentOptions{
		Server: "https://" + raw.Addr().String(), Node: "bj", Token: tok, Fingerprint: a.PKI().Fingerprint(), Version: "e2e",
		MaxBackoff: 200 * time.Millisecond,
		Probe: func(_ context.Context, s probe.Spec) probe.Result {
			if agentFails.Load() {
				return probe.Result{Vote: probe.VoteFail, Reason: probe.ReasonConnRefused, Evidence: &probe.Evidence{}}
			}
			return probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Latency: time.Millisecond, Evidence: &probe.Evidence{}}
		},
		Engine: func(d *engine.Deps) {
			d.Gateways = func() ([]netscope.Gateway, error) { return nil, nil }
			d.HostClock = func() (hostclock.Reading, bool) { return hostclock.Reading{}, false }
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agentDone := make(chan struct{})
	go func() { ag.Run(ctx); close(agentDone) }()
	defer func() { cancel(); <-agentDone }()

	vote := func(r engine.MonitorRow, node string) string {
		for _, v := range r.Votes {
			if v.Node == node {
				return v.Vote
			}
		}
		return ""
	}
	waitFor(t, "both nodes vote ok", 10*time.Second, func() bool {
		r, _ := a.Engine().Row("web")
		return vote(r, "unagikuma") == "ok" && vote(r, "bj") == "ok" && r.Quorum != nil && r.Quorum.Kind == engine.VerdictOK
	})
	nodes := a.Engine().Nodes()
	if len(nodes) != 1 || !nodes[0].Online || nodes[0].Serial != a.Config().Serial || nodes[0].Version != "e2e" {
		t.Fatalf("nodes: %+v", nodes)
	}

	// Failing from the agent only is a partial failure, not an outage.
	agentFails.Store(true)
	waitFor(t, "partial verdict", 10*time.Second, func() bool {
		r, _ := a.Engine().Row("web")
		return vote(r, "bj") == "fail" && r.Quorum != nil && r.Quorum.Kind == engine.VerdictPartial && r.Status == "ok"
	})
}
