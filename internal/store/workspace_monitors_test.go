package store

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"testing"
	"time"
)

func TestWorkspaceRetiredStateOwnership(t *testing.T) {
	s, _, _ := open(t)
	uid, e := s.CreateUser("owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	tenant, _ := s.DefaultTenant()
	ctx := TenantContext{tenant.ID, uid}
	a, e := s.CreateWorkspace(ctx, "A")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CreateWorkspace(ctx, "B")
	if e != nil {
		t.Fatal(e)
	}
	ar, e := s.WorkspaceMonitors(WorkspaceContext{ctx, a.ID})
	if e != nil {
		t.Fatal(e)
	}
	br, e := s.WorkspaceMonitors(WorkspaceContext{ctx, b.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = ar.SyncMonitors([]MonitorDef{{"same", "h"}}); e != nil {
		t.Fatal(e)
	}
	ai, _ := ar.MonitorID("same")
	if e = ar.SaveState(ai, engine.StateRecord{Name: "same", Status: "critical"}); e != nil {
		t.Fatal(e)
	}
	if e = ar.SyncMonitors(nil); e != nil {
		t.Fatal(e)
	}
	if e = br.SyncMonitors([]MonitorDef{{"same", "h"}}); e != nil {
		t.Fatal(e)
	}
	bi, _ := br.MonitorID("same")
	if bi == ai {
		t.Fatal("revived another department ID")
	}
	if e = br.SaveState(bi, engine.StateRecord{Name: "same", Status: "ok"}); e != nil {
		t.Fatal(e)
	}
	if states, e := ar.States(); e != nil || len(states) != 1 || states[0].Status != "critical" {
		t.Fatalf("overwrote A retired state %+v %v", states, e)
	}
	if states, e := br.States(); e != nil || len(states) != 1 || states[0].Status != "ok" {
		t.Fatalf("B state %+v %v", states, e)
	}
	if e = br.DeleteState(bi); e != nil {
		t.Fatal(e)
	}
	if states, e := ar.States(); e != nil || len(states) != 1 {
		t.Fatalf("deleted A state %+v %v", states, e)
	}
}

func TestWorkspaceMonitors(t *testing.T) {
	s, _, _ := open(t)
	ownerID, e := s.CreateUser("owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	uid, e := s.CreateUserRole("department-admin", "password", RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	tenant, _ := s.DefaultTenant()
	owner := TenantContext{tenant.ID, ownerID}
	a, e := s.CreateWorkspace(owner, "A")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CreateWorkspace(owner, "B")
	if e != nil {
		t.Fatal(e)
	}
	ac, bc := WorkspaceContext{owner, a.ID}, WorkspaceContext{owner, b.ID}
	for _, c := range []WorkspaceContext{ac, bc} {
		if e = s.SetWorkspaceMember(c, uid, RoleAdmin, false); e != nil {
			t.Fatal(e)
		}
	}
	ar, e := s.WorkspaceMonitors(ac)
	if e != nil {
		t.Fatal(e)
	}
	br, e := s.WorkspaceMonitors(bc)
	if e != nil {
		t.Fatal(e)
	}
	for i, r := range []*MonitorRepository{ar, br} {
		name := []string{"a", "b"}[i]
		if e = r.SyncMonitors([]MonitorDef{{name, "same-address"}}); e != nil {
			t.Fatal(e)
		}
	}
	ai, _ := ar.MonitorID("a")
	bi, _ := br.MonitorID("b")
	if ai == bi || ai == 0 || bi == 0 {
		t.Fatal("identities shared")
	}
	if e = s.SetWorkspaceMember(bc, uid, RoleViewer, false); e != nil {
		t.Fatal(e)
	}
	ac.UserID = uid
	bc.UserID = uid
	ar, e = s.WorkspaceMonitors(ac)
	if e != nil {
		t.Fatal(e)
	}
	br, e = s.WorkspaceMonitors(bc)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.TenantMonitors(ac.TenantContext); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("company-wide bypass %v", e)
	}
	if _, e = s.WorkspaceMonitors(WorkspaceContext{TenantContext: owner}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("empty wildcard %v", e)
	}
	if e = br.SyncMonitors(nil); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("viewer sync %v", e)
	}
	if e = ar.SyncMonitors([]MonitorDef{{"b", "steal"}}); e == nil {
		t.Fatal("stole B monitor")
	}
	if got, e := ar.MonitorID("a"); e != nil || got != ai {
		t.Fatalf("failed sync non-atomic %d %v", got, e)
	}
	at := s.now().Add(-time.Minute)
	if e = ar.SaveState(ai, engine.StateRecord{Name: "a", Status: "ok"}); e != nil {
		t.Fatal(e)
	}
	if e = ar.Verdict(ai, engine.VerdictRecord{Monitor: "a", Status: "ok", At: at}); e != nil {
		t.Fatal(e)
	}
	for _, fn := range []func() error{
		func() error { _, e := ar.MonitorName(bi); return e },
		func() error { _, e := ar.MonitorID("b"); return e },
		func() error { return ar.SaveState(bi, engine.StateRecord{Name: "b"}) },
		func() error { return ar.DeleteState(bi) },
		func() error { _, e := br.Rounds(ai, at.Add(-time.Second), s.now(), 10); return e },
		func() error { _, e := br.Cells(ai, at, s.now(), 1); return e },
		func() error { _, e := br.Latency(ai, at, s.now()); return e },
		func() error { _, e := br.UptimeOf(ai, at, s.now()); return e },
	} {
		if e = fn(); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("guessed ID %v", e)
		}
	}
	if rows, e := ar.Rounds(ai, at.Add(-time.Second), s.now(), 10); e != nil || len(rows) != 1 {
		t.Fatalf("history %v %v", rows, e)
	}
	if states, e := br.States(); e != nil || len(states) != 0 {
		t.Fatalf("state leak %+v %v", states, e)
	}
	if ids, e := ar.MonitorIDs(); e != nil || len(ids) != 1 || ids["a"] != ai {
		t.Fatalf("IDs %v %v", ids, e)
	}
	if s.MonitorID("a") != 0 || s.MonitorName(ai) != "" {
		t.Fatal("legacy cache leak")
	}
	s.SaveState(engine.StateRecord{Name: "a", Status: "critical"})
	s.DeleteState("a")
	if states, e := ar.States(); e != nil || len(states) != 1 || states[0].Status != "ok" {
		t.Fatalf("legacy state crossing %+v %v", states, e)
	}
	if e = s.SyncMonitors([]MonitorDef{{"legacy", "h"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = ar.MonitorID("a"); e != nil {
		t.Fatalf("legacy retired A %v", e)
	}
	if e = ar.SyncMonitors(nil); e != nil {
		t.Fatal(e)
	}
	if got, e := br.MonitorID("b"); e != nil || got != bi {
		t.Fatalf("A retired B %d %v", got, e)
	}
	if e = s.RemoveWorkspaceMember(WorkspaceContext{owner, a.ID}, uid); e != nil {
		t.Fatal(e)
	}
	if _, e = ar.MonitorIDs(); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("stale grant %v", e)
	}
}
