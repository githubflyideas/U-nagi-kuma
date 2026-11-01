package store

import "database/sql"

// SyncDefaultRuntimeSnapshot atomically registers default-runtime definitions
// and its durable restart snapshot. This trusted instance API is not available
// to workspace callers. It never retires another workspace's definitions.
func (s *Store) SyncDefaultRuntimeSnapshot(defs []MonitorDef, key, value string) error {
	if key == "" {
		return ErrTenantAccess
	}
	return s.syncWorkspaceMonitorsPublish(s.defaultTenant, nil, "", defs, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
		return err
	})
}

// CheckDefaultRuntimeRecord rejects unregistered as well as known foreign-only
// names. Unlike configuration admission, runtime output cannot create identity.
// Default rename aliases are resolved to their registered numeric ID first.
func (s *Store) CheckDefaultRuntimeRecord(name string) error {
	if name == "" {
		return nil
	} // legacy instance event, never workspace authority
	if err := s.CheckDefaultRuntimeName(name); err != nil {
		return err
	}
	id := s.MonitorID(name)
	var registered bool
	err := s.core.QueryRow(`SELECT EXISTS(SELECT 1 FROM monitors m JOIN workspaces w ON w.id=m.workspace_id WHERE m.tenant_id=? AND w.is_default=1 AND ((?<>0 AND m.id=?) OR (?=0 AND m.name=?)))`, s.defaultTenant, id, id, id, name).Scan(&registered)
	if err != nil {
		return err
	}
	if !registered {
		return ErrTenantAccess
	}
	return nil
}

// CheckDefaultRuntimeName is an admission preflight for the legacy default
// runtime, NOT a tenant runtime authority. Unknown names may be registered in
// the default workspace. A known foreign-only name may not be interpreted as
// a default monitor. The authoritative check remains SyncMonitors' transaction.
// Empty names identify legacy instance-level events, not a workspace wildcard.
func (s *Store) CheckDefaultRuntimeName(name string) error {
	if name == "" {
		return nil
	}
	var foreign, local bool
	err := s.core.QueryRow(`SELECT
 EXISTS(SELECT 1 FROM monitors m JOIN workspaces w ON w.id=m.workspace_id WHERE m.name=? AND (m.tenant_id<>? OR w.is_default=0)),
 EXISTS(SELECT 1 FROM monitors m JOIN workspaces w ON w.id=m.workspace_id WHERE m.name=? AND m.tenant_id=? AND w.is_default=1 AND m.retired_at IS NULL)`, name, s.defaultTenant, name, s.defaultTenant).Scan(&foreign, &local)
	if err != nil {
		return err
	}
	if foreign && !local {
		return ErrTenantAccess
	}
	return nil
}
