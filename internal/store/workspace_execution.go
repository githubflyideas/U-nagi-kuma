package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/probe"
)

// WorkspaceMonitorKey is an internal registry key, never a display name or target.
// Encoding both components is unambiguous; changing a title/target keeps the ID.
func WorkspaceMonitorKey(workspace WorkspaceID, name string) string {
	return "workspace:" + base64.RawURLEncoding.EncodeToString([]byte(workspace)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

// WorkspaceRuntimeWriter is trusted supervisor glue bound to immutable scope and
// generation. Its incident/outbox operations never call the legacy global Sink.
// It is deliberately not an engine.Sink and must never be exposed over HTTP.
type WorkspaceRuntimeWriter struct{ reader *RuntimeSecretReader }

type workspaceExecution struct {
	ctx    context.Context
	reader *RuntimeSecretReader
	cancel context.CancelFunc
}

var ErrWorkspaceRuntimeBusy = errors.New("workspace execution capacity occupied")

// BeginExecution admits at most one pass per Workspace and eight per Store.
// Admission and lease registration share the same authority transaction. Finish
// MUST be deferred by the supervisor, after its probe work has joined.
func (w *WorkspaceRuntimeWriter) BeginExecution(parent context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(parent)
	lease := &workspaceExecution{ctx: ctx, cancel: cancel}
	err := w.access(func(*sql.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		lease.reader = w.reader
		s := w.reader.s
		if len(s.workspaceExecutions) >= 8 {
			return ErrWorkspaceRuntimeBusy
		}
		for old := range s.workspaceExecutions {
			if old.reader.tenant == w.reader.tenant && old.reader.workspace == w.reader.workspace {
				return ErrWorkspaceRuntimeBusy
			}
		}
		if s.workspaceExecutions == nil {
			s.workspaceExecutions = map[*workspaceExecution]struct{}{}
		}
		s.workspaceExecutions[lease] = struct{}{}
		return nil
	})
	finish := func() {
		cancel()
		if lease.reader != nil {
			s := lease.reader.s
			s.wmu.Lock()
			delete(s.workspaceExecutions, lease)
			s.wmu.Unlock()
		}
	}
	if err != nil {
		finish()
		return nil, nil, err
	}
	return ctx, finish, nil
}

// Called with wmu held after a successful Workspace transaction. A failed write
// never cancels the running generation. Cancellation happens before the mutator
// returns, but network teardown/join remains the supervisor's responsibility.
func (s *Store) fenceWorkspaceExecutions(tenant TenantID, workspace WorkspaceID) {
	for lease := range s.workspaceExecutions {
		r := lease.reader
		if r.tenant != tenant || r.workspace != workspace {
			continue
		}
		var valid int
		err := s.core.QueryRow(`SELECT 1 FROM workspace_secret_readers r JOIN workspaces w ON w.tenant_id=r.tenant_id AND w.id=r.workspace_id WHERE r.tenant_id=? AND r.workspace_id=? AND r.generation=? AND r.token_hash=? AND r.active=1`, r.tenant, r.workspace, r.generation, r.hash).Scan(&valid)
		if err != nil {
			lease.cancel()
		}
	}
}

func (s *Store) BindWorkspaceRuntime(reader *RuntimeSecretReader) (*WorkspaceRuntimeWriter, error) {
	if reader == nil || reader.s != s {
		return nil, ErrTenantAccess
	}
	w := &WorkspaceRuntimeWriter{reader: reader}
	if err := w.access(func(*sql.Tx) error { return nil }); err != nil {
		return nil, err
	}
	return w, nil
}
func (w *WorkspaceRuntimeWriter) access(fn func(*sql.Tx) error) error {
	if w == nil || w.reader == nil {
		return ErrTenantAccess
	}
	r := w.reader
	_, err := r.read(func(tx *sql.Tx) (string, error) {
		var valid int
		// A manually issued secret reader is not execution authority: only the exact
		// currently published configuration generation, in a nondefault Workspace.
		err := tx.QueryRow(`SELECT 1 FROM workspace_applied_configs c JOIN workspaces w ON w.tenant_id=c.tenant_id AND w.id=c.workspace_id WHERE c.tenant_id=? AND c.workspace_id=? AND c.generation=? AND w.is_default=0`, r.tenant, r.workspace, r.generation).Scan(&valid)
		if err != nil {
			return "", tenantAccessError(err)
		}
		return "", fn(tx)
	})
	return err
}

// Register synchronizes only this scope. Registration and generation admission
// share a transaction. No default name/alias cache is populated.
func (w *WorkspaceRuntimeWriter) Register(defs []MonitorDef) (ids map[string]int64, err error) {
	err = w.access(func(tx *sql.Tx) error {
		r := w.reader
		ids = map[string]int64{}
		for _, d := range defs {
			key := WorkspaceMonitorKey(r.workspace, d.Name)
			if d.Name == "" {
				return ErrWorkspaceConfigInvalid
			}
			if _, ok := ids[d.Name]; ok {
				return ErrWorkspaceConfigInvalid
			}
			id, e := activeOrNewWorkspace(tx, r.tenant, r.workspace, key, d.DefHash, ms(r.s.now()))
			if e != nil {
				return e
			}
			if _, e = tx.Exec(`UPDATE monitors SET def_hash=? WHERE id=?`, d.DefHash, id); e != nil {
				return e
			}
			ids[d.Name] = id
		}
		rows, e := tx.Query(`SELECT id FROM monitors WHERE tenant_id=? AND workspace_id=? AND retired_at IS NULL`, r.tenant, r.workspace)
		if e != nil {
			return e
		}
		var retire []int64
		for rows.Next() {
			var id int64
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			keep := false
			for _, v := range ids {
				if id == v {
					keep = true
					break
				}
			}
			if !keep {
				retire = append(retire, id)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range retire {
			if _, e = tx.Exec(`UPDATE monitors SET retired_at=? WHERE id=?`, ms(r.s.now()), id); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		ids = nil
	}
	return
}

// Pause records administrative state without fabricating a probe/history sample.
func (w *WorkspaceRuntimeWriter) Pause(id int64) error {
	flushMu.Lock()
	defer flushMu.Unlock()
	err := w.access(func(tx *sql.Tx) error {
		r := w.reader
		var name, hash string
		if e := tx.QueryRow(`SELECT name,def_hash FROM monitors WHERE id=? AND tenant_id=? AND workspace_id=? AND retired_at IS NULL`, id, r.tenant, r.workspace).Scan(&name, &hash); e != nil {
			return tenantAccessError(e)
		}
		var last int64
		e := tx.QueryRow(`SELECT last_check FROM monitor_state WHERE tenant_id=? AND workspace_id=? AND name=?`, r.tenant, r.workspace, name).Scan(&last)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		return saveMonitorState(tx, r.tenant, engine.StateRecord{Name: name, State: engine.NewState(), Status: engine.StatusPaused, LastCheck: fromMS(last), LastChange: r.s.now(), DefHash: hash})
	})
	if err == nil {
		delete(w.reader.s.batch.last, id)
	}
	return err
}

// Record advances the existing pure engine state machine without invoking its
// incident/notification/event loop. History commits synchronously. Core and
// history remain separate databases: a core commit failure can leave a history
// sample without updated state; callers MUST report failure, never retry blindly.
func (w *WorkspaceRuntimeWriter) Record(id int64, result probe.Result, interval time.Duration, params engine.StepParams) (engine.StateRecord, error) {
	return w.record(id, result, interval, params, nil)
}

// RecordNotification commits state, incident transition, event and durable intent
// in one core transaction. History remains a separate database (see Record).
func (w *WorkspaceRuntimeWriter) RecordNotification(id int64, result probe.Result, interval time.Duration, params engine.StepParams, plan WorkspaceNotificationPlan) (engine.StateRecord, error) {
	return w.record(id, result, interval, params, &plan)
}
func (w *WorkspaceRuntimeWriter) record(id int64, result probe.Result, interval time.Duration, params engine.StepParams, plan *WorkspaceNotificationPlan) (state engine.StateRecord, err error) {
	flushMu.Lock()
	defer flushMu.Unlock()
	err = w.access(func(tx *sql.Tx) error {
		r := w.reader
		var name, hash string
		e := tx.QueryRow(`SELECT name,def_hash FROM monitors WHERE id=? AND tenant_id=? AND workspace_id=? AND retired_at IS NULL`, id, r.tenant, r.workspace).Scan(&name, &hash)
		if e != nil {
			return tenantAccessError(e)
		}
		prev := engine.NewState()
		var raw, oldHash, oldStatus string
		var lastChange int64
		e = tx.QueryRow(`SELECT state,def_hash,status,last_change FROM monitor_state WHERE tenant_id=? AND workspace_id=? AND name=?`, r.tenant, r.workspace, name).Scan(&raw, &oldHash, &oldStatus, &lastChange)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e == nil && oldHash == hash {
			if e = json.Unmarshal([]byte(raw), &prev); e != nil {
				return e
			}
		}
		kind := engine.VerdictUnknown
		switch result.Vote {
		case probe.VoteOK:
			kind = engine.VerdictOK
		case probe.VoteFail:
			kind = engine.VerdictCritical
		}
		next := engine.Step(prev, engine.Verdict{Kind: kind}, params).To
		at := r.s.now()
		state = engine.StateRecord{Name: name, State: next, Status: next.Display(), Reason: result.Reason, LatencyMS: float64(result.Latency) / float64(time.Millisecond), LastCheck: at, LastChange: fromMS(lastChange), DefHash: hash}
		if oldHash != hash || oldStatus != state.Status || state.LastChange.IsZero() {
			state.LastChange = at
		}
		// Do not retain raw exchanges/details, which may contain response credentials.
		v := engine.VerdictRecord{Monitor: name, At: at, Verdict: kind, Status: state.Status, Hard: next.Hard, Reason: result.Reason, LatencyMS: state.LatencyMS, Excluded: kind == engine.VerdictUnknown, Interval: interval}
		if e = saveMonitorState(tx, r.tenant, state); e != nil {
			return e
		}
		if plan != nil {
			if e = w.recordIncident(tx, id, state, *plan); e != nil {
				return e
			}
		}
		if !r.s.batch.flushHistory([]item{{mid: id, verdict: &v}}) {
			return errors.New("workspace history write failed")
		}
		return nil
	})
	if err != nil {
		state = engine.StateRecord{}
	}
	return
}
