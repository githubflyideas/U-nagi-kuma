package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceSecretAccessAndDTO(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	uid, e := s.CreateUserRole("secret-dept-admin", "password", RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	owner := a
	if e = s.SetWorkspaceMember(a, uid, RoleAdmin, false); e != nil {
		t.Fatal(e)
	}
	a.UserID = uid
	x, e := s.WorkspaceSecrets(a, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.Put("ref", "NEVER-IN-DTO", []string{"probe"}); e != nil {
		t.Fatal(e)
	}
	for _, ctx := range []WorkspaceContext{{}, {TenantContext{TenantID: "wrong", UserID: uid}, a.WorkspaceID}, {a.TenantContext, b.WorkspaceID}} {
		if _, e = s.WorkspaceSecrets(ctx, key); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("guessed scope %v", e)
		}
		if _, e = s.IssueWorkspaceSecretReader(ctx); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("guessed service scope %v", e)
		}
	}
	list, e := x.List()
	if e != nil || len(list) != 1 || list[0].Name != "ref" {
		t.Fatalf("list %+v %v", list, e)
	}
	dto, e := json.Marshal(list)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"NEVER-IN-DTO", "key_id", "dek", "value"} {
		if strings.Contains(string(dto), bad) {
			t.Fatalf("unsafe DTO %s", dto)
		}
	}
	var cipher string
	if e = s.core.QueryRow(`SELECT value FROM secrets WHERE workspace_id=?`, a.WorkspaceID).Scan(&cipher); e != nil || strings.Contains(cipher, "NEVER-IN-DTO") {
		t.Fatalf("plaintext %v", e)
	}
	for _, role := range []string{RoleViewer, RoleOperator} {
		if e = s.SetWorkspaceMember(owner, uid, role, false); e != nil {
			t.Fatal(e)
		}
		if _, e = x.Get("ref"); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("plaintext for %s %v", role, e)
		}
		if e = x.Put("ref", "changed", nil); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("write for %s %v", role, e)
		}
		if e = x.Delete("ref"); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("delete for %s %v", role, e)
		}
		if _, e = x.List(); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("list for %s %v", role, e)
		}
		if _, e = s.IssueWorkspaceSecretReader(a); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("issue for %s %v", role, e)
		}
		if e = s.RevokeWorkspaceSecretReader(a); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("revoke for %s %v", role, e)
		}
		if e = s.SetWorkspaceRuntimeActive(a, false); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("disable for %s %v", role, e)
		}
	}
	if e = s.SetWorkspaceMember(owner, uid, RoleAdmin, true); e != nil {
		t.Fatal(e)
	}
	if _, e = x.Get("ref"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("disabled grant %v", e)
	}
	if e = s.SetWorkspaceMember(owner, uid, RoleAdmin, false); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`UPDATE users SET disabled=1 WHERE id=?`, uid); e != nil {
		t.Fatal(e)
	}
	if _, e = x.Get("ref"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("disabled user %v", e)
	}
	if e = s.RemoveWorkspaceMember(owner, uid); e != nil {
		t.Fatal(e)
	}
	if _, e = x.Get("ref"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("revoked grant %v", e)
	}
	var nilRepo *WorkspaceSecretRepository
	if _, e = nilRepo.Get("ref"); !errors.Is(e, ErrTenantAccess) {
		t.Fatal(e)
	}
	var nilReader *RuntimeSecretReader
	if _, e = nilReader.Get("ref"); !errors.Is(e, ErrTenantAccess) {
		t.Fatal(e)
	}
	typ := reflect.TypeOf((*RuntimeSecretReader)(nil))
	if typ.NumMethod() != 2 || typ.Method(0).Name != "Get" || typ.Method(1).Name != "Resolve" {
		t.Fatalf("service gained methods: %v", typ)
	}
}

