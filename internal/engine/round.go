package engine

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/githubflyideas/unagikuma/internal/netscope"
	"github.com/githubflyideas/unagikuma/internal/probe"
)

// roundResult is what a round goroutine reports back to the loop.
type roundResult struct {
	name    string
	round   string // cluster round id ("" = judged alone)
	defHash string
	start   time.Time
	end     time.Time
	due     time.Time
	res     probe.Result
	// gateLocal: the pre-HARD self-check found every recently healthy wan
	// target unreachable too — the failure is most likely ours.
	gateLocal bool
	gateObs   []Observation
	push      bool // synthesized from a push or a missed push deadline
}

// gateCache shares pre-HARD self-check outcomes between rounds.
type gateCache struct {
	mu    sync.Mutex
	at    time.Time
	local bool
}

// dispatchDue starts the rounds that are due, within the concurrency and
// packet-rate budgets.
func (e *Engine) dispatchDue(ctx context.Context, wg *sync.WaitGroup) {
	now := e.now()
	for _, name := range e.sched.PopDue(now) {
		m := e.monitors[name]
		rt := e.states[name]
		if m == nil || rt == nil || m.Paused || m.Push || rt.Running {
			continue
		}
		if !e.participates(m) {
			// Agents only: no local probe, no local budget.
			due := rt.Due
			if due.IsZero() {
				due = now
			}
			e.startRound(m, rt, now, due, false)
			continue
		}
		if d := e.pps.Delay(now); d > 0 {
			// Over the packet budget: start it a little later.
			e.sched.Set(name, now.Add(d))
			continue
		}
		select {
		case e.sem <- struct{}{}:
		default:
			// All workers busy: try again shortly. A round that cannot start
			// within its interval counts as dropped (overload detection).
			e.sched.Set(name, now.Add(50*time.Millisecond))
			if !rt.Due.IsZero() && now.Sub(rt.Due) > m.Interval {
				e.watchdog.RecordRound(now, true)
				e.stats.dropped.Add(1)
				rt.Due = now
			}
			continue
		}
		e.pps.Allow(now)
		due := rt.Due
		if due.IsZero() {
			due = now
		}
		rt.Running = true
		e.watchdog.RecordRound(now, false)
		e.stats.started.Add(1)
		e.stats.lagMicros.Add(max(0, now.Sub(due).Microseconds()))
		spec := m.Probe
		hardIfFail := rt.State.HardOnFail(max(1, m.Attempts))
		scope := rt.Scope
		domain := rt.Domain
		fam := rt.Family
		gateN := e.cfg.Tunables.Int("localnet.gate-hosts")
		var targets []string
		if hardIfFail && e.localnet.cfg.Enabled && scope != netscope.LAN {
			targets = e.localnet.GateTargets(famOr(fam), domain, gateN, now)
		}
		gateTimeout := e.cfg.Tunables.Duration("localnet.gate-timeout")
		gateTTL := e.cfg.Tunables.Duration("localnet.gate-cache")
		defHash, secretErr := m.DefHash, m.SecretError
		round := ""
		if clustered(m) {
			round = e.startRound(m, rt, now, due, true)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-e.sem }()
			start := e.now()
			var res probe.Result
			if secretErr != "" {
				res = probe.Result{Vote: probe.VoteAbstain, Reason: probe.ReasonSecretUnavailable, Detail: secretErr}
			} else {
				rctx, cancel := context.WithTimeout(ctx, spec.Timeout+e.cfg.Tunables.Duration("engine.round-grace"))
				res = e.deps.Probe(rctx, spec)
				cancel()
			}
			rr := roundResult{name: name, round: round, defHash: defHash, start: start, end: e.now(), due: due, res: res}
			if res.Vote == probe.VoteFail && probe.NetworkClass(res.Reason) && len(targets) > 0 {
				rr.gateLocal, rr.gateObs = e.runGate(ctx, targets, gateTimeout, gateTTL)
			}
			select {
			case e.results <- rr:
			case <-ctx.Done():
			}
		}()
	}
}

func famOr(f Family) Family {
	if f == 0 {
		return FamilyV4
	}
	return f
}

// HardOnFail reports whether a failing verdict in this state would make the
// monitor HARD CRITICAL (and open an incident).
func (s MonState) HardOnFail(attempts int) bool {
	if s.Avail == StatusCritical && s.Hard {
		return false // already HARD
	}
	if s.Avail == StatusCritical {
		return s.Attempt+1 >= attempts
	}
	return attempts <= 1
}

