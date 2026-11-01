package store

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"testing"
)

func TestWorkspaceRetiredNameDefaultReuse(t *testing.T) {
	s, _, _ := open(t)
	uid, e := s.CreateUser("owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	tenant, _ := s.DefaultTenant()
	ctx := TenantContext{tenant.ID, uid}
	w, e := s.CreateWorkspace(ctx, "A")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.WorkspaceMonitors(WorkspaceContext{ctx, w.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.SyncMonitors([]MonitorDef{{"reused", "h"}}); e != nil {
		t.Fatal(e)
	}
	old, _ := r.MonitorID("reused")
	if e = r.SaveState(old, engine.StateRecord{Name: "reused", Status: "critical"}); e != nil {
		t.Fatal(e)
	}
	if e = r.SyncMonitors(nil); e != nil {
		t.Fatal(e)
	}
	if e = s.SyncMonitors([]MonitorDef{{"reused", "h"}}); e != nil {
		t.Fatal(e)
	}
	if s.MonitorID("reused") == old {
		t.Fatal("foreign retired identity revived")
	}
	s.SaveState(engine.StateRecord{Name: "reused", Status: "ok"})
	if states, e := s.States(); e != nil || len(states) != 1 || states[0].Status != "ok" {
		t.Fatalf("default reuse state %+v %v", states, e)
	}
	if states, e := r.States(); e != nil || len(states) != 1 || states[0].Status != "critical" {
		t.Fatalf("foreign retired state %+v %v", states, e)
	}
}

func TestWorkspaceCompanyRepositoryIncidentBoundary(t *testing.T) {
	s, _, _ := open(t)
	uid, e := s.CreateUser("owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	tenant, _ := s.DefaultTenant()
	ctx := TenantContext{tenant.ID, uid}
	w, e := s.CreateWorkspace(ctx, "department")
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.WorkspaceMonitors(WorkspaceContext{ctx, w.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.SyncMonitors([]MonitorDef{{"private", "h"}}); e != nil {
		t.Fatal(e)
	}
	id, _ := r.MonitorID("private")
	company, e := s.TenantMonitors(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = company.SaveState(id, engine.StateRecord{Name: "private", IncidentID: 123}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("unscoped incident accepted %v", e)
	}
}

func TestWorkspaceAdversarialAuthorization(t *testing.T) {
	s, _, _ := open(t)
	owner, e := s.CreateUser("owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	user, e := s.CreateUserRole("user", "password", RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	tenant, _ := s.DefaultTenant()
	ctx := TenantContext{tenant.ID, owner}
	a, e := s.CreateWorkspace(ctx, "A")
	if e != nil {
		t.Fatal(e)
	}
	ac := WorkspaceContext{ctx, a.ID}
	if e = s.SetWorkspaceMember(ac, user, RoleAdmin, false); e != nil {
		t.Fatal(e)
	}
	uc := WorkspaceContext{TenantContext{tenant.ID, user}, a.ID}
	r, e := s.WorkspaceMonitors(uc)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.SyncMonitors([]MonitorDef{{"a", "h"}}); e != nil {
		t.Fatal(e)
	}
	id, _ := r.MonitorID("a")
	if e = s.DeleteWorkspace(ac); e == nil {
		t.Fatal("deleted workspace with stable monitor/history identity")
	}
	for _, role := range []string{RoleViewer, RoleOperator} {
		if e = s.SetWorkspaceMember(ac, user, role, false); e != nil {
			t.Fatal(e)
		}
		if e = s.SetWorkspaceMember(uc, user, RoleAdmin, false); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("%s self promotion %v", role, e)
		}
		if e = r.SyncMonitors(nil); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("%s stale repo write %v", role, e)
		}
		if got, e := r.MonitorID("a"); e != nil || got != id {
			t.Fatalf("%s own read %d %v", role, got, e)
		}
	}
	if e = s.SetWorkspaceMember(ac, user, RoleAdmin, true); e != nil {
		t.Fatal(e)
	}
	if _, e = r.MonitorIDs(); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("disabled workspace %v", e)
	}
	if e = s.SetWorkspaceMember(ac, user, RoleAdmin, false); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"tenant_members", "users"} {
		q := `UPDATE users SET disabled=1 WHERE id=?`
		if table == "tenant_members" {
			q = `UPDATE tenant_members SET disabled=1 WHERE user_id=?`
		}
		if _, e = s.core.Exec(q, user); e != nil {
			t.Fatal(e)
		}
		if e = r.SyncMonitors(nil); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("disabled %s write %v", table, e)
		}
		if _, e = r.States(); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("disabled %s read %v", table, e)
		}
		if _, e = s.core.Exec(`UPDATE users SET disabled=0 WHERE id=?`, user); e != nil {
			t.Fatal(e)
		}
	}
	const foreign = "cccccccccccccccccccccccccccccccc"
	if _, e = s.core.Exec(`INSERT INTO tenants(id,name) VALUES(?,'Foreign')`, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`INSERT INTO tenant_members(tenant_id,user_id,role,disabled) VALUES(?,?,'admin',0)`, foreign, owner); e != nil {
		t.Fatal(e)
	}
	wrong := WorkspaceContext{TenantContext{foreign, owner}, a.ID}
	if _, e = s.WorkspaceMonitors(wrong); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("wrong company %v", e)
	}
	if e = s.SetWorkspaceMember(wrong, user, RoleAdmin, false); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("wrong company grant %v", e)
	}
	for _, q := range []string{
		`UPDATE workspaces SET tenant_id='cccccccccccccccccccccccccccccccc' WHERE name='A'`,
		`UPDATE workspaces SET id='dddddddddddddddddddddddddddddddd' WHERE name='A'`,
		`UPDATE workspaces SET is_default=1 WHERE name='A'`,
		`DELETE FROM workspaces WHERE is_default=1`,
		`UPDATE monitors SET workspace_id=(SELECT id FROM workspaces WHERE tenant_id='cccccccccccccccccccccccccccccccc') WHERE name='a'`,
		`UPDATE monitors SET workspace_id=NULL WHERE name='a'`,
		`UPDATE workspace_members SET tenant_id='cccccccccccccccccccccccccccccccc' WHERE workspace_id=(SELECT id FROM workspaces WHERE name='A')`,
	} {
		if _, e = s.core.Exec(q); e == nil {
			t.Fatalf("identity mutation accepted %s", q)
		}
	}
	if _, e = s.core.Exec(`DELETE FROM tenant_members WHERE tenant_id=? AND user_id=?`, tenant.ID, user); e != nil {
		t.Fatal(e)
	}
	if _, e = r.MonitorIDs(); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("removed company membership %v", e)
	}
	if _, e = s.core.Exec(`INSERT INTO tenant_members(tenant_id,user_id,role,disabled) VALUES(?,?,'viewer',0)`, tenant.ID, user); e != nil {
		t.Fatal(e)
	}
	if _, e = s.WorkspaceMonitors(uc); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("old grant resurrected %v", e)
	}
	// Explicit company administration may enumerate/retire every department.
	company, e := s.TenantMonitors(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if ids, e := company.MonitorIDs(); e != nil || ids["a"] != id {
		t.Fatalf("company view %+v %v", ids, e)
	}
	if e = company.SyncMonitors(nil); e != nil {
		t.Fatal(e)
	}
	if _, e = company.MonitorName(id); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("company retirement %v", e)
	}
}
