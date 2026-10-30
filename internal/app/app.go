// Package app wires unagikuma together: configuration file, store, engine,
// notification sender, heartbeat and housekeeping. The HTTP server talks
// to it; cmd/unagikuma creates and runs it.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/directory"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/license"
	"github.com/githubflyideas/unagikuma/internal/netscope"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/store"
	"github.com/githubflyideas/unagikuma/internal/tunables"
)

// Options configure the application.
type Options struct {
	// HandoverWait is how long a master keeps serving its state after
	// handing over before it steps down (default 20s; tests).
	HandoverWait time.Duration
	// LicenseRing replaces the release's licence keys (tests).
	LicenseRing  *license.Keyring
	DataDir      string
	ConfigPath   string // default <DataDir>/monitors.bind
	Instance     string
	BaseURL      string
	AllowEnv     []string
	HeartbeatURL string // from the command line; the settings page can set one too
	Location     *time.Location
	Log          *slog.Logger
	Version      string
	// Now and Probe are for tests.
	Now   func() time.Time
	Probe func(context.Context, probe.Spec) probe.Result
}

// App is a running unagikuma instance.
type App struct {
	opts      Options
	log       *slog.Logger
	store     *store.Store
	secrets   *store.Secrets
	license   *license.Checker
	engine    *engine.Engine
	sender    *notify.Sender
	cfg       *configManager
	heartbeat *engine.Heartbeat
	started   time.Time

	pki    *cluster.PKI
	master *cluster.Master
	am     agentMode // membership of a cluster this server joined
	// applied is the configuration the engine runs, readable without the
	// configuration lock (agent sessions consult it while a new one is
	// being applied).
	applied atomic.Pointer[config.Config]
	// dir is the contact directory the channels are checked against.
	dir       atomic.Pointer[directory.Directory]
	dirReload atomic.Bool
	// applyMu orders configuration applies with directory re-applies.
	applyMu sync.Mutex

	pubMu   sync.RWMutex
	publish func(typ string, v any)

	restart      chan struct{}
	restartOnce  sync.Once
	stateBusy    atomic.Bool
	stateRetries atomic.Int32 // failed state copies in a row
}

