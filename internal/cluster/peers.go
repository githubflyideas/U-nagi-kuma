package cluster

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

// Every node checks the others it was told about, so the cluster can judge
// a node by majority: a node is down only when more than half of the nodes
// cannot reach it. A check asks the peer's ReachPath, which also returns the
// peer's own view; it only proves reachability, so the peer's certificate
// is not verified (nothing secret is exchanged).

// PeerView is what this node knows about one peer.
type PeerView struct {
	Name  string          `json:"name"`
	Addr  string          `json:"addr"`
	OK    bool            `json:"ok"`
	At    time.Time       `json:"at"`
	Error string          `json:"error,omitempty"`
	Reach map[string]bool `json:"reach,omitempty"` // the peer's own view
}

type peerSet struct {
	mu          sync.Mutex
	master      string
	views       map[string]*PeerView
	masterReach map[string]bool // the master's own view, as last fetched
}

func (a *Agent) setPeers(p Peers) {
	ps := &a.peers
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.views == nil {
		ps.views = map[string]*PeerView{}
	}
	ps.master = p.Master
	keep := map[string]bool{}
	for _, n := range p.Nodes {
		if n.Name == a.o.Node {
			continue
		}
		keep[n.Name] = true
		if v := ps.views[n.Name]; v != nil {
			v.Addr = n.Addr
		} else {
			ps.views[n.Name] = &PeerView{Name: n.Name, Addr: n.Addr}
		}
	}
	for n := range ps.views {
		if !keep[n] {
			delete(ps.views, n)
		}
	}
}

// Peers returns this node's view of the other nodes, the master first.
// The master's entry reflects the agent connection.
func (a *Agent) Peers() []PeerView {
	st := a.Status()
	ps := &a.peers
	ps.mu.Lock()
	out := make([]PeerView, 0, len(ps.views)+1)
	master := ps.master
	for _, v := range ps.views {
		c := *v
		out = append(out, c)
	}
	ps.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if master != "" {
		mv := PeerView{Name: master, Addr: hostOf(a.Server()), OK: st.Connected, At: time.Now()}
		if mr, ok := a.masterView(); ok {
			mv.Reach = mr
		}
		out = append([]PeerView{mv}, out...)
	}
	return out
}

// masterView is the master's own reach, as last fetched.
func (a *Agent) masterView() (map[string]bool, bool) {
	a.peers.mu.Lock()
	defer a.peers.mu.Unlock()
	return a.peers.masterReach, a.peers.masterReach != nil
}

// Reach is which nodes this one reaches: the master (connected) and every
// peer that answered its last check.
func (a *Agent) Reach() map[string]bool {
	out := map[string]bool{}
	for _, v := range a.Peers() {
		out[v.Name] = v.OK
	}
	return out
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Host
	}
	return u
}

// checkPeers runs a check of every peer each interval while ctx lives.
func (a *Agent) checkPeers(ctx context.Context) {
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // reachability only; see the package comment
	}}
	for {
		a.mu.Lock()
		every := a.ts.Duration("cluster.peer-interval")
		a.mu.Unlock()
		a.peers.mu.Lock()
		todo := make([]PeerView, 0, len(a.peers.views))
		for _, v := range a.peers.views {
			todo = append(todo, *v)
		}
		a.peers.mu.Unlock()
		var wg sync.WaitGroup
		for _, v := range todo {
			wg.Add(1)
			go func(v PeerView) {
				defer wg.Done()
				res := a.checkPeer(ctx, client, v.Addr)
				a.peers.mu.Lock()
				if cur := a.peers.views[v.Name]; cur != nil && cur.Addr == v.Addr {
					cur.OK, cur.At, cur.Error, cur.Reach = res.OK, res.At, res.Error, res.Reach
				}
				a.peers.mu.Unlock()
			}(v)
		}
		// The master's own view, when it is reachable on the address in use.
		if a.Status().Connected {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := a.checkPeer(ctx, client, hostOf(a.Server()))
				if res.OK && res.Reach != nil {
					a.peers.mu.Lock()
					a.peers.masterReach = res.Reach
					a.peers.mu.Unlock()
				}
			}()
		}
		wg.Wait()
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

func (a *Agent) checkPeer(ctx context.Context, c *http.Client, addr string) PeerView {
	v := PeerView{Addr: addr, At: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+ReachPath, nil)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	resp, err := c.Do(req)
	if err != nil {
		v.Error = err.Error()
		return v
	}
	defer resp.Body.Close()
	var rv ReachView
	if resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&rv) == nil {
		v.OK, v.Reach = true, rv.Reach
		return v
	}
	// It answered, so it is reachable, even if it is too old to tell more.
	v.OK = true
	return v
}

// ServeReach answers a peer's check with this node's name and view.
func ServeReach(w http.ResponseWriter, node string, reach map[string]bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(ReachView{Node: node, Reach: reach})
}
