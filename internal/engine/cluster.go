package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/netscope"
	"github.com/githubflyideas/unagikuma/internal/probe"
)

// NodeSpec is a remote probing node (an agent) of the cluster.
type NodeSpec struct {
	Name    string
	Title   string
	Region  string
	Probe   bool          // takes part in rounds
	Policy  string        // policy of its system incidents ("" = system)
	Timeout time.Duration // disconnected this long opens an offline incident
}

// Remote sends probe requests to agents. Probe must not block; it reports
// false when the node is not connected.
type Remote interface {
	Probe(node string, req ProbeRequest) bool
}

// ProbeRequest asks an agent to run one round of a monitor. The agent
// already holds the monitor's definition; DefHash proves it is the same.
type ProbeRequest struct {
	Round   string `json:"round"`
	Monitor string `json:"monitor"`
	DefHash string `json:"def_hash"`
}

// RemoteVote is an agent's judged answer to a ProbeRequest.
type RemoteVote struct {
	Node     string       `json:"node"`
	Round    string       `json:"round"`
	Monitor  string       `json:"monitor"`
	DefHash  string       `json:"def_hash"`
	Result   probe.Result `json:"result"`
	Vote     probe.Vote   `json:"vote"`
	Reason   string       `json:"reason"`
	Excluded bool         `json:"excluded,omitempty"`
}

// NodeStatus is what the transport knows about an agent.
type NodeStatus struct {
	Node     string        `json:"node"`
	Online   bool          `json:"online"`
	Since    time.Time     `json:"since"`     // when Online last changed
	LastSeen time.Time     `json:"last_seen"` // last message received
	Skew     time.Duration `json:"skew"`      // agent clock minus ours
	RTT      time.Duration `json:"rtt"`
	Version  string        `json:"version,omitempty"`
	Platform string        `json:"platform,omitempty"`
	Serial   uint32        `json:"serial"` // configuration serial the agent runs
	Addr     string        `json:"addr,omitempty"`
	Health   *Health       `json:"health,omitempty"` // the agent's view of its own network
	// Scripts are the node's check scripts, for EXEC monitors.
	Scripts []probe.Script `json:"scripts,omitempty"`
	// Unsupported are monitors this node is meant to check but cannot:
	// it runs an older release without their check type.
	Unsupported []string `json:"unsupported,omitempty"`
}

// VoteView is one node's latest vote on a monitor, for the UI.
type VoteView struct {
	Node      string  `json:"node"`
	Vote      string  `json:"vote"` // ok | fail | abstain | absent | offline
	Reason    string  `json:"reason,omitempty"`
	Detail    string  `json:"detail,omitempty"`
	LatencyMS float64 `json:"latency_ms,omitempty"`
	Excluded  bool    `json:"excluded,omitempty"`
}

// nodeRuntime is the engine's view of one agent.
type nodeRuntime struct {
	status       NodeStatus
	offlineSince time.Time // zero while online
}

// pendingRound collects the votes of one cluster round.
type pendingRound struct {
	id       string
	monitor  string
	defHash  string
	due      time.Time
	deadline time.Time
	votes    map[string]NodeVote
	results  map[string]probe.Result
	waiting  map[string]bool
}

// IncidentRegional is the low-priority incident of a monitor that keeps
// failing from some nodes while the others reach it.
const IncidentRegional = "regional"

// clustered reports whether a monitor is judged by more than this node.
func clustered(m *MonitorSpec) bool { return len(m.Nodes) > 0 }

func (e *Engine) participates(m *MonitorSpec) bool {
	if !clustered(m) {
		return true
	}
	for _, n := range m.Nodes {
		if n == e.self {
			return true
		}
	}
	return false
}