// New opens the store, loads the configuration and prepares the engine.
func New(o Options) (*App, error) {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.ConfigPath == "" {
		o.ConfigPath = filepath.Join(o.DataDir, "monitors.bind")
	}
	if o.Probe == nil {
		o.Probe = probe.Run
	}
	a := &App{opts: o, log: o.Log, started: o.Now(), publish: func(string, any) {}, restart: make(chan struct{})}
	if promoted, err := applyPromote(o.DataDir, o.ConfigPath); err != nil {
		return nil, fmt.Errorf("taking over as master: %w", err)
	} else if promoted {
		o.Log.Warn("this server took over as master with the state of the previous one")
	}
	st, err := store.Open(o.DataDir, store.Options{Log: o.Log, Now: o.Now})
	if err != nil {
		return nil, err
	}
	a.store = st
	kek, err := store.LoadOrCreateKey(o.DataDir)
	if err != nil {
		st.Close()
		return nil, err
	}
	a.secrets = st.Secrets(kek)
	ring := license.Release()
	if o.LicenseRing != nil {
		ring = *o.LicenseRing
	}
	if a.license, err = license.OpenWith(o.DataDir, ring); err != nil {
		st.Close()
		return nil, err
	}
	a.sender = &notify.Sender{Log: o.Log, Instance: a.instance(), Client: &http.Client{Transport: &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	a.cfg = &configManager{app: a, path: o.ConfigPath}
	if a.pki, err = cluster.LoadOrCreatePKI(o.DataDir, o.Now()); err != nil {
		st.Close()
		return nil, err
	}
	a.master = cluster.NewMaster(cluster.MasterOptions{
		Auth:  func(token, addr string) (string, error) { return st.AgentNodeByToken(token, addr) },
		Known: a.isAgentNode,
		Tunables: func() tunables.Set {
			if c := a.applied.Load(); c != nil {
				return c.Tunables
			}
			ts, _ := tunables.New(nil)
			return ts
		},
		Log: o.Log.With("component", "cluster"), Now: o.Now,
		Term:       a.Term,
		Retired:    func() bool { return a.SupersededBy() != nil },
		Superseded: func(term uint64, node string) { a.supersede(term, node) },
		Standby:    a.isStandby,
		WriteState: a.writeState,
	})
	a.engine = engine.New(engine.Deps{
		Clock: o.Now, Probe: o.Probe, Send: a.send, Sink: &sink{a: a, Store: st},
		InContainer: netscope.InContainer(), Remote: a.master, Log: o.Log,
	})
	a.master.SetSink(a.engine)
	a.loadMembership()
	if err := a.cfg.load(); err != nil {
		st.Close()
		return nil, err
	}
	states, _ := st.States()
	incidents, _ := st.OpenIncidents()
	intents, _ := st.PendingIntents()
	a.engine.Restore(states, incidents, intents)
	a.applyConfig(a.cfg.current().Config)
	if !st.FirstRun {
		// A brand-new installation was not "restarted".
		a.engine.StartupNotice(st.PrevClean, st.PrevBeat)
	}
	if v := a.opts.Version; v != "" {
		if prev, _ := st.Setting("version"); prev != v {
			if prev != "" {
				_ = st.Audit(store.AuditEntry{Actor: "system", Action: "system.upgrade", Detail: map[string]any{"from": prev, "to": v}})
			}
			_ = st.SetSetting("version", v)
		}
	}
	a.heartbeat = &engine.Heartbeat{
		Client:    &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}},
		URL:       a.heartbeatURL,
		Interval:  func() time.Duration { return a.cfg.current().Config.Tunables.Duration("heartbeat.interval") },
		Timeout:   func() time.Duration { return a.cfg.current().Config.Tunables.Duration("heartbeat.timeout") },
		Now:       o.Now,
		UserAgent: "unagikuma/" + a.instance(),
	}
	return a, nil
}

// SetPublisher installs the live-event broadcaster (the SSE hub).
func (a *App) SetPublisher(f func(typ string, v any)) {
	a.pubMu.Lock()
	a.publish = f
	a.pubMu.Unlock()
}

func (a *App) emit(typ string, v any) {
	a.pubMu.RLock()
	f := a.publish
	a.pubMu.RUnlock()
	f(typ, v)
}

// renameMonitors carries the history of renamed monitors over before the
// new configuration is applied: renames made through the API, and was=
// written in the file.
func (a *App) renameMonitors(old, cfg *config.Config, renames map[string]string) {
	all := map[string]string{}
	for o, n := range renames {
		all[o] = n
	}
	for _, m := range cfg.Monitors {
		if w := m.Opts["was"]; w != "" {
			all[w] = m.Name
		}
	}
	for o, n := range all {
		// Only a monitor that existed and is gone, under a new name that
		// did not exist: was= is a no-op once applied.
		if o == n || old.FindMonitor(o) == nil || cfg.FindMonitor(o) != nil || cfg.FindMonitor(n) == nil || old.FindMonitor(n) != nil {
			continue
		}
		if err := a.store.RenameMonitor(o, n); err != nil {
			a.log.Error("rename monitor", "from", o, "to", n, "err", err)
			continue
		}
		a.engine.RenameMonitor(o, n)
		a.log.Info("monitor renamed", "from", o, "to", n)
	}
}

func (a *App) applyConfig(cfg *config.Config) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	a.applyLocked(cfg)
}

