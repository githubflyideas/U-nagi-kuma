package store

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"testing"
	"time"
)

func tenantMonitorRepos(t *testing.T, s *Store) (*MonitorRepository, *MonitorRepository, TenantContext) {
	t.Helper()
	uid, err := s.CreateUser("tenant-admin", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	b := TenantID("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if _, err = s.core.Exec(`INSERT INTO tenants(id,name) VALUES(?,'B')`, b); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`INSERT INTO tenant_members(tenant_id,user_id,role,disabled) VALUES(?,?,'admin',0)`, b, uid); err != nil {
		t.Fatal(err)
	}
	ca, cb := TenantContext{a.ID, uid}, TenantContext{b, uid}
	ra, err := s.TenantMonitors(ca)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := s.TenantMonitors(cb)
	if err != nil {
		t.Fatal(err)
	}
	return ra, rb, cb
}

func TestTenantMonitorIdentity(t *testing.T) {
	s, _, _ := open(t)
	a, b, cb := tenantMonitorRepos(t, s)
	for _, r := range []*MonitorRepository{a, b} {
		if err := r.SyncMonitors([]MonitorDef{{"same", "identical-target"}, {"second", "identical-target"}}); err != nil {
			t.Fatal(err)
		}
	}
	ai, err := a.MonitorID("same")
	if err != nil {
		t.Fatal(err)
	}
	bi, err := b.MonitorID("same")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.MonitorID("second")
	if err != nil {
		t.Fatal(err)
	}
	if ai == 0 || bi == 0 || ai == bi || bi == second {
		t.Fatalf("deduped identities: %d %d %d", ai, bi, second)
	}
	if s.MonitorID("same") != ai || s.MonitorName(bi) != "" {
		t.Fatal("legacy cache is not default-only")
	}
	if err = a.SyncMonitors(nil); err != nil {
		t.Fatal(err)
	}
	if got, err := b.MonitorID("same"); err != nil || got != bi {
		t.Fatalf("cross-tenant retirement %d %v", got, err)
	}
	if _, err = s.TenantMonitors(TenantContext{}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("missing context %v", err)
	}
	cb.UserID++
	if _, err = s.TenantMonitors(cb); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("unauthorized fallback %v", err)
	}
}

func TestTenantMonitorStates(t *testing.T) {
	s, _, _ := open(t)
	a, b, cb := tenantMonitorRepos(t, s)
	for _, r := range []*MonitorRepository{a, b} {
		if err := r.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
			t.Fatal(err)
		}
	}
	ai, _ := a.MonitorID("same")
	bi, _ := b.MonitorID("same")
	if err := a.SaveState(ai, engine.StateRecord{Name: "same", Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := b.SaveState(bi, engine.StateRecord{Name: "same", Status: "critical"}); err != nil {
		t.Fatal(err)
	}
	if err := b.SaveState(ai, engine.StateRecord{Name: "same", Status: "paused"}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("wrong ID: %v", err)
	}
	states, err := s.States()
	if err != nil || len(states) != 1 || states[0].Status != "ok" {
		t.Fatalf("legacy state leak: %+v %v", states, err)
	}
	s.DeleteState("same")
	states, err = b.States()
	if err != nil || len(states) != 1 || states[0].Status != "critical" {
		t.Fatalf("cross delete: %+v %v", states, err)
	}
	if _, err = s.core.Exec(`UPDATE tenant_members SET role='viewer' WHERE tenant_id=?`, cb.TenantID); err != nil {
		t.Fatal(err)
	}
	if err = b.SaveState(bi, engine.StateRecord{Name: "same", Status: "ok"}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("stale role write: %v", err)
	}
	if _, err = b.States(); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("company-wide viewer read: %v", err)
	}
	if _, err = s.core.Exec(`DELETE FROM tenant_members WHERE tenant_id=?`, cb.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err = b.States(); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("revoked read: %v", err)
	}
}

