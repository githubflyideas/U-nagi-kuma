package store

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
