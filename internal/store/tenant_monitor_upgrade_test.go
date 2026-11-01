package store

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"path/filepath"
	"testing"
	"time"
)

func TestTenantMonitorDeleteAndNames(t *testing.T) {
	s, _, _ := open(t)
	a, b, _ := tenantMonitorRepos(t, s)
	for _, r := range []*MonitorRepository{a, b} {
		if err := r.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
			t.Fatal(err)
		}
	}
	ai, _ := a.MonitorID("same")
	bi, _ := b.MonitorID("same")
	if name, err := b.MonitorName(ai); !errors.Is(err, ErrTenantAccess) || name != "" {
		t.Fatalf("foreign name: %q %v", name, err)
	}
	if name, err := b.MonitorName(bi); err != nil || name != "same" {
		t.Fatalf("own name: %q %v", name, err)
	}
	ids, err := b.MonitorIDs()
	if err != nil || len(ids) != 1 || ids["same"] != bi {
		t.Fatalf("IDs: %+v %v", ids, err)
	}
	for _, r := range []*MonitorRepository{a, b} {
		id, _ := r.MonitorID("same")
		if err := r.SaveState(id, engine.StateRecord{Name: "same", Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = b.DeleteState(ai); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("cross delete: %v", err)
	}
	if err = b.DeleteState(bi); err != nil {
		t.Fatal(err)
	}
	states, err := b.States()
	if err != nil || len(states) != 0 {
		t.Fatalf("delete: %+v %v", states, err)
	}
	states, err = a.States()
	if err != nil || len(states) != 1 {
		t.Fatalf("delete crossed tenant: %+v %v", states, err)
	}
}

// Regression: real v14/v15 databases, exact old IDs/history, atomic rollback,
// the pre-upgrade backup, retry, and idempotent Open (not a synthetic schema).
func TestTenantMonitorMigrationUpgrade(t *testing.T) {
	for _, version := range []int{14, 15} {
		t.Run(string(rune('A'+version-14)), func(t *testing.T) {
			dir := legacyTenantStore(t)
			db, err := openDB(filepath.Join(dir, "core.db"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO monitor_state(name,state,status) VALUES('old monitor','{}','critical')`); err != nil {
				t.Fatal(err)
			}
			if version == 15 {
				if err = migrateDB(&Store{dir: dir, core: db}, "core", coreMigrations[:15]); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			for attempt := 0; attempt < 2; attempt++ {
				s, err := Open(dir, Options{Tool: true})
				if err != nil {
					t.Fatal(err)
				}
				tenant, _ := s.DefaultTenant()
				r, err := s.TenantMonitors(TenantContext{tenant.ID, 11})
				if err != nil {
					t.Fatal(err)
				}
				id, err := r.MonitorID("old monitor")
				if err != nil || id != 71 {
					t.Fatalf("legacy ID %d %v", id, err)
				}
				state, err := r.States()
				if err != nil || len(state) != 1 || state[0].Status != "critical" {
					t.Fatalf("legacy state %+v %v", state, err)
				}
				rounds, err := r.Rounds(71, time.UnixMilli(1), time.UnixMilli(200000), 10)
				if err != nil || len(rounds) != 1 || rounds[0].At.UnixMilli() != 123456 {
					t.Fatalf("legacy history %+v %v", rounds, err)
				}
				s.Close()
			}
		})
	}
}

func TestTenantMonitorMigrationRollback(t *testing.T) {
	dir := legacyTenantStore(t)
	db, err := openDB(filepath.Join(dir, "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Store{dir: dir, core: db}
	if err = migrateDB(s, "core", coreMigrations[:15]); err != nil {
		t.Fatal(err)
	}
	broken := append([]string{}, coreMigrations[:15]...)
	broken = append(broken, tenantMonitorMigration+`; SELECT * FROM force_failure;`)
	if err = migrateDB(s, "core", broken); err == nil {
		t.Fatal("injected migration succeeded")
	}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 15 {
		t.Fatalf("version %d %v", version, err)
	}
	var owner string
	if err = db.QueryRow(`SELECT tenant_id FROM monitors`).Scan(&owner); err == nil {
		t.Fatal("partial column survived rollback")
	}
	backup, err := openDB(filepath.Join(dir, "core.db.bak-15"))
	if err != nil {
		t.Fatal(err)
	}
	if err = backup.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 15 {
		t.Fatalf("backup version %d %v", version, err)
	}
	var id int64
	if err = backup.QueryRow(`SELECT id FROM monitors WHERE name='old monitor'`).Scan(&id); err != nil || id != 71 {
		t.Fatalf("backup ID %d %v", id, err)
	}
	backup.Close()
	if err = migrateDB(s, "core", coreMigrations); err != nil {
		t.Fatal(err)
	}
	if err = migrateDB(s, "core", coreMigrations); err != nil {
		t.Fatal(err)
	}
}

func TestTenantMonitorRestartAndDefaultRename(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	opts := Options{Tool: true, Now: func() time.Time { return now }}
	s, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	a, b, cb := tenantMonitorRepos(t, s)
	for _, r := range []*MonitorRepository{a, b} {
		if err = r.SyncMonitors([]MonitorDef{{"same", "h"}, {"renamed", "h2"}}); err != nil {
			t.Fatal(err)
		}
	}
	ai, _ := a.MonitorID("same")
	bi, _ := b.MonitorID("same")
	br, _ := b.MonitorID("renamed")
	if err = b.SaveState(bi, engine.StateRecord{Name: "same", Status: "critical"}); err != nil {
		t.Fatal(err)
	}
	for j := 0; j < 2; j++ {
		if err = b.Verdict(bi, engine.VerdictRecord{Monitor: "same", Status: "critical", Hard: true, At: now.Add(time.Duration(j-2) * time.Minute), Interval: time.Minute}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.RenameMonitor("same", "renamed"); err != nil {
		t.Fatal(err)
	}
	if id, e := b.MonitorID("same"); e != nil || id != bi {
		t.Fatalf("cross rename %d %v", id, e)
	}
	if id, e := b.MonitorID("renamed"); e != nil || id != br {
		t.Fatalf("cross replacement %d %v", id, e)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.TenantMonitors(cb)
	if err != nil {
		t.Fatal(err)
	}
	if id, e := b.MonitorID("same"); e != nil || id != bi {
		t.Fatalf("restart ID %d %v", id, e)
	}
	if s.MonitorID("renamed") != ai || s.MonitorName(bi) != "" {
		t.Fatal("restart legacy cache")
	}
	state, err := b.States()
	if err != nil || len(state) != 1 || state[0].Status != "critical" {
		t.Fatalf("restart state %+v %v", state, err)
	}
	rows, err := b.Rounds(bi, now.Add(-time.Hour), now, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("restart history %+v %v", rows, err)
	}
	u, err := b.UptimeOf(bi, now.Add(-time.Hour).Truncate(time.Hour), now)
	if err != nil || u.DownS != 60 {
		t.Fatalf("restart aggregate %+v %v", u, err)
	}
	if _, err = s.core.Exec(`UPDATE users SET disabled=1 WHERE id=?`, cb.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = b.MonitorID("same"); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("disabled cache read %v", err)
	}
	if err = b.Verdict(bi, engine.VerdictRecord{Monitor: "same"}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("disabled history write %v", err)
	}
}

func TestTenantMonitorAuthorizationMatrix(t *testing.T) {
	s, _, _ := open(t)
	a, b, cb := tenantMonitorRepos(t, s)
	if err := b.SyncMonitors([]MonitorDef{{"same", "h"}}); err != nil {
		t.Fatal(err)
	}
	id, _ := b.MonitorID("same")
	for _, role := range []string{RoleViewer, RoleOperator, RoleAdmin} {
		if _, err := s.core.Exec(`UPDATE tenant_members SET role=? WHERE tenant_id=?`, role, cb.TenantID); err != nil {
			t.Fatal(err)
		}
		_, readErr := b.MonitorID("same")
		if (role == RoleAdmin && readErr != nil) || (role != RoleAdmin && !errors.Is(readErr, ErrTenantAccess)) {
			t.Fatalf("company-wide %s read %v", role, readErr)
		}
		err := b.Verdict(id, engine.VerdictRecord{Monitor: "same", At: s.now(), Status: "ok"})
		if role == RoleAdmin {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("%s wrote: %v", role, err)
		}
	}
	if err := b.SaveState(id, engine.StateRecord{Name: "same", IncidentID: 123}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("unscoped incident reference accepted %v", err)
	}
	if err := b.SaveState(id, engine.StateRecord{Name: "wrong"}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("wrong state name %v", err)
	}
	if err := b.Verdict(id, engine.VerdictRecord{Monitor: "wrong"}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("wrong verdict name %v", err)
	}
	if _, err := s.core.Exec(`UPDATE users SET areas='restricted' WHERE id=?`, cb.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.States(); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("broadened area restriction %v", err)
	}
	if _, err := s.core.Exec(`UPDATE users SET areas='' WHERE id=?`, cb.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.core.Exec(`DELETE FROM tenant_members WHERE tenant_id=?`, cb.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncMonitors(nil); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("revoked sync %v", err)
	}
	if _, err := b.Rounds(id, time.Time{}, s.now(), 10); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("revoked history %v", err)
	}
	// A repository cannot be manufactured by zero initialization.
	for _, r := range []*MonitorRepository{nil, {}, {s: s}, {s: s, ctx: TenantContext{TenantID: a.ctx.TenantID}}} {
		if _, err := r.MonitorID("same"); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("missing principal: %v", err)
		}
		if err := r.SyncMonitors(nil); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("missing principal sync: %v", err)
		}
	}
	// Reads do not leak non-default registry rows through the legacy cache.
	var n int
	if err := s.core.QueryRow(`SELECT COUNT(*) FROM monitors WHERE tenant_id=?`, cb.TenantID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rejected sync mutated DB: %d %v", n, err)
	}
}
