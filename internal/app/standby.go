package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/backup"
	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// Handing the master role to another server without an election: the
// master keeps standbys (NODE standby=https://…) supplied with a copy of
// its core/configuration state and on-disk keys, but not history.db. Agents
// follow the highest term they have seen; retirement only converges while
// communication works. Terms do not fence partitioned masters or provide
// single-writer consensus. Promotion is an explicit operator decision.
// Coordinated handover is refused until target readiness can be acknowledged.
const (
	termFile       = "cluster-term"
	supersededFile = "cluster-superseded.json"
	standbyDir     = "standby"
	stateFile      = "state.nkb"
	stateMetaFile  = "state.json"
	promoteFile    = "promote.json"
	previousFile   = "cluster-previous.json"
)

// StandbyState is the copy of the master's state a standby holds.
type StandbyState struct {
	Scope   string    `json:"scope,omitempty"` // expected protocol scope, not a verification acknowledgement
	Term    uint64    `json:"term"`            // the master's term when it was taken
	Fetched time.Time `json:"fetched"`
	Size    int64     `json:"size"`
	Key     string    `json:"key"`
	Error   string    `json:"error,omitempty"` // last failed fetch
}

// Superseded records that this master was replaced.
type Superseded struct {
	Term uint64    `json:"term"`
	At   time.Time `json:"at"`
	By   string    `json:"by,omitempty"` // the node it was handed to, or that reported the newer master
}

func (a *App) dataPath(name ...string) string {
	return filepath.Join(append([]string{a.opts.DataDir}, name...)...)
}

// Term is this master's term: 0 until the first handover.
func (a *App) Term() uint64 {
	b, err := os.ReadFile(a.dataPath(termFile))
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return n
}