// startRound opens a cluster round: remote participants get a probe
// request, nodes that are not connected count as offline. The local probe
// (when this node takes part) reports through onLocalVote.
func (e *Engine) startRound(m *MonitorSpec, rt *monRuntime, now, due time.Time, local bool) string {
	ts := e.cfg.Tunables
	pr := &pendingRound{
		id: newID(), monitor: m.Name, defHash: m.DefHash, due: due,
		deadline: now.Add(m.Probe.Timeout + ts.Duration("engine.round-grace") + ts.Duration("cluster.vote-grace")),
		votes:    map[string]NodeVote{}, results: map[string]probe.Result{}, waiting: map[string]bool{},
	}
	for _, n := range m.Nodes {
		if n == e.self {
			if local {
				pr.waiting[n] = true
			}
			continue
		}
		nr := e.nodes[n]
		online := nr != nil && nr.status.Online
		if !online || e.deps.Remote == nil || !e.deps.Remote.Probe(n, ProbeRequest{Round: pr.id, Monitor: m.Name, DefHash: m.DefHash}) {
			pr.votes[n] = NodeVote{Node: n, Online: false}
			continue
		}
		pr.waiting[n] = true
	}
	e.rounds[pr.id] = pr
	rt.Running = true
	if len(pr.waiting) == 0 {
		e.finishRound(pr)
	}
	return pr.id
}

// onLocalVote adds this node's result to its cluster round.
func (e *Engine) onLocalVote(rr roundResult) {
	pr := e.rounds[rr.round]
	if pr == nil {
		return // the round already ended at its deadline
	}
	m, rt := e.monitors[pr.monitor], e.states[pr.monitor]
	if m == nil || rt == nil || m.DefHash != pr.defHash {
		e.dropRound(pr)
		return
	}
	pr.votes[e.self] = e.judgeLocal(rt, m.Scope, m.Probe, rr)
	pr.results[e.self] = rr.res
	delete(pr.waiting, e.self)
	if len(pr.waiting) == 0 {
		e.finishRound(pr)
	}
}

// RemoteVote hands an agent's vote to the engine (any goroutine).
func (e *Engine) RemoteVote(v RemoteVote) {
	select {
	case e.votes <- v:
	default:
		e.log.Warn("vote queue full, vote dropped", "node", v.Node, "monitor", v.Monitor)
	}
}

func (e *Engine) onRemoteVote(v RemoteVote) {
	pr := e.rounds[v.Round]
	if pr == nil || pr.monitor != v.Monitor || !pr.waiting[v.Node] {
		return // late, unknown or duplicate: history only
	}
	delete(pr.waiting, v.Node)
	if v.DefHash != pr.defHash {
		// Not yet synced to this definition: counts as no answer.
		pr.votes[v.Node] = NodeVote{Node: v.Node, Online: true, Present: false}
	} else {
		nv := NodeVote{Node: v.Node, Online: true, Present: true, Vote: v.Vote, Reason: v.Reason, Excluded: v.Excluded}
		if m := e.monitors[pr.monitor]; m != nil && m.TimeSensitive && nv.Vote != probe.VoteAbstain {
			if nr := e.nodes[v.Node]; nr != nil && absDur(nr.status.Skew) > e.cfg.Tunables.Duration("cluster.clock-skew") {
				nv.Vote, nv.Reason = probe.VoteAbstain, probe.ReasonClockSkew
			}
		}
		pr.votes[v.Node] = nv
		pr.results[v.Node] = v.Result
	}
	if len(pr.waiting) == 0 {
		e.finishRound(pr)
	}
}

// finishRound judges a round with the votes it has; participants that did
// not answer count as absent.
func (e *Engine) finishRound(pr *pendingRound) {
	delete(e.rounds, pr.id)
	m, rt := e.monitors[pr.monitor], e.states[pr.monitor]
	if rt != nil {
		rt.Running = false
	}
	if m == nil || rt == nil || m.DefHash != pr.defHash {
		return
	}
	votes := make([]NodeVote, 0, len(m.Nodes))
	for _, n := range m.Nodes {
		nv, ok := pr.votes[n]
		if !ok {
			nv = NodeVote{Node: n, Online: true, Present: false}
		}
		votes = append(votes, nv)
	}
	e.judge(m, rt, votes, pr.results, e.now(), pr.due, false)
}

func (e *Engine) dropRound(pr *pendingRound) {
	delete(e.rounds, pr.id)
	if rt := e.states[pr.monitor]; rt != nil {
		rt.Running = false
	}
}

