package store

import (
	"database/sql"
	"errors"
	"sync"
)

// Monitors have a stable numeric id. The configuration names them, so the
// id follows the name: a rename carries it (and the history keyed by it)
// over, a deleted monitor's id is retired, and a monitor that comes back
// with the same definition (a rollback) gets its old id and history back.

// MonitorDef is what SyncMonitors needs of a configured monitor.
type MonitorDef struct {
	Name    string
	DefHash string
}

type monitorIDs struct {
	mu       sync.Mutex
	byTenant map[TenantID]*monitorCache
}

type monitorCache struct {
	byName  map[string]int64
	aliases map[string]string // old name → new name, for rounds still queued under the old one
}

// MonitorID is a trusted default-tenant compatibility lookup (0 when unknown).
func (s *Store) MonitorID(name string) int64 {
	s.ids.mu.Lock()
	defer s.ids.mu.Unlock()
	return s.ids.byTenant[s.defaultTenant].byName[s.ids.byTenant[s.defaultTenant].resolve(name)]
}

// MonitorName resolves only active default-tenant IDs ("" for foreign/unknown).
func (s *Store) MonitorName(id int64) string {
	s.ids.mu.Lock()
	defer s.ids.mu.Unlock()
	for name, v := range s.ids.byTenant[s.defaultTenant].byName {
		if v == id {
			return name
		}
	}
	return ""
}

// MonitorIDs returns only active default-tenant monitor IDs.
func (s *Store) MonitorIDs() map[string]int64 {
	s.ids.mu.Lock()
	defer s.ids.mu.Unlock()
	out := make(map[string]int64, len(s.ids.byTenant[s.defaultTenant].byName))
	for k, v := range s.ids.byTenant[s.defaultTenant].byName {
		out[k] = v
	}
	return out
}

func (m *monitorCache) resolve(name string) string {
	for i := 0; i < 8; i++ { // chains of renames; bounded against loops
		next, ok := m.aliases[name]
		if !ok {
			break
		}
		name = next
	}
	return name
}

