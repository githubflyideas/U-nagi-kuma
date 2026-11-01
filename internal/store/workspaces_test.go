package store

import (
	"errors"
	"testing"
)

func TestWorkspaceAuthorization(t *testing.T) {
	s, _, _ := open(t)
	admin, err := s.CreateUser("owner", "password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.CreateUserRole("member", "password", RoleViewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant, _ := s.DefaultTenant()
	owner := TenantContext{tenant.ID, admin}
	a, err := s.CreateWorkspace(owner, "A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateWorkspace(owner, "B")
	if err != nil {
		t.Fatal(err)
	}
	ac, bc := WorkspaceContext{owner, a.ID}, WorkspaceContext{owner, b.ID}
	if err = s.SetWorkspaceMember(ac, user, RoleAdmin, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SetWorkspaceMember(bc, user, RoleViewer, false); err != nil {
		t.Fatal(err)
	}
	ac.UserID, bc.UserID = user, user
	if err = s.RenameWorkspace(ac, "A renamed"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []WorkspaceContext{bc, {TenantContext: owner}, {TenantContext: TenantContext{TenantID: "bad", UserID: user}, WorkspaceID: a.ID}} {
		if err = s.RenameWorkspace(c, "forbidden"); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("scope accepted: %+v %v", c, err)
		}
	}
	if err = s.SetWorkspaceMember(bc, user, RoleAdmin, false); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("self promotion %v", err)
	}
	if _, err = s.CreateWorkspace(ac.TenantContext, "C"); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("company bypass %v", err)
	}
	ws, err := s.Workspaces(ac.TenantContext)
	if err != nil || len(ws) != 3 {
		t.Fatalf("list %+v %v", ws, err)
	}
	members, err := s.WorkspaceMembers(ac)
	if err != nil || len(members) != 1 || members[0].Role != RoleAdmin {
		t.Fatalf("members %+v %v", members, err)
	}
	if err = s.RemoveWorkspaceMember(WorkspaceContext{owner, a.ID}, user); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Workspace(ac); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("revoked %v", err)
	}
	if err = s.DeleteWorkspace(WorkspaceContext{owner, a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Workspace(WorkspaceContext{owner, a.ID}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("deleted %v", err)
	}
	if _, err = s.core.Exec(`UPDATE tenant_members SET disabled=1 WHERE tenant_id=? AND user_id=?`, tenant.ID, user); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Workspace(bc); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("disabled company %v", err)
	}
}

func TestWorkspaceSchema(t *testing.T) {
	s, _, _ := open(t)
	uid, err := s.CreateUserRole("viewer", "password", RoleViewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wid, role string
	if err = s.core.QueryRow(`SELECT w.id,m.role FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE w.is_default=1 AND m.user_id=?`, uid).Scan(&wid, &role); err != nil {
		t.Fatal(err)
	}
	if role != RoleViewer {
		t.Fatalf("broadened role: %s", role)
	}
	if err = s.SyncMonitors([]MonitorDef{{"legacy", "h"}}); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err = s.core.QueryRow(`SELECT workspace_id FROM monitors WHERE id=?`, s.MonitorID("legacy")).Scan(&owner); err != nil || owner != wid {
		t.Fatalf("owner %s %v", owner, err)
	}
}