// SupersededBy reports whether this master was replaced.
func (a *App) SupersededBy() *Superseded {
	b, err := os.ReadFile(a.dataPath(supersededFile))
	if err != nil {
		return nil
	}
	var s Superseded
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

// supersede steps this master down: it stops checking and refuses agents
// so they move on to the newer master. It stays down across restarts.
func (a *App) supersede(term uint64, by string) {
	if s := a.SupersededBy(); s != nil && s.Term >= term {
		return
	}
	s := Superseded{Term: term, At: a.now(), By: by}
	b, _ := json.Marshal(s)
	if err := os.WriteFile(a.dataPath(supersededFile), b, 0o644); err != nil {
		a.log.Error("cannot record the handover", "err", err)
	}
	a.log.Warn("this master was replaced; it stops checking", "term", term, "by", by)
	_ = a.store.Audit(auditEntry("cluster.superseded", by, map[string]any{"term": term}))
	if c := a.applied.Load(); c != nil {
		a.applyConfig(c)
	}
	a.master.Retire(fmt.Sprintf("this master was replaced (term %d)", term))
	a.emit("banner", nil)
}

// isStandby reports whether a node may hold the master's state.
func (a *App) isStandby(node string) bool {
	if c := a.applied.Load(); c != nil {
		if n := c.FindNode(node); n != nil {
			return n.Standby != ""
		}
	}
	return false
}

// writeState streams an encrypted state archive for a standby. The
// passphrase is new for each download and travels in a header over the
// pinned TLS session only.
func (a *App) writeState(w http.ResponseWriter) error {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	key := hex.EncodeToString(b[:])
	w.Header().Set(cluster.StateKeyHeader, key)
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err := backup.Write(context.Background(), w, a.store.BackupDB, backup.Options{
		DataDir: a.opts.DataDir, ConfigPath: a.opts.ConfigPath, Passphrase: key,
		Instance: a.instance(), Version: a.opts.Version, Now: a.now(),
	})
	return err
}

// syncStandbys asks connected standbys to fetch the state.
func (a *App) syncStandbys() {
	c := a.applied.Load()
	if c == nil || a.SupersededBy() != nil {
		return
	}
	for _, n := range c.Nodes {
		if n.Standby != "" {
			a.master.NotifyState(n.Name)
		}
	}
}

// ErrNotStandby means a handover target is not a connected standby.
var ErrNotStandby = errors.New("the node is not a connected standby (NODE standby=https://…)")

// ErrHandoverReadiness refuses coordinated transfer until the protocol can
// acknowledge a verified current snapshot and successful target startup.
var ErrHandoverReadiness = errors.New("handover refused: target snapshot and restart readiness cannot be acknowledged by this protocol; fence the old master and use explicit offline recovery")

// HandOver fails closed: a connected standby is not proof of restore readiness.
// No promotion is dispatched and the source continues serving.
func (a *App) HandOver(node, author string) (uint64, error) {
	if !a.isStandby(node) {
		return 0, ErrNotStandby
	}
	connected := false
	for _, n := range a.master.Connected() {
		connected = connected || n == node
	}
	if !connected {
		return 0, ErrNotStandby
	}
	return 0, ErrHandoverReadiness
}

// ---- standby side ----

// fetchState keeps a fresh copy of the master's state (standby nodes).
func (a *App) fetchState(ctx context.Context, ag *cluster.Agent) (StandbyState, error) {
	dir := a.dataPath(standbyDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return StandbyState{}, err
	}
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return StandbyState{}, err
	}
	defer os.Remove(tmp.Name())
	fctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	key, term, err := ag.FetchState(fctx, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	meta := a.Standby()
	if err != nil {
		meta.Error = err.Error()
		a.saveStandby(meta)
		return meta, err
	}
	fi, _ := os.Stat(tmp.Name())
	if err := os.Rename(tmp.Name(), filepath.Join(dir, stateFile)); err != nil {
		return meta, err
	}
	meta = StandbyState{Scope: "core-config-keys-no-history", Term: term, Fetched: a.now(), Size: fi.Size(), Key: key}
	a.saveStandby(meta)
	a.log.Info("master state copied", "term", term, "bytes", fi.Size())
	return meta, nil
}

func (a *App) saveStandby(m StandbyState) {
	b, _ := json.Marshal(m)
	_ = os.WriteFile(a.dataPath(standbyDir, stateMetaFile), b, 0o600)
}

// Standby is the state copy this server holds (zero when none).
func (a *App) Standby() StandbyState {
	var m StandbyState
	if b, err := os.ReadFile(a.dataPath(standbyDir, stateMetaFile)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// ErrNoState means there is no copy of the master's state to take over
// with.
var ErrNoState = errors.New("this server holds no copy of the master's state: it is not a standby, or has not fetched it yet")

// promoteRequest is left for the next start, which restores the state
// before opening anything.
type promoteRequest struct {
	Term uint64    `json:"term"`
	At   time.Time `json:"at"`
}

// Promote handles explicit operator recovery (term 0). A connected master's
// state must refresh successfully; failures never fall back to a stale copy.
// Nonzero terms are legacy handover requests and fail closed. Disconnected
// disaster recovery still requires the operator to fence the previous master
// and accept the last snapshot's age/scope; this is not a freshness guarantee.
func (a *App) Promote(ctx context.Context, term uint64, author string) (uint64, error) {
	if term != 0 {
		return 0, ErrHandoverReadiness
	}
	a.am.mu.Lock()
	ag := a.am.agent
	a.am.mu.Unlock()
	if ag != nil && ag.Status().Connected {
		if _, err := a.fetchState(ctx, ag); err != nil {
			return 0, fmt.Errorf("promotion refused: refresh the master's state: %w", err)
		}
	}
	meta := a.Standby()
	if meta.Key == "" {
		return 0, ErrNoState
	}
	if _, err := os.Stat(a.dataPath(standbyDir, stateFile)); err != nil {
		return 0, ErrNoState
	}
	if term == 0 {
		term = meta.Term + 1
		if ag != nil {
			term = max(meta.Term, ag.Term()) + 1
		}
	}
	if err := writePromote(a.opts.DataDir, term, a.now()); err != nil {
		return 0, err
	}
	a.log.Warn("taking over as master: restarting", "term", term, "by", author)
	a.requestRestart()
	return term, nil
}

func writePromote(dir string, term uint64, now time.Time) error {
	b, _ := json.Marshal(promoteRequest{Term: term, At: now})
	return os.WriteFile(filepath.Join(dir, promoteFile), b, 0o600)
}

// PromoteOffline prepares a stopped standby to start as master (the
// promote command).
func PromoteOffline(dir string, now time.Time) (uint64, StandbyState, error) {
	var meta StandbyState
	b, err := os.ReadFile(filepath.Join(dir, standbyDir, stateMetaFile))
	if err != nil || json.Unmarshal(b, &meta) != nil || meta.Key == "" {
		return 0, meta, ErrNoState
	}
	term := meta.Term + 1
	if m, err := readMembershipTerm(dir); err == nil && m >= term {
		term = m + 1
	}
	return term, meta, writePromote(dir, term, now)
}

func readMembershipTerm(dir string) (uint64, error) {
	b, err := os.ReadFile(filepath.Join(dir, agentFile))
	if err != nil {
		return 0, err
	}
	var m Membership
	return m.Term, json.Unmarshal(b, &m)
}

// applyPromote runs at start, before anything is opened: a standby told
// to take over restores the master's state and becomes the master.
func applyPromote(dir, configPath string) (bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, promoteFile))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var req promoteRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return false, fmt.Errorf("%s: %w", promoteFile, err)
	}
	var meta StandbyState
	mb, err := os.ReadFile(filepath.Join(dir, standbyDir, stateMetaFile))
	if err != nil || json.Unmarshal(mb, &meta) != nil {
		return false, ErrNoState
	}
	f, err := os.Open(filepath.Join(dir, standbyDir, stateFile))
	if err != nil {
		return false, ErrNoState
	}
	defer f.Close()
	// The master this standby followed: it is told to step down once it
	// can be reached, in case it was only cut off and comes back.
	var prev []string
	if b, err := os.ReadFile(filepath.Join(dir, agentFile)); err == nil {
		var m Membership
		if json.Unmarshal(b, &m) == nil && m.Server != "" {
			prev = append(prev, m.Server)
		}
	}
	// Restore stages and checks the archive before replacing databases.
	// Never delete local state before archive validation succeeds.
	manifest, err := backup.Restore(f, dir, configPath, meta.Key, true)
	if err != nil {
		return false, fmt.Errorf("restore the master's state: %w", err)
	}
	// Core-only snapshots do not contain the old master's history. Preserve
	// the standby's unrelated local history for operator recovery, but never
	// serve it as if it were replicated master history. Same-volume renames
	// avoid copying a potentially huge database and never overwrite a prior
	// recovery directory. Failure leaves the promotion marker for inspection.
	hasHistory := false
	for _, name := range manifest.Files {
		hasHistory = hasHistory || name == "history.db"
	}
	if !hasHistory {
		var saved string
		for _, suffix := range []string{"", "-wal", "-shm"} {
			path := filepath.Join(dir, "history.db"+suffix)
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return false, err
			}
			if saved == "" {
				saved, err = os.MkdirTemp(filepath.Join(dir, standbyDir), "previous-history-")
				if err != nil {
					return false, fmt.Errorf("preserve local history: %w", err)
				}
			}
			if err := os.Rename(path, filepath.Join(saved, "history.db"+suffix)); err != nil {
				return false, fmt.Errorf("preserve local history in %s: %w", saved, err)
			}
		}
	}
	for _, name := range []string{agentFile, agentTokenFile, supersededFile, promoteFile} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	if len(prev) > 0 {
		b, _ := json.Marshal(prev)
		_ = os.WriteFile(filepath.Join(dir, previousFile), b, 0o644)
	}
	return true, os.WriteFile(filepath.Join(dir, termFile), []byte(strconv.FormatUint(req.Term, 10)+"\n"), 0o644)
}