func TestWorkspaceSecretAtomicFailure(t *testing.T) {
	s, _, _ := open(t)
	a, _, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	if e := x.Put("ref", "before", nil); e != nil {
		t.Fatal(e)
	}
	credential, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.RuntimeSecrets(credential, key)
	if e != nil {
		t.Fatal(e)
	}
	for _, op := range []string{"UPDATE", "DELETE"} {
		if _, e = s.core.Exec(`CREATE TRIGGER fail_secret BEFORE ` + op + ` ON secrets BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
			t.Fatal(e)
		}
		if op == "UPDATE" {
			e = x.Put("ref", "after", nil)
		} else {
			e = x.Delete("ref")
		}
		if e == nil {
			t.Fatal("write accepted")
		}
		if got, e := x.Get("ref"); e != nil || got != "before" {
			t.Fatalf("failed write changed %q %v", got, e)
		}
		if _, e = s.core.Exec(`DROP TRIGGER fail_secret`); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.core.Exec(`CREATE TRIGGER fail_reader BEFORE UPDATE ON workspace_secret_readers BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if token, e := s.IssueWorkspaceSecretReader(a); e == nil || token != "" {
		t.Fatal("failed issuance leaked token")
	}
	if e = s.RevokeWorkspaceSecretReader(a); e == nil {
		t.Fatal("revoke accepted")
	}
	if e = s.SetWorkspaceRuntimeActive(a, false); e == nil {
		t.Fatal("disable accepted")
	}
	if got, e := r.Get("ref"); e != nil || got != "before" {
		t.Fatalf("failed lifecycle changed %q %v", got, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER fail_reader`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`UPDATE secrets SET workspace_id=? WHERE workspace_id=?`, "wrong", a.WorkspaceID); e == nil {
		t.Fatal("mutable ownership")
	}
	if _, e = s.core.Exec(`UPDATE workspace_secret_readers SET generation=generation WHERE workspace_id=?`, a.WorkspaceID); e == nil {
		t.Fatal("generation rollback")
	}
}

func TestWorkspaceSecretRestart(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, Options{Tool: true})
	if e != nil {
		t.Fatal(e)
	}
	a, b, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	y, _ := s.WorkspaceSecrets(b, key)
	for _, e = range []error{x.Put("same", "a", nil), y.Put("same", "b", nil)} {
		if e != nil {
			t.Fatal(e)
		}
	}
	old, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RevokeWorkspaceSecretReader(a); e != nil {
		t.Fatal(e)
	}
	live, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	inactive, e := s.IssueWorkspaceSecretReader(b)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetWorkspaceRuntimeActive(b, false); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(dir, Options{Tool: true})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, token := range []string{old, inactive} {
		if _, e = s.RuntimeSecrets(token, key); !errors.Is(e, ErrTenantAccess) {
			t.Fatalf("restart revived %v", e)
		}
	}
	r, e := s.RuntimeSecrets(live, key)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := r.Get("same"); e != nil || got != "a" {
		t.Fatalf("restart %q %v", got, e)
	}
	y, e = s.WorkspaceSecrets(b, key)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := y.Get("same"); e != nil || got != "b" {
		t.Fatalf("restart B %q %v", got, e)
	}
	if e = s.SetWorkspaceRuntimeActive(b, true); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RuntimeSecrets(inactive, key); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("reactivation revived %v", e)
	}
}

func TestWorkspaceSecretMigrationRollback(t *testing.T) {
	dir := legacyTenantStore(t)
	db, e := openDB(filepath.Join(dir, "core.db"))
	if e != nil {
		t.Fatal(e)
	}
	s := &Store{dir: dir, core: db}
	if e = migrateDB(s, "core", coreMigrations[:17]); e != nil {
		t.Fatal(e)
	}
	key := bytes.Repeat([]byte{4}, 32)
	dek := bytes.Repeat([]byte{5}, 32)
	wrapped, e := seal(key, dek)
	if e != nil {
		t.Fatal(e)
	}
	value, e := seal(dek, []byte("legacy"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO secrets(name,key_id,dek,value,scopes,updated_at) VALUES('old',?,?,?,'[]',123)`, keyID(key), wrapped, value); e != nil {
		t.Fatal(e)
	}
	broken := append([]string{}, coreMigrations[:17]...)
	broken = append(broken, workspaceSecretMigration+`; SELECT * FROM injected_failure;`)
	if e = migrateDB(s, "core", broken); e == nil {
		t.Fatal("migration accepted")
	}
	var version int
	if e = db.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil || version != 17 {
		t.Fatalf("partial version %d %v", version, e)
	}
	var got string
	if e = db.QueryRow(`SELECT value FROM secrets WHERE name='old'`).Scan(&got); e != nil || got != value {
		t.Fatalf("rollback ciphertext %v", e)
	}
	if e = db.QueryRow(`SELECT workspace_id FROM secrets`).Scan(&got); e == nil {
		t.Fatal("partial column")
	}
	backup, e := openDB(filepath.Join(dir, "core.db.bak-17"))
	if e != nil {
		t.Fatal(e)
	}
	if e = backup.QueryRow(`SELECT value FROM secrets WHERE name='old'`).Scan(&got); e != nil || got != value {
		t.Fatalf("backup %v", e)
	}
	backup.Close()
	db.Close()
	for i := 0; i < 2; i++ {
		st, e := Open(dir, Options{Tool: true})
		if e != nil {
			t.Fatal(e)
		}
		if got, e = st.Secrets(key).Get("old"); e != nil || got != "legacy" {
			t.Fatalf("upgrade default %q %v", got, e)
		}
		tenant, e := st.DefaultTenant()
		if e != nil {
			t.Fatal(e)
		}
		ws, e := st.Workspaces(TenantContext{tenant.ID, 11})
		if e != nil {
			t.Fatal(e)
		}
		r, e := st.WorkspaceSecrets(WorkspaceContext{TenantContext{tenant.ID, 11}, ws[0].ID}, key)
		if e != nil {
			t.Fatal(e)
		}
		if got, e = r.Get("old"); e != nil || got != "legacy" {
			t.Fatalf("backfill %q %v", got, e)
		}
		if e = st.core.QueryRow(`SELECT value FROM secrets WHERE name='old'`).Scan(&got); e != nil || got != value {
			t.Fatal("ciphertext changed")
		}
		st.Close()
	}
}