// expireRounds ends rounds whose deadline passed.
func (e *Engine) expireRounds(now time.Time) {
	for _, pr := range e.rounds {
		if !now.Before(pr.deadline) {
			e.finishRound(pr)
		}
	}
}

// NodeState records an agent's connection state (any goroutine).
func (e *Engine) NodeState(s NodeStatus) {
	e.do(func() { e.onNodeState(s) })
}

func (e *Engine) onNodeState(s NodeStatus) {
	nr := e.nodes[s.Node]
	if nr == nil {
		return // not a configured node
	}
	now := e.now()
	was := nr.status.Online
	nr.status = s
	switch {
	case s.Online && !was:
		nr.offlineSince = time.Time{}
		e.event(Event{Type: EventSystem, Severity: "info", Message: "node " + s.Node + " connected", Data: map[string]any{"msg": "node_connected", "node": s.Node, "version": s.Version}})
		e.resolveSystem(nodeOfflineKey(s.Node), now, e.text("node_back", s.Node))
	case !s.Online && was:
		nr.offlineSince = now
		e.event(Event{Type: EventSystem, Severity: "warning", Message: "node " + s.Node + " disconnected", Data: map[string]any{"msg": "node_disconnected", "node": s.Node}})
	}
}

func nodeOfflineKey(node string) string { return SystemNodeOffline + ":" + node }

// SystemNodeOffline is the system incident of an agent that stays away.
const SystemNodeOffline = "node_offline"

// applyNodes keeps the node table in line with the configuration. A node
// counts as disconnected from the moment it is configured, so one that never
// connects is reported after its timeout too.
func (e *Engine) applyNodes(specs []NodeSpec, now time.Time) {
	keep := map[string]bool{}
	for _, n := range specs {
		keep[n.Name] = true
		if e.nodes[n.Name] == nil {
			e.nodes[n.Name] = &nodeRuntime{status: NodeStatus{Node: n.Name, Since: now}, offlineSince: now}
		}
	}
	for name := range e.nodes {
		if !keep[name] {
			delete(e.nodes, name)
			e.resolveSystem(nodeOfflineKey(name), now, "")
		}
	}
}

// checkNodes opens offline incidents for agents away longer than their
// timeout.
func (e *Engine) checkNodes(now time.Time) {
	for _, n := range e.cfg.Nodes {
		nr := e.nodes[n.Name]
		if nr == nil || nr.status.Online || nr.offlineSince.IsZero() {
			continue
		}
		timeout := n.Timeout
		if timeout <= 0 {
			timeout = e.cfg.Tunables.Duration("cluster.node-timeout")
		}
		if now.Sub(nr.offlineSince) < timeout {
			continue
		}
		key := nodeOfflineKey(n.Name)
		if inc := e.system[key]; inc != nil && inc.State.Open() {
			continue
		}
		title := n.Name
		if n.Title != "" {
			title = n.Title + " (" + n.Name + ")"
		}
		pol := n.Policy
		if pol == "" {
			pol = "system"
		}
		e.openSystemAs(key, pol, e.text("node_offline_title", title), e.text("node_offline_detail", nr.offlineSince), now)
	}
}

// Nodes returns the configured agents with their latest state.
func (e *Engine) Nodes() []NodeStatus {
	var out []NodeStatus
	e.do(func() {
		for _, n := range e.cfg.Nodes {
			if nr := e.nodes[n.Name]; nr != nil {
				out = append(out, nr.status)
			}
		}
	})
	return out
}

// Self is the name this node votes under.
func (e *Engine) Self() string {
	var s string
	e.do(func() { s = e.self })
	return s
}

