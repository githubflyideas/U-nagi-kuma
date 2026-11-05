package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
)

// FullReachView is what this node answers on ReachPath: name, reach, term
// and role, so peers can detect a concurrent promote.
func (a *App) FullReachView() cluster.ReachView {
	node, reach := a.ReachView()
	v := cluster.ReachView{Node: node, Reach: reach}
	if a.isMember() {
		v.Role = "replica"
		a.am.mu.Lock()
		ag := a.am.agent
		a.am.mu.Unlock()
		if ag != nil {
			v.Term = ag.Term()
		}
		return v
	}
	if a.SupersededBy() != nil {
		return v
	}
	v.Role, v.Term = "master", a.Term()
	return v
}

// watchForeignMasters looks at peer Reach answers for another master. A
// higher term (or the same term with a lexicographically smaller name)
// wins: this server steps down. Equal-or-foreign masters also raise a
// CRITICAL "two masters" notice so the split is not silent.
func (a *App) watchForeignMasters() {
	if a.isMember() || a.SupersededBy() != nil {
		return
	}
	self := a.nodeName()
	myTerm := a.Term()
	for _, addr := range a.peerListenAddrs() {
		rv, ok := probeReach(addr)
		if !ok || rv.Role != "master" || rv.Node == "" || rv.Node == self {
			continue
		}
		switch {
		case rv.Term > myTerm:
			a.log.Warn("another master has a higher term; stepping down", "peer", rv.Node, "term", rv.Term, "addr", addr)
			a.engine.TwoMasters(rv.Node, rv.Term)
			a.supersede(rv.Term, rv.Node)
			return
		case rv.Term == myTerm && rv.Node < self:
			a.log.Warn("another master has the same term and wins the name tie-break; stepping down", "peer", rv.Node, "term", rv.Term, "addr", addr)
			a.engine.TwoMasters(rv.Node, rv.Term)
			// Bump past the tie so agents prefer the winner once it is the only master.
			a.supersede(rv.Term+1, rv.Node)
			return
		case rv.Term == myTerm && rv.Node > self:
			a.engine.TwoMasters(rv.Node, rv.Term)
		}
	}
}

// peerListenAddrs lists places other nodes of this cluster may be listening:
// NODE standby= URLs and the previous-master list left by a promote.
func (a *App) peerListenAddrs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return
		}
		if seen[u.Host] {
			return
		}
		seen[u.Host] = true
		out = append(out, u.Host)
	}
	if c := a.applied.Load(); c != nil {
		self := c.MName
		for _, n := range c.Nodes {
			if n.Name == self {
				continue
			}
			add(n.Standby)
		}
	}
	if b, err := os.ReadFile(a.dataPath(previousFile)); err == nil {
		var prev []string
		if json.Unmarshal(b, &prev) == nil {
			for _, s := range prev {
				add(s)
			}
		}
	}
	return out
}

func probeReach(addr string) (cluster.ReachView, bool) {
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+cluster.ReachPath, nil)
	if err != nil {
		return cluster.ReachView{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return cluster.ReachView{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return cluster.ReachView{}, false
	}
	var rv cluster.ReachView
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&rv) != nil {
		return cluster.ReachView{}, false
	}
	return rv, true
}

// probeForeignMastersAtStart runs once before system notices fire for a
// returning old master: if a peer already has a newer term, step down
// immediately so this node does not briefly act as master (BUG-15).
func (a *App) probeForeignMastersAtStart() {
	if a.isMember() || a.SupersededBy() != nil {
		return
	}
	if len(a.peerListenAddrs()) == 0 {
		return
	}
	a.watchForeignMasters()
}
