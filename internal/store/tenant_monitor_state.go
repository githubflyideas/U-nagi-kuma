package store

import (
	"database/sql"
	"encoding/json"
	"github.com/githubflyideas/unagikuma/internal/engine"
)

type stateExecutor interface {
	Exec(string, ...any) (sql.Result, error)
}

func saveMonitorState(db stateExecutor, tenant TenantID, r engine.StateRecord) error {
	_, err := db.Exec(`INSERT INTO monitor_state(workspace_id,tenant_id,name,state,status,reason,detail,latency_ms,last_check,last_change,incident_id,def_hash)
 VALUES(COALESCE((SELECT workspace_id FROM monitors WHERE tenant_id=? AND name=? AND retired_at IS NULL),
 (SELECT id FROM workspaces WHERE tenant_id=? AND is_default=1)),?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(tenant_id,workspace_id,name) DO UPDATE SET state=excluded.state,status=excluded.status,reason=excluded.reason,
		detail=excluded.detail,latency_ms=excluded.latency_ms,last_check=excluded.last_check,last_change=excluded.last_change,
		incident_id=excluded.incident_id,def_hash=excluded.def_hash`,
		tenant, r.Name, tenant, tenant, r.Name, mustJSON(r.State), r.Status, r.Reason, r.Detail, r.LatencyMS, ms(r.LastCheck), ms(r.LastChange), r.IncidentID, r.DefHash)
	return err
}

func scanMonitorStates(rows *sql.Rows, err error) ([]engine.StateRecord, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.StateRecord
	for rows.Next() {
		var r engine.StateRecord
		var st string
		var reason, detail, hash sql.NullString
		var lat sql.NullFloat64
		var lc, lch, inc sql.NullInt64
		if err := rows.Scan(&r.Name, &st, &r.Status, &reason, &detail, &lat, &lc, &lch, &inc, &hash); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(st), &r.State)
		r.Reason, r.Detail, r.DefHash, r.LatencyMS = reason.String, detail.String, hash.String, lat.Float64
		r.LastCheck, r.LastChange, r.IncidentID = fromMS(lc.Int64), fromMS(lch.Int64), inc.Int64
		out = append(out, r)
	}
	return out, rows.Err()
}

// ownedMonitor resolves an ID within this tenant, never by an untrusted name.
func (r *MonitorRepository) ownedMonitor(tx *sql.Tx, id int64) (string, error) {
	var name string
	err := tx.QueryRow(`SELECT name FROM monitors WHERE tenant_id=? AND id=? AND retired_at IS NULL AND (?=0 OR workspace_id=?)`, r.ctx.TenantID, id, r.workspaceScoped, r.workspace).Scan(&name)
	return name, tenantAccessError(err)
}

func (r *MonitorRepository) SaveState(id int64, state engine.StateRecord) error {
	return r.access(true, func(tx *sql.Tx) error {
		name, err := r.ownedMonitor(tx, id)
		if err != nil {
			return err
		}
		// Incidents are intentionally not exposed by this repository yet.
		if name != state.Name || (state.IncidentID != 0 && (r.ctx.TenantID != r.s.defaultTenant || r.workspaceScoped)) {
			return ErrTenantAccess
		}
		if state.IncidentID != 0 {
			var isDefault bool
			if err = tx.QueryRow(`SELECT w.is_default FROM monitors m JOIN workspaces w ON w.id=m.workspace_id AND w.tenant_id=m.tenant_id WHERE m.id=?`, id).Scan(&isDefault); err != nil {
				return err
			}
			if !isDefault {
				return ErrTenantAccess
			}
		}
		return saveMonitorState(tx, r.ctx.TenantID, state)
	})
}

func (r *MonitorRepository) States() (out []engine.StateRecord, err error) {
	err = r.access(false, func(tx *sql.Tx) error {
		rows, e := tx.Query(`SELECT name,state,status,reason,detail,latency_ms,last_check,last_change,incident_id,def_hash FROM monitor_state WHERE tenant_id=? AND (?=0 OR workspace_id=?)`, r.ctx.TenantID, r.workspaceScoped, r.workspace)
		out, e = scanMonitorStates(rows, e)
		return e
	})
	return
}
