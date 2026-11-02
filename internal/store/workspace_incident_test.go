package store

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceIncidentMigrationRollbackReopen(t *testing.T) {
	dir := legacyTenantStore(t)
	db, e := openDB(filepath.Join(dir, "core.db"))
	if e != nil {
		t.Fatal(e)
	}
	s := &Store{dir: dir, core: db}
	if e = migrateDB(s, "core", coreMigrations[:19]); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM monitors`).Scan(&count); e != nil {
		t.Fatal(e)
	}
	broken := append([]string{}, coreMigrations[:19]...)
	broken = append(broken, workspaceIncidentMigration+`; SELECT * FROM injected_incident_failure;`)
	if e = migrateDB(s, "core", broken); e == nil {
		t.Fatal("late migration accepted")
	}
	var version, n int
	if e = db.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 19 {
		t.Fatalf("rollback %d %v", version, e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name LIKE 'workspace_incident%' OR name LIKE 'workspace_notification%' OR name='workspace_events'`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial schema %d %v", n, e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		live, e := Open(dir, Options{Tool: true})
		if e != nil {
			t.Fatal(e)
		}
		if e = live.core.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != len(coreMigrations) {
			t.Fatalf("upgrade %d %v", version, e)
		}
		if e = live.core.QueryRow(`SELECT count(*) FROM monitors`).Scan(&n); e != nil || n != count {
			t.Fatalf("monitor retention %d %v", n, e)
		}
		if e = live.Close(); e != nil {
			t.Fatal(e)
		}
	}
	backup, e := openDB(filepath.Join(dir, "core.db.bak-19"))
	if e != nil {
		t.Fatal(e)
	}
	defer backup.Close()
	if e = backup.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 19 {
		t.Fatalf("backup %d %v", version, e)
	}
}
