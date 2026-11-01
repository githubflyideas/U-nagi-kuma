package engine

import (
	"slices"
	"time"
)

// DiscoverySnapshot describes the last reconciliation of configured remote
// nodes, not live sessions or valid votes. The registry is the applied NODE
// configuration; no broadcast, network scan or peer self-enrollment occurs.
// Local node identity is available separately through Self().
type DiscoverySnapshot struct {
	Source      string    `json:"source"`
	Members     []string  `json:"members"`
	IntervalS   int64     `json:"interval_s"`
	LastRefresh time.Time `json:"last_refresh"`
	NextRefresh time.Time `json:"next_refresh"`
}

// Discovery returns a detached snapshot; heartbeat updates never alter it.
func (e *Engine) Discovery() DiscoverySnapshot {
	var out DiscoverySnapshot
	e.do(func() {
		out = e.discovery
		out.Members = slices.Clone(out.Members)
	})
	return out
}

// refreshDiscovery runs on the engine's existing timer. A delayed tick performs
// one reconciliation, not a burst of missed refreshes. Explicit configuration
// applies reconcile immediately so authorization removals are never delayed.
func (e *Engine) refreshDiscovery(now time.Time) {
	if !e.applied || now.Before(e.discovery.NextRefresh) {
		return
	}
	e.applyNodes(e.cfg.Nodes, now)
}

func (e *Engine) recordDiscovery(now time.Time) {
	members := make([]string, 0, len(e.nodes))
	for name := range e.nodes {
		members = append(members, name)
	}
	slices.Sort(members)
	interval := e.cfg.Tunables.Duration("cluster.discovery-interval")
	e.discovery = DiscoverySnapshot{
		Source: "configured_registry", Members: members, IntervalS: int64(interval / time.Second),
		LastRefresh: now, NextRefresh: now.Add(interval),
	}
}
