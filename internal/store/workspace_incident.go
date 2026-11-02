package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/notify"
)

// WorkspaceNotificationPlan contains no resolved credentials. Empty Channel
// records incident transitions without requesting delivery.
type WorkspaceNotificationPlan struct {
	Policy  engine.NotifyPolicy
	Channel string
	Name    string
	Title   string
}

type WorkspaceEvent struct {
	ID         int64     `json:"id"`
	MonitorID  int64     `json:"monitor_id"`
	IncidentID int64     `json:"incident_id"`
	Kind       string    `json:"kind"`
	At         time.Time `json:"at"`
}
type WorkspaceIncident struct {
	ID        int64                `json:"id"`
	MonitorID int64                `json:"monitor_id"`
	State     engine.IncidentState `json:"state"`
}

// WorkspaceEvents rechecks live membership inside the read transaction. Monitor
// zero lists this Workspace; a foreign/retired monitor ID is not a scope selector.
func (s *Store) WorkspaceEvents(ctx WorkspaceContext, monitorID int64, limit int) (out []WorkspaceEvent, err error) {
	out = []WorkspaceEvent{}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	err = s.workspaceTx(ctx, false, func(tx *sql.Tx) error {
		if monitorID != 0 {
			var n int
			if e := tx.QueryRow(`SELECT 1 FROM monitors WHERE id=? AND tenant_id=? AND workspace_id=?`, monitorID, ctx.TenantID, ctx.WorkspaceID).Scan(&n); e != nil {
				return tenantAccessError(e)
			}
		}
		rows, e := tx.Query(`SELECT id,monitor_id,incident_id,kind,at FROM workspace_events WHERE tenant_id=? AND workspace_id=? AND (?=0 OR monitor_id=?) ORDER BY id DESC LIMIT ?`, ctx.TenantID, ctx.WorkspaceID, monitorID, monitorID, limit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v WorkspaceEvent
			var at int64
			if e = rows.Scan(&v.ID, &v.MonitorID, &v.IncidentID, &v.Kind, &at); e != nil {
				return e
			}
			v.At = fromMS(at)
			out = append(out, v)
		}
		return rows.Err()
	})
	if err != nil {
		out = nil
	}
	return
}
func (s *Store) WorkspaceIncidents(ctx WorkspaceContext, limit int) (out []WorkspaceIncident, err error) {
	out = []WorkspaceIncident{}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	err = s.workspaceTx(ctx, false, func(tx *sql.Tx) error {
		rows, e := tx.Query(`SELECT id,monitor_id,state FROM workspace_incidents WHERE tenant_id=? AND workspace_id=? ORDER BY id DESC LIMIT ?`, ctx.TenantID, ctx.WorkspaceID, limit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v WorkspaceIncident
			var raw string
			if e = rows.Scan(&v.ID, &v.MonitorID, &raw); e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(raw), &v.State); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	if err != nil {
		out = nil
	}
	return
}

func (w *WorkspaceRuntimeWriter) recordIncident(tx *sql.Tx, id int64, state engine.StateRecord, plan WorkspaceNotificationPlan) error {
	r := w.reader
	at := r.s.now()
	var iid int64
	var raw string
	var incident *engine.IncidentState
	err := tx.QueryRow(`SELECT id,state FROM workspace_incidents WHERE tenant_id=? AND workspace_id=? AND monitor_id=? ORDER BY id DESC LIMIT 1`, r.tenant, r.workspace, id).Scan(&iid, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		incident = &engine.IncidentState{}
		if err = json.Unmarshal([]byte(raw), incident); err != nil {
			return err
		}
	}
	var notices []engine.PlannedNotice
	var events []string
	hard := state.State.Avail == engine.StatusCritical && state.State.Hard
	switch {
	case hard && (incident == nil || !incident.Open()):
		old := incident
		incident, notices, events = engine.Fail(incident, at, "business", plan.Policy)
		if old != incident {
			iid = 0
			events = append(events, "opened")
		}
	case incident != nil && incident.Open() && state.State.Avail == engine.StatusOK:
		notices = engine.Recover(incident, at, plan.Policy)
		events = append(events, "resolved")
	case incident != nil && !state.State.Held:
		notices, events = engine.Advance(incident, at, plan.Policy)
	}
	if incident == nil {
		return nil
	}
	rawBytes, err := json.Marshal(incident)
	if err != nil {
		return err
	}
	if iid == 0 {
		res, e := tx.Exec(`INSERT INTO workspace_incidents(tenant_id,workspace_id,monitor_id,state,updated_at) VALUES(?,?,?,?,?)`, r.tenant, r.workspace, id, string(rawBytes), ms(at))
		if e != nil {
			return e
		}
		iid, e = res.LastInsertId()
		if e != nil {
			return e
		}
	} else {
		if _, err = tx.Exec(`UPDATE workspace_incidents SET state=?,updated_at=? WHERE id=? AND tenant_id=? AND workspace_id=?`, string(rawBytes), ms(at), iid, r.tenant, r.workspace); err != nil {
			return err
		}
	}
	if !incident.Open() {
		if _, err = tx.Exec(`UPDATE workspace_notification_intents SET state='cancelled',updated_at=? WHERE tenant_id=? AND workspace_id=? AND incident_id=? AND state='pending' AND json_extract(message,'$.kind')!='RECOVERY'`, ms(at), r.tenant, r.workspace, iid); err != nil {
			return err
		}
	}
	for _, ev := range events {
		if _, err = tx.Exec(`INSERT INTO workspace_events(tenant_id,workspace_id,monitor_id,incident_id,kind,at) VALUES(?,?,?,?,?,?)`, r.tenant, r.workspace, id, iid, ev, ms(at)); err != nil {
			return err
		}
	}
	if plan.Channel == "" {
		return nil
	}
	for _, n := range notices {
		message := notify.Message{Kind: string(n.Kind), Monitor: plan.Name, Title: plan.Title, Status: state.Status, Count: n.Seq, Max: n.Max, IncidentID: iid, Time: at, Priority: "high", Type: "HTTP"}
		b, e := json.Marshal(message)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`INSERT INTO workspace_notification_intents(tenant_id,workspace_id,monitor_id,incident_id,generation,channel,message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, r.tenant, r.workspace, id, iid, r.generation, plan.Channel, string(b), ms(at), ms(at)); e != nil {
			return e
		}
	}
	return nil
}