// trackPartial turns a lasting partial failure — the same healthy nodes
// failing while others reach the target — into a low-priority regional
// incident with a single notification. It is often a real problem for users
// in that region, so it should not stay silent forever, but it is not an
// outage either.
func (e *Engine) trackPartial(m *MonitorSpec, rt *monRuntime, v Verdict, now time.Time) {
	if v.Kind != VerdictPartial || m.PartialEscalate <= 0 {
		rt.PartialKey, rt.PartialSince = "", time.Time{}
		if inc := e.regional[m.Name]; inc != nil && inc.State.Open() {
			np := e.regionalPolicy(inc)
			Recover(&inc.State, now, np)
			e.saveIncident(inc)
			e.event(Event{Type: EventIncident, Severity: "info", Monitor: m.Name, Message: "regional incident resolved", Data: map[string]any{"msg": "regional_resolved", "incident_id": inc.ID}})
		}
		return
	}
	failing := append([]string(nil), v.FailingNodes...)
	sort.Strings(failing)
	key := strings.Join(failing, ",")
	if key != rt.PartialKey {
		rt.PartialKey, rt.PartialSince = key, now
	}
	if inc := e.regional[m.Name]; inc != nil && inc.State.Open() {
		if inc.Detail != key {
			inc.Detail = key
			e.saveIncident(inc)
		}
		return
	}
	if now.Sub(rt.PartialSince) < m.PartialEscalate {
		return
	}
	pol := e.policy(m.Policy)
	inc := &Incident{Monitor: m.Name, Kind: IncidentRegional, Policy: pol.Name, Title: m.Title, Reason: "regional", Detail: key,
		State: IncidentState{Low: true, OpenedAt: rt.PartialSince, HardAt: now}}
	notices, _ := Advance(&inc.State, now, e.regionalPolicy(inc))
	e.saveIncident(inc)
	e.regional[m.Name] = inc
	e.incidents[inc.ID] = inc
	e.event(Event{Type: EventIncident, Severity: "warning", Monitor: m.Name, Message: "regional incident opened",
		Data: map[string]any{"msg": "regional_opened", "incident_id": inc.ID, "fail_nodes": failing, "since": rt.PartialSince}})
	e.notices(inc, notices, now)
}

// regionalPolicy is the monitor's policy limited to one notification and no
// recovery notice.
func (e *Engine) regionalPolicy(inc *Incident) NotifyPolicy {
	np := e.policy(inc.Policy).Notify
	np.Max, np.Recovery, np.Repeat = 1, false, 0
	return np
}

// voteViews renders a round's votes for the UI.
func (e *Engine) voteViews(votes []NodeVote, results map[string]probe.Result, at time.Time) []VoteView {
	out := make([]VoteView, 0, len(votes))
	for _, nv := range votes {
		vv := VoteView{Node: nv.Node, Reason: nv.Reason, Excluded: nv.Excluded}
		switch {
		case !nv.Online:
			vv.Vote = "offline"
		case !nv.Present:
			vv.Vote = "absent"
		default:
			vv.Vote = string(nv.Vote)
		}
		if r, ok := results[nv.Node]; ok {
			vv.Detail = r.Detail
			if r.Vote == probe.VoteOK {
				vv.LatencyMS = ms(r.Latency)
			}
		}
		out = append(out, vv)
	}
	return out
}

// AgentVote is an agent's judged answer to one probe request.
type AgentVote struct {
	Result   probe.Result
	Vote     probe.Vote
	Reason   string
	Excluded bool
}

// ProbeFor runs one probe the master asked for and judges it with this
// node's own health: an agent runs an engine without monitors only for its
// local network detector and suspend watchdog. Safe for any goroutine.
func (e *Engine) ProbeFor(ctx context.Context, name string, spec probe.Spec, scope netscope.Scope) AgentVote {
	start := e.now()
	res := e.deps.Probe(ctx, spec)
	end := e.now()
	out := AgentVote{Result: res, Vote: res.Vote, Reason: res.Reason}
	e.do(func() {
		rt := e.agentRT[name]
		if rt == nil {
			rt = &monRuntime{}
			e.agentRT[name] = rt
		}
		nv := e.judgeLocal(rt, scope, spec, roundResult{name: name, start: start, end: end, res: res})
		rt.LastResult = res // the agent's own internet-exit check reads it
		out.Vote, out.Reason, out.Excluded = nv.Vote, nv.Reason, nv.Excluded
	})
	return out
}

// ForgetAgentMonitors drops the per-monitor caches of an agent for monitors
// it no longer runs.
func (e *Engine) ForgetAgentMonitors(keep map[string]bool) {
	e.do(func() {
		for n := range e.agentRT {
			if !keep[n] {
				delete(e.agentRT, n)
			}
		}
	})
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
