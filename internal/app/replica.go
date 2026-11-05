package app

// Every server that joined a cluster is a replica of the master, like a
// secondary DNS server: it keeps a copy of the master's state (configuration,
// secrets, data) and, when it cannot reach the master for a while, checks,
// stores and alerts on its own with the master's configuration. The link
// only carries the copy and the votes; nothing is ever decided for a site
// that cannot be reached. Promotion to master stays a manual decision.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/githubflyideas/unagikuma/internal/backup"
	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/store"
)

const (
	replicaDir    = "replica"
	replicaConfig = "master-copy.bind" // the master's configuration, as last copied
	// replicaConfigOld is the name before the copy was renamed; kept only to
	// move an existing copy to replicaConfig on the first start after the change.
	replicaConfigOld = "master.bind"
)

// replicaState is what a replica runs while it cannot reach the master.
type replicaState struct {
	mu    sync.Mutex
	cfg   *config.Config // master configuration copy (nil: none yet)
	local bool           // checking on its own right now
	since time.Time      // when local checking started

	merging bool      // a merge push is running
	merged  time.Time // last merge push that went through
	copied  time.Time // when the copy was taken
}

// ReplicaStatus is shown on the node's own pages.
type ReplicaStatus struct {
	Replica  bool      `json:"replica"`
	Local    bool      `json:"local"`
	Since    time.Time `json:"since,omitzero"`
	Copied   time.Time `json:"copied,omitzero"`
	Monitors int       `json:"monitors"`
	Site     string    `json:"site,omitempty"`
}

// ReplicaStatus reports whether this server is a replica and whether it is
// checking on its own.
func (a *App) ReplicaStatus() ReplicaStatus {
	a.am.mu.Lock()
	member := a.am.member != nil
	a.am.mu.Unlock()
	site := a.site() // takes r.mu itself
	r := &a.replica
	r.mu.Lock()
	defer r.mu.Unlock()
	st := ReplicaStatus{Replica: member, Local: r.local, Since: r.since, Copied: r.copied, Site: site}
	if r.cfg != nil {
		st.Monitors = len(r.cfg.Monitors)
	}
	return st
}