func TestTenantMonitorHistory(t *testing.T) {
	s, _, _ := open(t)
	a, b, _ := tenantMonitorRepos(t, s)
	start := s.now().Add(-time.Hour).Truncate(time.Hour)
	for _, r := range []*MonitorRepository{a, b} {
		if err := r.SyncMonitors([]MonitorDef{{"same", "same-target"}}); err != nil {
			t.Fatal(err)
		}
	}
	ai, _ := a.MonitorID("same")
	bi, _ := b.MonitorID("same")
	for i, r := range []*MonitorRepository{a, b} {
		id := ai
		status := "ok"
		lat := 10.0
		if i == 1 {
			id = bi
			status = "critical"
			lat = 80
		}
		for j := 0; j < 2; j++ {
			v := engine.VerdictRecord{Monitor: "same", At: start.Add(time.Duration(j) * time.Minute), Status: status, Hard: true, LatencyMS: lat, Interval: time.Minute}
			if err := r.Verdict(id, v); err != nil {
				t.Fatal(err)
			}
		}
		rounds, err := r.Rounds(id, start, start.Add(time.Hour), 10)
		if err != nil || len(rounds) != 2 || rounds[0].Status != status {
			t.Fatalf("rounds %+v %v", rounds, err)
		}
		u, err := r.UptimeOf(id, start, start.Add(time.Hour))
		if err != nil || u.UpS+u.DownS != 60 || (i == 0 && u.DownS != 0) || (i == 1 && u.UpS != 0) {
			t.Fatalf("aggregate %+v %v", u, err)
		}
		pts, err := r.Latency(id, start, start.Add(time.Hour))
		if err != nil || len(pts) != 2 || pts[0].Avg != lat {
			t.Fatalf("latency %+v %v", pts, err)
		}
		cells, err := r.Cells(id, start, start.Add(time.Hour), 1)
		if err != nil || len(cells) != 1 || cells[0].Status != status {
			t.Fatalf("cells %+v %v", cells, err)
		}
	}
	for _, read := range []func() error{
		func() error { _, e := b.Rounds(ai, start, start.Add(time.Hour), 10); return e },
		func() error { _, e := b.UptimeOf(ai, start, start.Add(time.Hour)); return e },
		func() error { _, e := b.Latency(ai, start, start.Add(time.Hour)); return e },
		func() error { _, e := b.Cells(ai, start, start.Add(time.Hour), 1); return e },
	} {
		if err := read(); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("cross-ID history: %v", err)
		}
	}
	if err := b.Verdict(ai, engine.VerdictRecord{Monitor: "same", At: start, Status: "paused"}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("cross-ID write: %v", err)
	}
	legacy, err := s.Rounds("same", start, start.Add(time.Hour), 10)
	if err != nil || len(legacy) != 2 || legacy[0].Status != "ok" {
		t.Fatalf("default history %+v %v", legacy, err)
	}
}

func TestTenantMonitorHistoryAtomicFailure(t *testing.T) {
	s, _, _ := open(t)
	_, b, _ := tenantMonitorRepos(t, s)
	if err := b.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
		t.Fatal(err)
	}
	id, _ := b.MonitorID("same")
	at := s.now().Add(-time.Hour).Truncate(time.Hour)
	v := engine.VerdictRecord{Monitor: "same", At: at, Status: "ok", Interval: time.Minute, LatencyMS: 10}
	if err := b.Verdict(id, v); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"rounds", "agg5m", "hourly"} {
		if _, err := s.hist.Exec(`CREATE TRIGGER reject_history BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT,'injected disk error'); END`); err != nil {
			t.Fatal(err)
		}
		v.At = at.Add(time.Minute)
		if err := b.Verdict(id, v); err == nil {
			t.Fatalf("%s write failure reported success", table)
		}
		rows, err := b.Rounds(id, at, at.Add(time.Hour), 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("partial commit %s %+v %v", table, rows, err)
		}
		if _, err := s.hist.Exec(`DROP TRIGGER reject_history`); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Verdict(id, v); err != nil {
		t.Fatal(err)
	}
	u, err := b.UptimeOf(id, at, at.Add(time.Hour))
	if err != nil || u.UpS != 60 {
		t.Fatalf("failed write polluted aggregation cache: %+v %v", u, err)
	}
}

func TestTenantMonitorSchema(t *testing.T) {
	s, _, _ := open(t)
	tenant, err := s.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SyncMonitors([]MonitorDef{{"same", "same-target"}}); err != nil {
		t.Fatal(err)
	}
	id := s.MonitorID("same")
	var owner TenantID
	if err = s.core.QueryRow(`SELECT tenant_id FROM monitors WHERE id=?`, id).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != tenant.ID {
		t.Fatalf("owner %s", owner)
	}
	const other = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err = s.core.Exec(`INSERT INTO tenants(id,name) VALUES(?,'other')`, other); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`INSERT INTO monitors(tenant_id,name,def_hash,created_at) VALUES(?,'same','same-target',1)`, other); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`INSERT INTO monitors(tenant_id,name,created_at) VALUES(?,'same',1)`, other); err == nil {
		t.Fatal("duplicate active name accepted")
	}
	if _, err = s.core.Exec(`UPDATE monitors SET tenant_id=? WHERE id=?`, other, id); err == nil {
		t.Fatal("ownership mutation accepted")
	}
}