// applyLocked is applyConfig with applyMu held.
func (a *App) applyLocked(cfg *config.Config) {
	a.store.SetTunables(cfg.Tunables)
	a.loadDirectory(cfg.Tunables)
	defs := make([]store.MonitorDef, 0, len(cfg.Monitors))
	for _, m := range cfg.Monitors {
		defs = append(defs, store.MonitorDef{Name: m.Name, DefHash: m.DefHash})
	}
	if err := a.store.SyncMonitors(defs); err != nil {
		a.log.Error("monitor ids", "err", err)
	}
	ec := a.engineConfig(cfg)
	a.applied.Store(cfg)
	if a.SupersededBy() != nil {
		// Replaced by a newer master: check nothing, notify nothing.
		ec.Monitors, ec.Maints = nil, nil
	}
	a.engine.Apply(ec)
	a.master.SetConfigs(agentConfigs(cfg, ec))
	go a.syncStandbys()
}

// isAgentNode reports whether name is a remote node of the running
// configuration (the master's own NODE record is not one).
func (a *App) isAgentNode(name string) bool {
	c := a.applied.Load()
	if c == nil || name == c.MName {
		return false
	}
	for _, n := range c.Nodes {
		if n.Name == name {
			return true
		}
	}
	return false
}

func (a *App) now() time.Time { return a.opts.Now() }

func (a *App) heartbeatURL() string {
	if v, _ := a.store.Setting("heartbeat_url"); v != "" {
		return v
	}
	return a.opts.HeartbeatURL
}

// Run runs the engine and the background jobs until ctx ends, then closes
// the store (writing the clean-shutdown marker).
func (a *App) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	run(func() { a.engine.Run(ctx) })
	run(func() { a.heartbeat.Run(ctx) })
	run(func() { a.runAgentMode(ctx) })
	run(func() { a.every(ctx, 2*time.Second, a.cfg.check) })
	run(func() { a.every(ctx, 30*time.Second, func() { _ = a.store.Beat() }) })
	run(func() { a.every(ctx, 15*time.Second, a.watchStorage) })
	run(func() { a.every(ctx, time.Hour, func() { a.license.Check(a.now()) }) })
	run(func() { a.every(ctx, 20*time.Second, a.tickChanges) })
	run(func() {
		a.every(ctx, a.Config().Config.Tunables.Duration("cluster.standby-sync"), a.syncStandbys)
	})
	run(func() {
		a.retirePrevious(ctx)
		a.every(ctx, time.Minute, func() { a.retirePrevious(ctx) })
	})
	run(func() {
		a.every(ctx, time.Hour, func() {
			ts := a.cfg.current().Config.Tunables
			r := store.RetentionFrom(ts)
			if a.store.LowDisk(&r, ts) {
				free, _ := a.store.FreeRatio()
				a.log.Warn("disk almost full: keeping raw results for a shorter time", "free", fmt.Sprintf("%.1f%%", free*100), "raw_retention", r.Raw)
			}
			if err := a.store.Prune(r); err != nil {
				a.log.Warn("prune", "err", err)
			}
		})
	})
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	run(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				a.log.Info("SIGHUP: checking monitors.bind")
				a.cfg.check()
			}
		}
	})
	<-ctx.Done()
	a.master.Close()
	wg.Wait()
	return a.store.Close()
}

func (a *App) every(ctx context.Context, d time.Duration, f func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f()
		}
	}
}

// Accessors for the HTTP layer.

// Engine returns the engine.
func (a *App) Engine() *engine.Engine { return a.engine }

// Cluster returns the master end of the agent protocol.
func (a *App) Cluster() *cluster.Master { return a.master }

// PKI returns the cluster CA and the certificate agents connect to.
func (a *App) PKI() *cluster.PKI { return a.pki }

// Store returns the store.
func (a *App) Store() *store.Store { return a.store }

// SecretStore returns the encrypted secrets.
func (a *App) SecretStore() *store.Secrets { return a.secrets }

// Config returns the active configuration.
func (a *App) Config() ConfigState { return a.cfg.current() }

// ConfigPath is the path of monitors.bind.
func (a *App) ConfigPath() string { return a.cfg.path }

