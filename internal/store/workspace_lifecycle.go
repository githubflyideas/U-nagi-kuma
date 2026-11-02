package store

import "database/sql"

// WorkspaceRuntimePublication reads metadata under live Workspace read authority.
// It never exposes stored bodies, capability material or secret envelopes.
func (s *Store) WorkspaceRuntimePublication(ctx WorkspaceContext) (state WorkspaceConfigState, active bool, err error) {
	err = s.workspaceTx(ctx, false, func(tx *sql.Tx) error {
		var e error
		state, e = workspaceConfigState(tx, ctx.TenantID, ctx.WorkspaceID)
		if e != nil || state.Revision == 0 {
			return e
		}
		var n int
		e = tx.QueryRow(`SELECT COUNT(*) FROM workspace_secret_readers WHERE tenant_id=? AND workspace_id=? AND generation=? AND active=1 AND token_hash IS NOT NULL`, ctx.TenantID, ctx.WorkspaceID, state.Generation).Scan(&n)
		active = n == 1
		return e
	})
	if err != nil {
		state = WorkspaceConfigState{}
		active = false
	}
	return
}

// AuthorizeWorkspaceDiscovery requires a live installation administrator. Tenant
// or Workspace administrator grants alone confer no installation authority.
func (s *Store) AuthorizeWorkspaceDiscovery(userID int64) error {
	var n int
	if err := s.core.QueryRow(`SELECT COUNT(*) FROM users WHERE id=? AND role='admin' AND disabled=0`, userID).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return ErrTenantAccess
	}
	return nil
}

// PublishedWorkspaceRuntimes enumerates trusted durable service authority for the
// installation supervisor, not a user-facing list API. Discovery is only a hint:
// Restore, admission and every result write recheck the exact live capability.
// Default Workspaces remain exclusively owned by the existing engine.
func (s *Store) PublishedWorkspaceRuntimes() ([]WorkspaceConfigState, error) {
	rows, err := s.core.Query(`SELECT c.tenant_id,c.workspace_id,c.revision,c.generation
 FROM workspace_applied_configs c
 JOIN workspace_secret_readers r ON r.tenant_id=c.tenant_id AND r.workspace_id=c.workspace_id AND r.generation=c.generation
 JOIN workspaces w ON w.tenant_id=c.tenant_id AND w.id=c.workspace_id
 WHERE w.is_default=0 AND r.active=1 AND r.token_hash IS NOT NULL
 ORDER BY c.tenant_id,c.workspace_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkspaceConfigState
	for rows.Next() {
		var v WorkspaceConfigState
		if err := rows.Scan(&v.TenantID, &v.WorkspaceID, &v.Revision, &v.Generation); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
