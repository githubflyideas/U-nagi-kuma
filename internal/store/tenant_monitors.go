package store

import "database/sql"

// MonitorRepository is the authorized monitor/state/history boundary, not an
// engine.Sink. Incidents, secrets and runtime configuration are NOT tenant-ready.
// Keep nondefault HTTP/UI and runtime activation disabled until those are scoped.
// A zero-value repository denies access. The principal must come from trusted
// authentication, NEVER user_id or tenant_id supplied in a request body.
type MonitorRepository struct {
	s               *Store
	ctx             TenantContext
	workspace       WorkspaceID
	workspaceScoped bool
}

// WorkspaceMonitors never treats a missing workspace as company-wide access.
func (s *Store) WorkspaceMonitors(ctx WorkspaceContext) (*MonitorRepository, error) {
	r := &MonitorRepository{s: s, ctx: ctx.TenantContext, workspace: ctx.WorkspaceID, workspaceScoped: true}
	if err := r.access(false, func(*sql.Tx) error { return nil }); err != nil {
		return nil, err
	}
	return r, nil
}

// TenantMonitors is explicitly company-wide and requires company admin even for reads.
func (s *Store) TenantMonitors(ctx TenantContext) (*MonitorRepository, error) {
	r := &MonitorRepository{s: s, ctx: ctx}
	if err := r.access(false, func(*sql.Tx) error { return nil }); err != nil {
		return nil, err
	}
	return r, nil
}

// Authorization is deliberately not cached. The immediate core transaction
// keeps membership/role/ownership stable through the complete operation, even
// when its data resides in history.db. Area-restricted users fail closed until
// monitor area ownership can be checked here (never silently broaden a grant).
func authorizeMonitors(tx *sql.Tx, ctx TenantContext, write bool) error {
	role, err := companyRole(tx, ctx)
	if err != nil {
		return err
	}
	if role != RoleAdmin {
		return ErrTenantAccess
	}
	return nil
}

func (r *MonitorRepository) access(write bool, fn func(*sql.Tx) error) error {
	if r == nil || r.s == nil || r.ctx.TenantID == "" || r.ctx.UserID <= 0 {
		return ErrTenantAccess
	}
	r.s.wmu.Lock()
	defer r.s.wmu.Unlock()
	tx, err := r.s.core.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if r.workspaceScoped {
		err = authorizeWorkspace(tx, WorkspaceContext{r.ctx, r.workspace}, write)
	} else {
		err = authorizeMonitors(tx, r.ctx, write)
	}
	if err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MonitorRepository) SyncMonitors(defs []MonitorDef) error {
	if r == nil || r.s == nil {
		return ErrTenantAccess
	}
	if r.workspaceScoped {
		if r.workspace == "" {
			return ErrTenantAccess
		}
		return r.s.syncWorkspaceMonitors(r.ctx.TenantID, &r.ctx, r.workspace, defs)
	}
	return r.s.syncMonitors(r.ctx.TenantID, &r.ctx, defs)
}

func (r *MonitorRepository) MonitorName(id int64) (name string, err error) {
	err = r.access(false, func(tx *sql.Tx) error { var e error; name, e = r.ownedMonitor(tx, id); return e })
	return
}

func (r *MonitorRepository) MonitorIDs() (out map[string]int64, err error) {
	err = r.access(false, func(tx *sql.Tx) error {
		rows, e := tx.Query(`SELECT name,id FROM monitors WHERE tenant_id=? AND retired_at IS NULL AND (?=0 OR workspace_id=?)`, r.ctx.TenantID, r.workspaceScoped, r.workspace)
		if e != nil {
			return e
		}
		defer rows.Close()
		out = map[string]int64{}
		for rows.Next() {
			var name string
			var id int64
			if e = rows.Scan(&name, &id); e != nil {
				return e
			}
			out[name] = id
		}
		return rows.Err()
	})
	return
}

func (r *MonitorRepository) DeleteState(id int64) error {
	return r.access(true, func(tx *sql.Tx) error {
		name, err := r.ownedMonitor(tx, id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM monitor_state WHERE tenant_id=? AND name=? AND workspace_id=(SELECT workspace_id FROM monitors WHERE id=?)`, r.ctx.TenantID, name, id)
		return err
	})
}

func (r *MonitorRepository) MonitorID(name string) (int64, error) {
	var id int64
	err := r.access(false, func(tx *sql.Tx) error {
		return tenantAccessError(tx.QueryRow(`SELECT id FROM monitors WHERE tenant_id=? AND name=? AND retired_at IS NULL AND (?=0 OR workspace_id=?)`, r.ctx.TenantID, name, r.workspaceScoped, r.workspace).Scan(&id))
	})
	return id, err
}
