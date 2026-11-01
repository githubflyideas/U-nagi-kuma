package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

type WorkspaceID string

// WorkspaceContext is trusted principal plumbing, not authentication. Empty is never a wildcard.
type WorkspaceContext struct {
	TenantContext
	WorkspaceID WorkspaceID
}
type Workspace struct {
	ID        WorkspaceID
	TenantID  TenantID
	Name      string
	IsDefault bool
}
type WorkspaceMember struct {
	WorkspaceContext
	Role     string
	Disabled bool
}

var ErrWorkspaceName = errors.New("workspace name must not be empty")

func companyRole(tx *sql.Tx, ctx TenantContext) (string, error) {
	if ctx.TenantID == "" || ctx.UserID <= 0 {
		return "", ErrTenantAccess
	}
	var role, areas string
	err := tx.QueryRow(`SELECT m.role,u.areas FROM tenant_members m JOIN users u ON u.id=m.user_id WHERE m.tenant_id=? AND m.user_id=? AND m.disabled=0 AND u.disabled=0`, ctx.TenantID, ctx.UserID).Scan(&role, &areas)
	if err != nil {
		return "", tenantAccessError(err)
	}
	if len(splitAreas(areas)) != 0 || (role != RoleAdmin && role != RoleOperator && role != RoleViewer) {
		return "", ErrTenantAccess
	}
	return role, nil
}
func authorizeWorkspace(tx *sql.Tx, ctx WorkspaceContext, write bool) error {
	if ctx.WorkspaceID == "" {
		return ErrTenantAccess
	}
	role, err := companyRole(tx, ctx.TenantContext)
	if err != nil {
		return err
	}
	var exists int
	if err = tx.QueryRow(`SELECT 1 FROM workspaces WHERE tenant_id=? AND id=?`, ctx.TenantID, ctx.WorkspaceID).Scan(&exists); err != nil {
		return tenantAccessError(err)
	}
	if role == RoleAdmin {
		return nil
	}
	if err = tx.QueryRow(`SELECT role FROM workspace_members WHERE tenant_id=? AND workspace_id=? AND user_id=? AND disabled=0`, ctx.TenantID, ctx.WorkspaceID, ctx.UserID).Scan(&role); err != nil {
		return tenantAccessError(err)
	}
	if write && role != RoleAdmin {
		return ErrTenantAccess
	}
	return nil
}
func (s *Store) workspaceTx(ctx WorkspaceContext, write bool, fn func(*sql.Tx) error) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	tx, err := s.core.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = authorizeWorkspace(tx, ctx, write); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if write {
		s.fenceWorkspaceExecutions(ctx.TenantID, ctx.WorkspaceID)
	}
	return nil
}
func (s *Store) companyTx(ctx TenantContext, admin bool, fn func(*sql.Tx, string) error) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	tx, err := s.core.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	role, err := companyRole(tx, ctx)
	if err != nil {
		return err
	}
	if admin && role != RoleAdmin {
		return ErrTenantAccess
	}
	if err = fn(tx, role); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreateWorkspace(ctx TenantContext, name string) (out Workspace, err error) {
	if strings.TrimSpace(name) == "" {
		return out, ErrWorkspaceName
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return
	}
	out = Workspace{ID: WorkspaceID(hex.EncodeToString(id[:])), TenantID: ctx.TenantID, Name: name}
	err = s.companyTx(ctx, true, func(tx *sql.Tx, _ string) error {
		_, e := tx.Exec(`INSERT INTO workspaces(id,tenant_id,name,is_default) VALUES(?,?,?,0)`, out.ID, out.TenantID, out.Name)
		return e
	})
	if err != nil {
		out = Workspace{}
	}
	return
}
func (s *Store) Workspace(ctx WorkspaceContext) (out Workspace, err error) {
	err = s.workspaceTx(ctx, false, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT id,tenant_id,name,is_default FROM workspaces WHERE tenant_id=? AND id=?`, ctx.TenantID, ctx.WorkspaceID).Scan(&out.ID, &out.TenantID, &out.Name, &out.IsDefault)
	})
	return
}

// Workspaces lists explicit grants, or every department for an active company admin.
func (s *Store) Workspaces(ctx TenantContext) (out []Workspace, err error) {
	err = s.companyTx(ctx, false, func(tx *sql.Tx, role string) error {
		rows, e := tx.Query(`SELECT id,tenant_id,name,is_default FROM workspaces w WHERE tenant_id=? AND (?='admin' OR EXISTS(SELECT 1 FROM workspace_members m WHERE m.workspace_id=w.id AND m.tenant_id=w.tenant_id AND m.user_id=? AND m.disabled=0)) ORDER BY name,id`, ctx.TenantID, role, ctx.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var w Workspace
			if e = rows.Scan(&w.ID, &w.TenantID, &w.Name, &w.IsDefault); e != nil {
				return e
			}
			out = append(out, w)
		}
		return rows.Err()
	})
	return
}
func (s *Store) RenameWorkspace(ctx WorkspaceContext, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrWorkspaceName
	}
	return s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		_, e := tx.Exec(`UPDATE workspaces SET name=? WHERE tenant_id=? AND id=?`, name, ctx.TenantID, ctx.WorkspaceID)
		return e
	})
}

// DeleteWorkspace refuses default/nonempty workspaces; stable histories are never destroyed.
func (s *Store) DeleteWorkspace(ctx WorkspaceContext) error {
	return s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		_, e := tx.Exec(`DELETE FROM workspaces WHERE tenant_id=? AND id=?`, ctx.TenantID, ctx.WorkspaceID)
		return e
	})
}
func (s *Store) SetWorkspaceMember(ctx WorkspaceContext, userID int64, role string, disabled bool) error {
	return s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		if role != RoleAdmin && role != RoleOperator && role != RoleViewer {
			return ErrTenantAccess
		}
		if _, e := companyRole(tx, TenantContext{ctx.TenantID, userID}); e != nil {
			return e
		}
		_, e := tx.Exec(`INSERT INTO workspace_members(tenant_id,workspace_id,user_id,role,disabled) VALUES(?,?,?,?,?) ON CONFLICT(workspace_id,user_id) DO UPDATE SET role=excluded.role,disabled=excluded.disabled`, ctx.TenantID, ctx.WorkspaceID, userID, role, disabled)
		return e
	})
}
func (s *Store) RemoveWorkspaceMember(ctx WorkspaceContext, userID int64) error {
	return s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		_, e := tx.Exec(`DELETE FROM workspace_members WHERE tenant_id=? AND workspace_id=? AND user_id=?`, ctx.TenantID, ctx.WorkspaceID, userID)
		return e
	})
}
func (s *Store) WorkspaceMembers(ctx WorkspaceContext) (out []WorkspaceMember, err error) {
	err = s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		rows, e := tx.Query(`SELECT user_id,role,disabled FROM workspace_members WHERE tenant_id=? AND workspace_id=? ORDER BY user_id`, ctx.TenantID, ctx.WorkspaceID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			m := WorkspaceMember{WorkspaceContext: ctx}
			if e = rows.Scan(&m.UserID, &m.Role, &m.Disabled); e != nil {
				return e
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return
}
