package store

import "testing"

func TestDefaultRuntimeSnapshotAtomic(t *testing.T) {
	s, _, _ := open(t)
	if err := s.SyncMonitors([]MonitorDef{{Name: "old", DefHash: "old"}}); err != nil {
		t.Fatal(err)
	}
	old := s.MonitorID("old")
	if _, err := s.core.Exec(`CREATE TRIGGER reject_runtime BEFORE INSERT ON settings WHEN NEW.key='runtime-test' BEGIN SELECT RAISE(ABORT,'injected snapshot failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncDefaultRuntimeSnapshot([]MonitorDef{{Name: "new", DefHash: "new"}}, "runtime-test", "next"); err == nil {
		t.Fatal("failed snapshot reported success")
	}
	if s.MonitorID("old") != old || s.MonitorID("new") != 0 {
		t.Fatal("failed publication changed cache")
	}
	var retired int
	if err := s.core.QueryRow(`SELECT count(*) FROM monitors WHERE id=? AND retired_at IS NOT NULL`, old).Scan(&retired); err != nil || retired != 0 {
		t.Fatal("failed publication retired old identity", err)
	}
	if _, err := s.core.Exec(`DROP TRIGGER reject_runtime`); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncDefaultRuntimeSnapshot([]MonitorDef{{Name: "new", DefHash: "new"}}, "runtime-test", "next"); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Setting("runtime-test")
	if err != nil || raw != "next" || s.MonitorID("new") == 0 || s.MonitorID("old") != 0 {
		t.Fatal("committed publication missing", raw, err)
	}
}
