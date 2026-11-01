package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type storedWorkspaceConfig struct {
	revision, generation  int64
	kid, dek, value, hash string
}

func storedConfig(t *testing.T, s *Store, ctx WorkspaceContext) storedWorkspaceConfig {
	t.Helper()
	var v storedWorkspaceConfig
	e := s.core.QueryRow(`SELECT c.revision,c.generation,c.key_id,c.dek,c.value,r.token_hash FROM workspace_applied_configs c JOIN workspace_secret_readers r ON r.tenant_id=c.tenant_id AND r.workspace_id=c.workspace_id WHERE c.tenant_id=? AND c.workspace_id=?`, ctx.TenantID, ctx.WorkspaceID).Scan(&v.revision, &v.generation, &v.kid, &v.dek, &v.value, &v.hash)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestWorkspaceConfigAtomicPublication(t *testing.T) {
	s, _, _ := open(t)
	a, _, key := secretWorkspaceFixture(t, s)
	x := s.Secrets(key)
	repo, e := s.WorkspaceSecrets(a, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = repo.Put("same", "PLAINTEXT-SECRET", nil); e != nil {
		t.Fatal(e)
	}
	body := []byte("literal=PRIVATE-CONFIG ${secret:same}")
	state, e := x.ApplyWorkspaceConfig(a, body, 0, configValidator)
	if e != nil {
		t.Fatal(e)
	}
	before := storedConfig(t, s, a)
	dek, e := unseal(key, before.dek)
	if e != nil {
		t.Fatal(e)
	}
	plain, e := unseal(dek, before.value)
	if e != nil {
		t.Fatal(e)
	}
	var payload workspaceConfigPayload
	if e = json.Unmarshal(plain, &payload); e != nil {
		t.Fatal(e)
	}
	if payload.State != state || !bytes.Equal(payload.Body, body) || payload.Credential == "" || before.hash != readerHash(payload.Credential) {
		t.Fatal("incorrect sealed payload")
	}
	for _, bad := range []string{payload.Credential, "PRIVATE-CONFIG", "PLAINTEXT-SECRET"} {
		if strings.Contains(before.value, bad) || strings.Contains(before.dek, bad) || before.hash == bad {
			t.Fatal("plaintext stored")
		}
	}
	old, e := s.RuntimeSecrets(payload.Credential, key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`CREATE TRIGGER workspace_config_fail BEFORE UPDATE OF value ON workspace_applied_configs BEGIN SELECT RAISE(ABORT,'injected snapshot failure'); END;`); e != nil {
		t.Fatal(e)
	}
	if _, e = x.ApplyWorkspaceConfig(a, []byte("next ${secret:same}"), state.Revision, configValidator); e == nil {
		t.Fatal("late failure published")
	}
	if got := storedConfig(t, s, a); got != before {
		t.Fatal("late failure changed snapshot or authority")
	}
	if got, e := old.Get("same"); e != nil || got != "PLAINTEXT-SECRET" {
		t.Fatalf("late failure revoked old reader: %v", e)
	}
	if got, _, e := x.RestoreWorkspaceConfig(a.TenantID, a.WorkspaceID, state.Generation, configValidator); e != nil || !bytes.Equal(got, body) {
		t.Fatalf("old snapshot lost: %v", e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER workspace_config_fail`); e != nil {
		t.Fatal(e)
	}
	// One CAS winner even with concurrent app-equivalent callers.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := x.ApplyWorkspaceConfig(a, body, state.Revision, configValidator)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	won, conflict := 0, 0
	for e := range errs {
		if e == nil {
			won++
		} else if errors.Is(e, ErrWorkspaceConfigConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if won != 1 || conflict != 1 {
		t.Fatalf("CAS winners=%d conflicts=%d", won, conflict)
	}
	if _, e = old.Get("same"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("replacement old capability: %v", e)
	}
}

func TestWorkspaceConfigBindingIntegrityAndRotationRollback(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x := s.Secrets(key)
	sa, e := x.ApplyWorkspaceConfig(a, []byte("A"), 0, configValidator)
	if e != nil {
		t.Fatal(e)
	}
	_, e = x.ApplyWorkspaceConfig(b, []byte("B"), 0, configValidator)
	if e != nil {
		t.Fatal(e)
	}
	va, vb := storedConfig(t, s, a), storedConfig(t, s, b)
	if _, e = s.core.Exec(`UPDATE workspace_applied_configs SET dek=?,value=? WHERE workspace_id=?`, vb.dek, vb.value, a.WorkspaceID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = x.RestoreWorkspaceConfig(a.TenantID, a.WorkspaceID, sa.Generation, configValidator); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("cross-scope ciphertext accepted: %v", e)
	}
	if _, e = s.core.Exec(`UPDATE workspace_applied_configs SET dek=?,value=? WHERE workspace_id=?`, va.dek, va.value, a.WorkspaceID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = x.RestoreWorkspaceConfig("other-company", a.WorkspaceID, sa.Generation, configValidator); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("cross-company recovery: %v", e)
	}
	if _, _, e = s.Secrets(bytes.Repeat([]byte{3}, 32)).RestoreWorkspaceConfig(a.TenantID, a.WorkspaceID, sa.Generation, configValidator); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("wrong KEK: %v", e)
	}
	if _, e = s.core.Exec(`CREATE TABLE config_rotation_count(n INTEGER); INSERT INTO config_rotation_count VALUES(0); CREATE TRIGGER config_rotation_fail AFTER UPDATE OF dek ON workspace_applied_configs BEGIN UPDATE config_rotation_count SET n=n+1; SELECT CASE WHEN (SELECT n FROM config_rotation_count)=2 THEN RAISE(ABORT,'late rotation failure') END; END;`); e != nil {
		t.Fatal(e)
	}
	if e = x.Rotate(bytes.Repeat([]byte{8}, 32)); e == nil {
		t.Fatal("rotation failure ignored")
	}
	if storedConfig(t, s, a) != va || storedConfig(t, s, b) != vb {
		t.Fatal("rotation partially committed")
	}
	if _, _, e = x.RestoreWorkspaceConfig(a.TenantID, a.WorkspaceID, sa.Generation, configValidator); e != nil {
		t.Fatalf("old KEK unusable after rollback: %v", e)
	}
}

func TestWorkspaceConfigMigrationRecovery(t *testing.T) {
	dir := legacyTenantStore(t)
	db, e := openDB(filepath.Join(dir, "core.db"))
	if e != nil {
		t.Fatal(e)
	}
	s := &Store{dir: dir, core: db}
	if e = migrateDB(s, "core", coreMigrations[:18]); e != nil {
		t.Fatal(e)
	}
	var previousMonitors int
	if e = db.QueryRow(`SELECT count(*) FROM monitors`).Scan(&previousMonitors); e != nil {
		t.Fatal(e)
	}
	broken := append([]string{}, coreMigrations[:18]...)
	broken = append(broken, workspaceConfigMigration+`; SELECT * FROM injected_config_migration_failure;`)
	if e = migrateDB(s, "core", broken); e == nil {
		t.Fatal("late migration succeeded")
	}
	var version, n int
	if e = db.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 18 {
		t.Fatalf("rollback version %d %v", version, e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='workspace_applied_configs'`).Scan(&n); e != nil || n != 0 {
		t.Fatal("partial migration")
	}
	backup, e := openDB(filepath.Join(dir, "core.db.bak-18"))
	if e != nil {
		t.Fatal(e)
	}
	if e = backup.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 18 {
		t.Fatal("wrong backup")
	}
	if e = backup.QueryRow(`SELECT count(*) FROM monitors`).Scan(&n); e != nil || n != previousMonitors {
		t.Fatal("backup monitor change")
	}
	backup.Close()
	db.Close()
	key := bytes.Repeat([]byte{7}, 32)
	var state WorkspaceConfigState
	for i := 0; i < 2; i++ {
		live, e := Open(dir, Options{Tool: true})
		if e != nil {
			t.Fatal(e)
		}
		if e = live.core.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 19 {
			t.Fatalf("migration version %d %v", version, e)
		}
		if e = live.core.QueryRow(`SELECT count(*) FROM monitors`).Scan(&n); e != nil || n != previousMonitors {
			t.Fatal("migration monitor change")
		}
		if i == 0 {
			a, _, _ := secretWorkspaceFixture(t, live)
			state, e = live.Secrets(key).ApplyWorkspaceConfig(a, []byte("post-upgrade"), 0, configValidator)
			if e != nil {
				t.Fatal(e)
			}
		} else {
			raw, _, e := live.Secrets(key).RestoreWorkspaceConfig(state.TenantID, state.WorkspaceID, state.Generation, configValidator)
			if e != nil || string(raw) != "post-upgrade" {
				t.Fatalf("upgrade/publish/reopen: %v", e)
			}
		}
		if e = live.Close(); e != nil {
			t.Fatal(e)
		}
	}
}
