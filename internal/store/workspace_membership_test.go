package store

import (
	"errors"
	"testing"
)

func TestWorkspaceMembershipWriteFailureAndNoBroaderGrants(t *testing.T) {
	s, _, _ := open(t)
	owner, e := s.CreateUser("owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	uid, e := s.CreateUserRole("member", "password", RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	outsider, e := s.CreateUserRole("outsider", "password", RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	tenant, _ := s.DefaultTenant()
	ctx := TenantContext{tenant.ID, owner}
	w, e := s.CreateWorkspace(ctx, "A")
	if e != nil {
		t.Fatal(e)
	}
	wc := WorkspaceContext{ctx, w.ID}
	if e = s.SetWorkspaceMember(wc, uid, RoleViewer, false); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`CREATE TRIGGER fail_grant BEFORE UPDATE ON workspace_members BEGIN SELECT RAISE(ABORT,'injected grant failure'); END`); e != nil {
		t.Fatal(e)
	}
	if e = s.SetWorkspaceMember(wc, uid, RoleAdmin, false); e == nil {
		t.Fatal("failed grant reported success")
	}
	members, e := s.WorkspaceMembers(wc)
	if e != nil || len(members) != 1 || members[0].Role != RoleViewer {
		t.Fatalf("failed write broadened %+v %v", members, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER fail_grant`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`DELETE FROM tenant_members WHERE tenant_id=? AND user_id=?`, tenant.ID, outsider); e != nil {
		t.Fatal(e)
	}
	if e = s.SetWorkspaceMember(wc, outsider, RoleAdmin, false); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("non-company grant %v", e)
	}
	// Legacy role synchronization touches only default workspace, never other department grants.
	if _, e = s.core.Exec(`UPDATE users SET role='admin' WHERE id=?`, uid); e != nil {
		t.Fatal(e)
	}
	members, e = s.WorkspaceMembers(wc)
	if e != nil || len(members) != 1 || members[0].Role != RoleViewer {
		t.Fatalf("promoted department grant %+v %v", members, e)
	}
	if _, e = s.core.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); e != nil {
		t.Fatal(e)
	}
	uc := WorkspaceContext{TenantContext{tenant.ID, uid}, w.ID}
	if e = s.RenameWorkspace(uc, "bad"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("retained company admin %v", e)
	}
	if e = s.SetWorkspaceMember(wc, uid, "superuser", false); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("invalid role %v", e)
	}
	if _, e = s.CreateWorkspace(ctx, " "); !errors.Is(e, ErrWorkspaceName) {
		t.Fatalf("empty name %v", e)
	}
	if _, e = s.CreateWorkspace(ctx, "A"); e == nil {
		t.Fatal("duplicate department name")
	}
	if _, e = s.core.Exec(`CREATE TRIGGER fail_workspace BEFORE UPDATE ON workspaces BEGIN SELECT RAISE(ABORT,'injected metadata failure'); END`); e != nil {
		t.Fatal(e)
	}
	if e = s.RenameWorkspace(wc, "changed"); e == nil {
		t.Fatal("failed rename reported success")
	}
	if got, e := s.Workspace(wc); e != nil || got.Name != "A" {
		t.Fatalf("partial metadata %+v %v", got, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER fail_workspace`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`DELETE FROM users WHERE id=?`, uid); e != nil {
		t.Fatal(e)
	}
	members, e = s.WorkspaceMembers(wc)
	if e != nil || len(members) != 0 {
		t.Fatalf("deleted user membership %+v %v", members, e)
	}
}