// watchStorage turns writes that keep failing (a full disk, a read-only
// file system) into a system incident, so it does not go unnoticed.
func (a *App) watchStorage() {
	wh := a.store.WriteHealth()
	failing := wh.Failing && a.now().Sub(wh.Since) >= a.Config().Config.Tunables.Duration("storage.write-alert")
	a.engine.StorageFailing(failing, wh.Since, wh.Error, wh.Lost)
}

// Banner returns the configuration banner, if any.
func (a *App) Banner() *Banner {
	a.cfg.mu.Lock()
	defer a.cfg.mu.Unlock()
	return a.cfg.banner
}

// SaveConfig validates and applies a new text.
func (a *App) SaveConfig(text []byte, ifMatch, author, message string) (ConfigState, error) {
	if a.SupersededBy() != nil {
		return a.Config(), ErrSuperseded
	}
	return a.cfg.save(text, ifMatch, author, message)
}

// ValidateConfig checks a text without applying it.
func (a *App) ValidateConfig(text []byte) (*config.Config, []config.Diagnostic) {
	_, cfg, diags := config.Load(a.cfg.path, text, a.validators())
	return cfg, diags
}

// EditConfig applies a structured change (record upsert, delete, tunables).
func (a *App) EditConfig(author, message string, change func(f *config.File) error) (ConfigState, error) {
	if a.SupersededBy() != nil {
		return a.Config(), ErrSuperseded
	}
	return a.cfg.edit(author, message, change)
}

// BumpSerial applies the file on disk with an increased serial.
func (a *App) BumpSerial(author string) (ConfigState, error) { return a.cfg.bumpAndApply(author) }

// Rollback re-applies a previous version.
func (a *App) Rollback(serial uint32, author string) (ConfigState, error) {
	if a.SupersededBy() != nil {
		return a.Config(), ErrSuperseded
	}
	return a.cfg.rollback(serial, author)
}

// Validators returns the validation hooks (for the CLI).
func (a *App) Validators() config.Options { return a.validators() }

// Sender returns the notification sender.
func (a *App) Sender() *notify.Sender { return a.sender }

// HeartbeatStatus returns the external heartbeat status.
func (a *App) HeartbeatStatus() engine.HeartbeatStatus { return a.heartbeat.Status() }

// BeatNow calls the external heartbeat once (settings page "test").
func (a *App) BeatNow(ctx context.Context) engine.HeartbeatStatus {
	a.heartbeat.Beat(ctx)
	return a.heartbeat.Status()
}

// Started is when this process started.
func (a *App) Started() time.Time { return a.started }

// DataDir is the data directory.
func (a *App) DataDir() string { return a.opts.DataDir }

// Version is the build version.
func (a *App) Version() string { return a.opts.Version }

// Instance is the instance name used in messages.
func (a *App) Instance() string { return a.instance() }

// SetSetting changes a runtime setting and re-applies what depends on it.
func (a *App) SetSetting(key, value string) error {
	switch key {
	case "heartbeat_url", "message_lang", "base_url", "instance_name":
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	if err := a.store.SetSetting(key, value); err != nil {
		return err
	}
	if key != "heartbeat_url" {
		a.applyConfig(a.cfg.current().Config)
	}
	return nil
}

// Setting reads a runtime setting.
func (a *App) Setting(key string) string {
	v, _ := a.store.Setting(key)
	if v == "" {
		switch key {
		case "message_lang":
			return a.messageLang()
		case "instance_name":
			return a.instance()
		case "heartbeat_url":
			return a.opts.HeartbeatURL
		case "base_url":
			return a.opts.BaseURL
		}
	}
	return v
}

// IsInvalid reports whether err is a configuration validation error.
func IsInvalid(err error) (*InvalidError, bool) {
	var ie *InvalidError
	ok := errors.As(err, &ie)
	return ie, ok
}

// Reapply re-applies the active configuration (after a secret or a
// setting it depends on changed).
func (a *App) Reapply() { a.applyConfig(a.cfg.current().Config) }

// License is the registration of this installation.
func (a *App) License() *license.Checker { return a.license }