// runGate re-probes recently healthy wan targets. It reports local=true only
// when every one of them failed at the network level.
func (e *Engine) runGate(ctx context.Context, targets []string, timeout, ttl time.Duration) (bool, []Observation) {
	now := e.now()
	e.gate.mu.Lock()
	if !e.gate.at.IsZero() && now.Sub(e.gate.at) < ttl {
		local := e.gate.local
		e.gate.mu.Unlock()
		return local, nil
	}
	e.gate.mu.Unlock()

	type out struct {
		obs Observation
		ok  bool
	}
	ch := make(chan out, len(targets))
	for _, t := range targets {
		go func(t string) {
			spec, ok := recheckSpec(t, timeout)
			if !ok {
				ch <- out{}
				return
			}
			rctx, cancel := context.WithTimeout(ctx, timeout+time.Second)
			r := e.deps.Probe(rctx, spec)
			cancel()
			o := observationFromResult(e.now(), spec, r, netscope.WAN)
			ch <- out{obs: o, ok: true}
		}(t)
	}
	reachable, failed := 0, 0
	var obs []Observation
	for range targets {
		o := <-ch
		if !o.ok {
			continue
		}
		obs = append(obs, o.obs)
		switch {
		case o.obs.Reachable:
			reachable++
		case o.obs.NetworkFail:
			failed++
		}
	}
	local := reachable == 0 && failed > 0
	e.gate.mu.Lock()
	e.gate.at, e.gate.local = now, local
	e.gate.mu.Unlock()
	return local, obs
}

// recheckSpec turns a recheck target ("tcp:203.0.113.5:443",
// "icmp:2001:db8::1") into a cheap probe.
func recheckSpec(t string, timeout time.Duration) (probe.Spec, bool) {
	kind, addr, ok := strings.Cut(t, ":")
	if !ok {
		return probe.Spec{}, false
	}
	switch kind {
	case "tcp":
		return probe.Spec{Name: "gate", Type: "TCP", Target: addr, Timeout: timeout}, true
	case "icmp":
		return probe.Spec{Name: "gate", Type: "PING", Target: addr, Timeout: timeout, Opts: map[string]string{"count": "1"}}, true
	}
	return probe.Spec{}, false
}

// parsePeer parses a probe's peer ("ip:port", "[v6]:port" or a bare IP).
func parsePeer(s string) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap, nil
	}
	a, err := parseAddr(s)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(a, 0), nil
}

// recheckTarget derives a cheap recheck target from a probe's peer.
func recheckTarget(typ string, peer netip.AddrPort) string {
	if !peer.IsValid() {
		return ""
	}
	switch typ {
	case "HTTP", "TCP":
		return "tcp:" + peer.String()
	}
	return "icmp:" + peer.Addr().String()
}

// observationFromResult converts a probe result into detector input.
func observationFromResult(at time.Time, s probe.Spec, r probe.Result, scope netscope.Scope) Observation {
	host := targetHost(s)
	o := Observation{At: at, Scope: scope, NetworkFail: probe.NetworkClass(r.Reason), Reachable: probe.RemoteReachable(r)}
	addr, perr := netip.ParseAddr(strings.Trim(host, "[]"))
	o.ByName = perr != nil
	if r.Reason == probe.ReasonDNSResolve {
		o.DNSFail = true
	}
	var peer netip.AddrPort
	if r.Evidence != nil && r.Evidence.Peer != "" {
		if ap, err := parsePeer(r.Evidence.Peer); err == nil {
			peer, addr = ap, ap.Addr()
		}
	}
	if addr.IsValid() {
		addr = addr.Unmap()
		o.Family = FamilyOf(addr)
		o.Domain = FailureDomain(host, addr)
		o.Recheck = recheckTarget(s.Type, peer)
	} else {
		o.Domain = FailureDomain(host, netip.Addr{})
	}
	return o
}

// targetHost extracts the host part of a probe target.
func targetHost(s probe.Spec) string {
	if strings.EqualFold(s.Type, "EXEC") || strings.EqualFold(s.Type, "DOCKER") {
		return s.Opts["address"] // the target is a plugin or a container
	}
	t := s.Target
	if i := strings.Index(t, "://"); i >= 0 {
		t = t[i+3:]
		if j := strings.IndexAny(t, "/?#"); j >= 0 {
			t = t[:j]
		}
		if k := strings.LastIndex(t, "@"); k >= 0 {
			t = t[k+1:]
		}
	}
	if strings.HasPrefix(t, "[") {
		if j := strings.Index(t, "]"); j > 0 {
			return t[1:j]
		}
	}
	if strings.Count(t, ":") == 1 {
		t, _, _ = strings.Cut(t, ":")
	}
	return t
}