// loadReplica reads the configuration copy kept from the last sync.
func (a *App) loadReplica() {
	p := a.dataPath(replicaDir, replicaConfig)
	if _, err := os.Stat(p); err != nil {
		if old := a.dataPath(replicaDir, replicaConfigOld); os.Rename(old, p) == nil {
			a.log.Info("moved the copy of the master's configuration to its new name", "file", replicaConfig)
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	_, cfg, diags := config.Load(p, b, a.validators())
	if cfg == nil || config.HasErrors(diags) {
		a.log.Warn("the copy of the master's configuration is not usable", "errors", len(diags))
		return
	}
	fi, _ := os.Stat(p)
	a.replica.mu.Lock()
	a.replica.cfg = cfg
	if fi != nil {
		a.replica.copied = fi.ModTime()
	}
	a.replica.mu.Unlock()
}

// keepReplicaCopy unpacks a fresh state archive: the master's configuration
// is kept for checking on its own, its secrets are re-encrypted into this
// server's secret store so its channels and checks keep working.
func (a *App) keepReplicaCopy(archive, key string) error {
	tmp, err := os.MkdirTemp(a.dataPath(standbyDir), ".unpack-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	cfgPath := filepath.Join(tmp, "monitors.bind")
	if _, err := backup.Restore(f, tmp, cfgPath, key, true); err != nil {
		return fmt.Errorf("unpack the master's state: %w", err)
	}
	if err := a.copySecrets(tmp); err != nil {
		a.log.Warn("could not copy the master's secrets", "err", err)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(a.dataPath(replicaDir), 0o700); err != nil {
		return err
	}
	dst := a.dataPath(replicaDir, replicaConfig)
	if err := os.WriteFile(dst+".tmp", b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(dst+".tmp", dst); err != nil {
		return err
	}
	a.loadReplica()
	a.replica.mu.Lock()
	local := a.replica.local
	a.replica.mu.Unlock()
	if local {
		a.Reapply()
	}
	return nil
}

func (a *App) copySecrets(dir string) error {
	kek, err := store.LoadOrCreateKey(dir)
	if err != nil {
		return err
	}
	st, err := store.Open(dir, store.Options{Log: a.log.With("component", "replica-copy"), Now: a.opts.Now})
	if err != nil {
		return err
	}
	defer st.Close()
	src := st.Secrets(kek)
	list, err := src.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, si := range list {
		v, err := src.Get(si.Name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if cur, err := a.secrets.Get(si.Name); err == nil && cur == v {
			continue
		}
		if err := a.secrets.Put(si.Name, v, si.Scopes); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// watchReplica switches checking on its own on when the master has been
// out of reach for cluster.replica-after, and off once it is back.
func (a *App) watchReplica() {
	a.am.mu.Lock()
	ag := a.am.agent
	a.am.mu.Unlock()
	r := &a.replica
	r.mu.Lock()
	want := false
	if ag != nil && r.cfg != nil {
		st := ag.Status()
		// The master's setting counts: it is the one the operator wrote.
		after := r.cfg.Tunables.Duration("cluster.replica-after")
		want = !st.Connected && a.now().Sub(st.Since) >= after
	}
	change := want != r.local
	if change {
		r.local = want
		r.since = time.Time{}
		if want {
			r.since = a.now()
		}
	}
	local := r.local
	r.mu.Unlock()
	if local {
		a.feedPeers()
	} else {
		a.maybePushMerge(change)
	}
	if !change {
		return
	}
	if want {
		a.log.Warn("the master is out of reach: this replica checks and alerts on its own", "site", a.site())
		_ = a.store.Audit(auditEntry("cluster.replica_local", "system", map[string]any{"site": a.site()}))
		a.Reapply()
		st := ag.Status()
		a.engine.MasterUnreachable(true, strings.TrimPrefix(ag.Server(), "https://"), st.Since)
		return
	}
	a.log.Info("the master is back: this replica hands checking back to it", "site", a.site())
	_ = a.store.Audit(auditEntry("cluster.replica_follow", "system", map[string]any{"site": a.site()}))
	a.engine.MasterUnreachable(false, "", time.Time{})
	a.Reapply()
}

// localEngineConfig is what a replica runs on its own: the master's
// monitors, channels and policies, judged by this node alone. Monitors
// pinned to other nodes only are left to them.
func (a *App) localEngineConfig() (engine.Config, bool) {
	r := &a.replica
	r.mu.Lock()
	cfg, local := r.cfg, r.local
	r.mu.Unlock()
	if !local || cfg == nil {
		return engine.Config{}, false
	}
	self := a.nodeName()
	ec := a.engineConfig(cfg)
	// The other nodes, the master among them, are judged by majority from
	// this node's peer checks (feedPeers); monitors are checked here only.
	ec.Nodes, ec.Self = nil, ""
	seenMaster := false
	for _, n := range cfg.Nodes {
		seenMaster = seenMaster || n.Name == cfg.MName
		if n.Name == self {
			continue
		}
		ec.Nodes = append(ec.Nodes, engine.NodeSpec{Name: n.Name, Title: n.Description, Region: n.Region, Policy: n.Policy, Timeout: n.NodeTimeout})
	}
	if !seenMaster && cfg.MName != self {
		ec.Nodes = append(ec.Nodes, engine.NodeSpec{Name: cfg.MName})
	}
	keep := ec.Monitors[:0]
	for _, m := range ec.Monitors {
		if len(m.Nodes) > 0 && !containsStr(m.Nodes, self) {
			continue
		}
		m.Nodes = nil
		keep = append(keep, m)
	}
	ec.Monitors = keep
	ec.Anchors, ec.LocalDetect = nil, true
	for _, n := range cfg.Nodes {
		if n.Name != self {
			continue
		}
		ec.LocalDetect = n.LocalDetect && !n.NoAnchors
		for _, x := range n.AnchorsLAN {
			ec.Anchors = append(ec.Anchors, engine.AnchorSpec{Kind: engine.AnchorLAN, Addr: x})
		}
		for _, x := range n.AnchorsWAN {
			ec.Anchors = append(ec.Anchors, engine.AnchorSpec{Kind: engine.AnchorWAN, Addr: x})
		}
	}
	return ec, true
}

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// nodeName is this server's node name in the cluster.
func (a *App) nodeName() string {
	a.am.mu.Lock()
	defer a.am.mu.Unlock()
	if a.am.member != nil {
		return a.am.member.Node
	}
	if c := a.applied.Load(); c != nil {
		return c.MName
	}
	return ""
}

// site names this server in messages: the node's title, or its name. A
// single machine has no site.
func (a *App) site() string {
	self := a.nodeName()
	var cfg *config.Config
	a.am.mu.Lock()
	member := a.am.member != nil
	a.am.mu.Unlock()
	if member {
		a.replica.mu.Lock()
		cfg = a.replica.cfg
		a.replica.mu.Unlock()
		if cfg == nil {
			return self
		}
	} else {
		cfg = a.applied.Load()
		if cfg == nil || len(cfg.Nodes) == 0 {
			return ""
		}
		hasRemote := false
		for _, n := range cfg.Nodes {
			hasRemote = hasRemote || n.Name != cfg.MName
		}
		if !hasRemote {
			return ""
		}
	}
	for _, n := range cfg.Nodes {
		if n.Name == self && n.Description != "" {
			return n.Description
		}
	}
	return self
}

// runReplica runs the replica watch while ctx lives.
func (a *App) runReplica(ctx context.Context) {
	a.every(ctx, 2*time.Second, a.watchReplica)
}
