package store

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceMigrationRollbackAndReopen(t *testing.T) {
	dir := legacyTenantStore(t)
	db, e := openDB(filepath.Join(dir, "core.db"))
	if e != nil {
		t.Fatal(e)
	}
	s := &Store{dir: dir, core: db}
	if e = migrateDB(s, "core", coreMigrations[:16]); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO monitor_state(tenant_id,name,state,status) SELECT id,'old monitor','{}','critical' FROM tenants WHERE is_default=1`); e != nil {
		t.Fatal(e)
	}
	broken := append([]string{}, coreMigrations[:16]...)
	broken = append(broken, workspaceMigration+`; SELECT * FROM injected_failure;`)
	if e = migrateDB(s, "core", broken); e == nil {
		t.Fatal("migration accepted failure")
	}
	var version, n int
	if e = db.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 16 {
		t.Fatalf("version %d %v", version, e)
	}
	if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('workspaces','workspace_members','monitor_state_v16')`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial migration %d %v", n, e)
	}
	if e = db.QueryRow(`SELECT workspace_id FROM monitors`).Scan(&n); e == nil {
		t.Fatal("partial monitor column")
	}
	backup, e := openDB(filepath.Join(dir, "core.db.bak-16"))
	if e != nil {
		t.Fatal(e)
	}
	if e = backup.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 16 {
		t.Fatalf("backup version %d %v", version, e)
	}
	if e = backup.QueryRow(`SELECT id FROM monitors WHERE name='old monitor'`).Scan(&n); e != nil || n != 71 {
		t.Fatalf("backup ID %d %v", n, e)
	}
	backup.Close()
	db.Close()
	var previous WorkspaceID
	for attempt := 0; attempt < 2; attempt++ {
		st, e := Open(dir, Options{Tool: true})
		if e != nil {
			t.Fatal(e)
		}
		tenant, e := st.DefaultTenant()
		if e != nil {
			t.Fatal(e)
		}
		ws, e := st.Workspaces(TenantContext{tenant.ID, 11})
		if e != nil || len(ws) != 1 || !ws[0].IsDefault {
			t.Fatalf("default %+v %v", ws, e)
		}
		if previous != "" && previous != ws[0].ID {
			t.Fatal("workspace identity changed")
		}
		previous = ws[0].ID
		for _, want := range []struct {
			uid  int64
			role string
		}{{11, RoleAdmin}, {12, RoleOperator}, {13, RoleViewer}, {14, RoleAdmin}} {
			var role string
			if e = st.core.QueryRow(`SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?`, previous, want.uid).Scan(&role); e != nil || role != want.role {
				t.Fatalf("backfill privilege %d %s %v", want.uid, role, e)
			}
		}
		r, e := st.WorkspaceMonitors(WorkspaceContext{TenantContext{tenant.ID, 13}, previous})
		if e != nil {
			t.Fatal(e)
		}
		if id, e := r.MonitorID("old monitor"); e != nil || id != 71 {
			t.Fatalf("ID %d %v", id, e)
		}
		if states, e := r.States(); e != nil || len(states) != 1 || states[0].Status != "critical" {
			t.Fatalf("state %+v %v", states, e)
		}
		if rows, e := r.Rounds(71, time.UnixMilli(1), time.UnixMilli(200000), 10); e != nil || len(rows) != 1 || rows[0].At.UnixMilli() != 123456 {
			t.Fatalf("history %+v %v", rows, e)
		}
		for _, uid := range []int64{12, 14} {
			if _, e = st.WorkspaceMonitors(WorkspaceContext{TenantContext{tenant.ID, uid}, previous}); !errors.Is(e, ErrTenantAccess) {
				t.Fatalf("areas/disabled broadened %d %v", uid, e)
			}
		}
		if e = r.SyncMonitors(nil); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("viewer promoted %v", e)
		}
		if e = st.Close(); e != nil {
			t.Fatal(e)
		}
	}
}

func TestWorkspaceSyncFailureAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, Options{Tool: true})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
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
	if e = ar.SyncMonitors([]MonitorDef{{"a", "h"}}); e != nil {
		t.Fatal(e)
	}
	if e = br.SyncMonitors([]MonitorDef{{"b", "h"}}); e != nil {
		t.Fatal(e)
	}
	ai, _ := ar.MonitorID("a")
	bi, _ := br.MonitorID("b")
	if e = ar.SaveState(ai, engine.StateRecord{Name: "a", Status: "ok"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`CREATE TRIGGER fail_retire BEFORE UPDATE OF retired_at ON monitors BEGIN SELECT RAISE(ABORT,'injected retire failure'); END`); e != nil {
		t.Fatal(e)
	}
	if e = ar.SyncMonitors([]MonitorDef{{"new", "h"}}); e == nil {
		t.Fatal("failed sync reported success")
	}
	if ids, e := ar.MonitorIDs(); e != nil || len(ids) != 1 || ids["a"] != ai {
		t.Fatalf("partial registry %+v %v", ids, e)
	}
	if got, e := br.MonitorID("b"); e != nil || got != bi {
		t.Fatalf("B changed %d %v", got, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER fail_retire`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`CREATE TRIGGER fail_state BEFORE UPDATE ON monitor_state BEGIN SELECT RAISE(ABORT,'injected state failure'); END`); e != nil {
		t.Fatal(e)
	}
	if e = ar.SaveState(ai, engine.StateRecord{Name: "a", Status: "critical"}); e == nil {
		t.Fatal("failed state reported success")
	}
	if states, e := ar.States(); e != nil || len(states) != 1 || states[0].Status != "ok" {
		t.Fatalf("partial state %+v %v", states, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER fail_state`); e != nil {
		t.Fatal(e)
	}
	at := time.Now().Add(-time.Minute)
	if e = ar.Verdict(ai, engine.VerdictRecord{Monitor: "a", Status: "ok", At: at}); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(dir, Options{Tool: true})
	if e != nil {
		t.Fatal(e)
	}
	ar, e = s.WorkspaceMonitors(WorkspaceContext{ctx, a.ID})
	if e != nil {
		t.Fatal(e)
	}
	br, e = s.WorkspaceMonitors(WorkspaceContext{ctx, b.ID})
	if e != nil {
		t.Fatal(e)
	}
	if id, e := ar.MonitorID("a"); e != nil || id != ai {
		t.Fatalf("restart ID %d %v", id, e)
	}
	if rows, e := ar.Rounds(ai, at.Add(-time.Second), time.Now(), 10); e != nil || len(rows) != 1 {
		t.Fatalf("restart history %+v %v", rows, e)
	}
	if s.MonitorID("a") != 0 || s.MonitorID("b") != 0 {
		t.Fatal("restart legacy cache leak")
	}
	if e = ar.SyncMonitors(nil); e != nil {
		t.Fatal(e)
	}
	if id, e := br.MonitorID("b"); e != nil || id != bi {
		t.Fatalf("retired B after reopen %d %v", id, e)
	}
}