// checkAnchors probes the default gateway and explicit anchors once.
func (e *Engine) checkAnchors(ctx context.Context) {
	var timeout time.Duration
	var anchors []AnchorSpec
	var enabled bool
	var exit string
	var wanTrouble bool
	if !e.do(func() {
		timeout = e.cfg.Tunables.Duration("localnet.gate-timeout")
		enabled = e.localnet.cfg.Enabled
		exit = e.cfg.Tunables.Raw("localnet.exit-target")
		if !enabled || exit != e.exit.Addr {
			e.exit = exitState{}
		}
		wanTrouble = e.wanTrouble()
		for _, a := range e.anchors {
			anchors = append(anchors, a)
		}
	}) || !enabled {
		return
	}
	var results []AnchorResult
	gws, err := e.deps.Gateways()
	if err == nil {
		for _, g := range gws {
			fam := FamilyOf(g.Addr)
			ok, rtt := e.pingAnchor(ctx, g.Addr.String(), timeout)
			results = append(results, AnchorResult{At: e.now(), Kind: AnchorAutoGateway, Family: fam, Addr: g.Addr.String(), OK: ok, RTT: rtt, Container: e.deps.InContainer})
		}
	}
	for _, a := range anchors {
		ok, rtt := e.pingAnchor(ctx, a.Addr, timeout)
		results = append(results, AnchorResult{At: e.now(), Kind: a.Kind, Family: anchorFamily(a.Addr), Addr: a.Addr, OK: ok, RTT: rtt})
	}
	// The internet exit is pinged for the path view only; it is not an
	// anchor, so an exit that filters ping cannot hold real alerts. The
	// targets come first: while every internet target answers, the way out
	// evidently works and the exit is left alone; once one fails, the exit
	// tells a broken site from a broken way out.
	var ex exitState
	switch {
	case exit != "" && wanTrouble:
		ok, rtt := e.pingAnchor(ctx, exit, timeout)
		ex = exitState{Addr: exit, OK: ok, RTT: rtt, At: e.now()}
	case exit != "":
		ex = exitState{Addr: exit, OK: true, Idle: true}
	}
	e.do(func() {
		for _, r := range results {
			e.localnet.Anchor(r)
		}
		if ex.Addr != "" && ex.Addr == e.cfg.Tunables.Raw("localnet.exit-target") {
			e.exit = ex
		}
	})
}

type exitState struct {
	Addr string
	OK   bool
	RTT  time.Duration
	At   time.Time
	Idle bool // not pinged: every internet target answers
}

// wanTrouble reports whether an internet target is failing or being
// confirmed as failing (the exit is worth asking then).
func (e *Engine) wanTrouble() bool {
	for name, rt := range e.states {
		m := e.monitors[name]
		if m == nil || m.Paused || rt.Scope != netscope.WAN {
			continue
		}
		if rt.State.Avail == StatusCritical || rt.State.Attempt > 0 || (rt.State.Avail == StatusUnknown && rt.State.UnknownReason != UnknownLocalNetwork) {
			return true
		}
	}
	// An agent does not judge: its own last results say it.
	for _, rt := range e.agentRT {
		if rt.Scope == netscope.WAN && rt.LastResult.Vote == probe.VoteFail {
			return true
		}
	}
	return false
}

// pingAnchor checks one anchor: ICMP echo, or — when ICMP needs privileges
// we do not have — a TCP connection attempt, where a refusal also proves
// the host is there.
func (e *Engine) pingAnchor(ctx context.Context, addr string, timeout time.Duration) (bool, time.Duration) {
	r := e.deps.Probe(ctx, probe.Spec{Name: "anchor", Type: "PING", Target: addr, Timeout: timeout, Opts: map[string]string{"count": "1"}})
	if r.Vote == probe.VoteAbstain {
		host := addr
		if strings.Contains(addr, ":") {
			host = "[" + addr + "]"
		}
		r = e.deps.Probe(ctx, probe.Spec{Name: "anchor", Type: "TCP", Target: host + ":53", Timeout: timeout})
	}
	return probe.RemoteReachable(r), r.Latency
}