func (s *Store) loadMonitorIDs() error {
	t, err := s.DefaultTenant()
	if err != nil {
		return err
	}
	s.defaultTenant = t.ID
	rows, err := s.core.Query(`SELECT tenant_id,id,name FROM monitors WHERE retired_at IS NULL AND workspace_id IN (SELECT id FROM workspaces WHERE is_default=1)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	s.ids.byTenant = map[TenantID]*monitorCache{s.defaultTenant: {byName: map[string]int64{}, aliases: map[string]string{}}}
	for rows.Next() {
		var tenant TenantID
		var id int64
		var name string
		if err = rows.Scan(&tenant, &id, &name); err != nil {
			return err
		}
		c := s.ids.byTenant[tenant]
		if c == nil {
			c = &monitorCache{byName: map[string]int64{}, aliases: map[string]string{}}
			s.ids.byTenant[tenant] = c
		}
		c.byName[name] = id
	}
	return rows.Err()
}

// idFor returns the id of a monitor, creating it when the name is new
// (rounds can arrive before the configuration that names it is synced).
func (s *Store) idFor(name string) (int64, error) {
	s.ids.mu.Lock()
	name = s.ids.byTenant[s.defaultTenant].resolve(name)
	id, ok := s.ids.byTenant[s.defaultTenant].byName[name]
	s.ids.mu.Unlock()
	if ok {
		return id, nil
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	tx, err := s.core.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id, err = activeOrNew(tx, s.defaultTenant, name, "", ms(s.now()))
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.ids.mu.Lock()
	s.ids.byTenant[s.defaultTenant].byName[name] = id
	s.ids.mu.Unlock()
	return id, nil
}

// activeOrNew returns the active id of name, reviving a retired one with
// the same definition, or inserting a new one.
func activeOrNew(tx *sql.Tx, tenant TenantID, name, hash string, now int64) (int64, error) {
	var workspace WorkspaceID
	if err := tx.QueryRow(`SELECT id FROM workspaces WHERE tenant_id=? AND is_default=1`, tenant).Scan(&workspace); err != nil {
		return 0, err
	}
	return activeOrNewWorkspace(tx, tenant, workspace, name, hash, now)
}
func activeOrNewWorkspace(tx *sql.Tx, tenant TenantID, workspace WorkspaceID, name, hash string, now int64) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM monitors WHERE name=? AND retired_at IS NULL AND tenant_id=? AND workspace_id=?`, name, tenant, workspace).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if hash != "" {
		err = tx.QueryRow(`SELECT id FROM monitors WHERE tenant_id=? AND name=? AND def_hash=? AND workspace_id=? AND retired_at IS NOT NULL ORDER BY retired_at DESC LIMIT 1`, tenant, name, hash, workspace).Scan(&id)
		if err == nil {
			_, err = tx.Exec(`UPDATE monitors SET retired_at=NULL WHERE id=?`, id)
			return id, err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	res, err := tx.Exec(`INSERT INTO monitors(tenant_id,workspace_id,name, def_hash, created_at) VALUES(?,?,?,?,?)`, tenant, workspace, name, hash, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SyncMonitors is the default-tenant runtime compatibility adapter:
// new names get an id (or their retired one back), missing ones retire.
func (s *Store) SyncMonitors(defs []MonitorDef) error {
	return s.syncMonitors(s.defaultTenant, nil, defs)
}

func (s *Store) syncMonitors(tenant TenantID, ctx *TenantContext, defs []MonitorDef) error {
	return s.syncWorkspaceMonitors(tenant, ctx, "", defs)
}
func (s *Store) syncWorkspaceMonitors(tenant TenantID, ctx *TenantContext, workspace WorkspaceID, defs []MonitorDef) error {
	return s.syncWorkspaceMonitorsPublish(tenant, ctx, workspace, defs, nil)
}

func (s *Store) syncWorkspaceMonitorsPublish(tenant TenantID, ctx *TenantContext, workspace WorkspaceID, defs []MonitorDef, publish func(*sql.Tx) error) error {
	if (ctx == nil && tenant != s.defaultTenant) || (ctx != nil && ctx.TenantID != tenant) {
		return ErrTenantAccess
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	tx, err := s.core.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var defaultWorkspace WorkspaceID
	if err = tx.QueryRow(`SELECT id FROM workspaces WHERE tenant_id=? AND is_default=1`, tenant).Scan(&defaultWorkspace); err != nil {
		return tenantAccessError(err)
	}
	if ctx != nil {
		if workspace != "" {
			err = authorizeWorkspace(tx, WorkspaceContext{*ctx, workspace}, true)
		} else {
			err = authorizeMonitors(tx, *ctx, true)
		}
		if err != nil {
			return err
		}
	} else {
		if workspace != "" && workspace != defaultWorkspace {
			return ErrTenantAccess
		}
		workspace = defaultWorkspace
	}
	newWorkspace := workspace
	if newWorkspace == "" {
		newWorkspace = defaultWorkspace
	}
	now := ms(s.now())
	active := map[string]int64{}
	hashes := map[string]string{}
	rows, err := tx.Query(`SELECT id, name, def_hash FROM monitors WHERE retired_at IS NULL AND tenant_id=? AND (?='' OR workspace_id=?)`, tenant, workspace, workspace)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var name, hash string
		if err := rows.Scan(&id, &name, &hash); err != nil {
			rows.Close()
			return err
		}
		active[name], hashes[name] = id, hash
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	ids := make(map[string]int64, len(defs))
	for _, d := range defs {
		if id, ok := active[d.Name]; ok {
			if hashes[d.Name] != d.DefHash {
				if _, err := tx.Exec(`UPDATE monitors SET def_hash=? WHERE id=?`, d.DefHash, id); err != nil {
					return err
				}
			}
			ids[d.Name] = id
			delete(active, d.Name)
			continue
		}
		id, err := activeOrNewWorkspace(tx, tenant, newWorkspace, d.Name, d.DefHash, now)
		if err != nil {
			return err
		}
		ids[d.Name] = id
	}
	for _, id := range active {
		if _, err := tx.Exec(`UPDATE monitors SET retired_at=? WHERE id=?`, now, id); err != nil {
			return err
		}
	}
	cacheIDs := map[string]int64{}
	rows, err = tx.Query(`SELECT name,id FROM monitors WHERE tenant_id=? AND workspace_id=? AND retired_at IS NULL`, tenant, defaultWorkspace)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		var id int64
		if err = rows.Scan(&name, &id); err != nil {
			rows.Close()
			return err
		}
		cacheIDs[name] = id
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if publish != nil {
		if err := publish(tx); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.ids.mu.Lock()
	c := s.ids.byTenant[tenant]
	if c == nil {
		c = &monitorCache{aliases: map[string]string{}}
		s.ids.byTenant[tenant] = c
	}
	c.byName = cacheIDs
	s.ids.mu.Unlock()
	return nil
}

// RenameMonitor moves a monitor to a new name: its id, open incidents,
// events, notification log, evidence and push token follow.
func (s *Store) RenameMonitor(oldName, newName string) error {
	s.batch.flushNow()
	s.wmu.Lock()
	defer s.wmu.Unlock()
	tx, err := s.core.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := ms(s.now())
	var foreign int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM monitors WHERE tenant_id=? AND name IN (?,?) AND retired_at IS NULL AND workspace_id NOT IN (SELECT id FROM workspaces WHERE tenant_id=? AND is_default=1)`, s.defaultTenant, oldName, newName, s.defaultTenant).Scan(&foreign); err != nil {
		return err
	}
	if foreign != 0 {
		return ErrTenantAccess
	}
	// A monitor already active under the new name is replaced.
	if _, err := tx.Exec(`UPDATE monitors SET retired_at=? WHERE name=? AND retired_at IS NULL AND tenant_id=?`, now, newName, s.defaultTenant); err != nil {
		return err
	}
	for _, q := range []string{
		`UPDATE monitors SET name=? WHERE name=? AND retired_at IS NULL AND tenant_id=(SELECT id FROM tenants WHERE is_default=1)`,
		`UPDATE incidents SET monitor=? WHERE monitor=?`,
		`UPDATE events SET monitor=? WHERE monitor=?`,
		`UPDATE deliveries SET monitor=? WHERE monitor=?`,
		`UPDATE decisions SET monitor=? WHERE monitor=?`,
		`UPDATE evidence SET monitor=? WHERE monitor=?`,
		`UPDATE push_tokens SET monitor=? WHERE monitor=?`,
	} {
		if _, err := tx.Exec(q, newName, oldName); err != nil {
			return err
		}
	}
	// The push URL stays the same.
	for _, k := range []string{"push_plain:", "push_gen:"} {
		if _, err := tx.Exec(`DELETE FROM settings WHERE key=?`, k+newName); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE settings SET key=? WHERE key=?`, k+newName, k+oldName); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM monitor_state WHERE workspace_id IN (SELECT id FROM workspaces WHERE is_default=1) AND tenant_id=(SELECT id FROM tenants WHERE is_default=1) AND name=?`, newName); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE monitor_state SET name=? WHERE workspace_id IN (SELECT id FROM workspaces WHERE is_default=1) AND tenant_id=(SELECT id FROM tenants WHERE is_default=1) AND name=?`, newName, oldName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.ids.mu.Lock()
	if id, ok := s.ids.byTenant[s.defaultTenant].byName[oldName]; ok {
		delete(s.ids.byTenant[s.defaultTenant].byName, oldName)
		s.ids.byTenant[s.defaultTenant].byName[newName] = id
	}
	s.ids.byTenant[s.defaultTenant].aliases[oldName] = newName
	delete(s.ids.byTenant[s.defaultTenant].aliases, newName)
	s.ids.mu.Unlock()
	return nil
}