// requestRestart asks the process to restart (the server command re-runs
// itself once Run returns).
func (a *App) requestRestart() {
	a.restartOnce.Do(func() { close(a.restart) })
}

// RestartRequested is closed when the app needs a restart.
func (a *App) RestartRequested() <-chan struct{} { return a.restart }

func auditEntry(action, actor string, detail map[string]any) store.AuditEntry {
	return store.AuditEntry{Action: action, Actor: actor, Target: "cluster", Detail: detail}
}

// ErrSuperseded refuses configuration changes on a master that handed
// over: they belong on the new master.
var ErrSuperseded = errors.New("this server handed the master role to another one; change the configuration there")

// retirePrevious tells the master this one replaced to step down, until
// it confirms: a master that was only cut off must not come back and
// check (and alert) alongside this one.
func (a *App) retirePrevious(ctx context.Context) {
	b, err := os.ReadFile(a.dataPath(previousFile))
	if err != nil || a.SupersededBy() != nil {
		return
	}
	var prev []string
	if json.Unmarshal(b, &prev) != nil {
		return
	}
	n, err := a.pki.RetireNotice(a.Term(), a.now())
	if err != nil {
		return
	}
	var left []string
	for _, srv := range prev {
		sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := cluster.SendRetire(sctx, srv, a.pki.Fingerprint(), n)
		cancel()
		if err != nil {
			a.log.Debug("previous master not told yet", "server", srv, "err", err)
			left = append(left, srv)
			continue
		}
		a.log.Info("the previous master acknowledged the handover", "server", srv)
	}
	if len(left) == 0 {
		_ = os.Remove(a.dataPath(previousFile))
		return
	}
	b, _ = json.Marshal(left)
	_ = os.WriteFile(a.dataPath(previousFile), b, 0o644)
}

// AcceptRetire handles a notice from the master that replaced this one.
func (a *App) AcceptRetire(n cluster.RetireNotice) error {
	if err := a.pki.CheckRetire(n); err != nil {
		return err
	}
	if n.Term <= a.Term() {
		return fmt.Errorf("term %d is not newer than this master's %d", n.Term, a.Term())
	}
	a.supersede(n.Term, "the new master")
	return nil
}
