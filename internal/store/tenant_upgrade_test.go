package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTenantMigrationRollbackAndRetry(t *testing.T) {
	dir := legacyTenantStore(t)
	db, err := openDB(filepath.Join(dir, "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Store{dir: dir, core: db}
	// A failure AFTER the entire new schema and backfill must roll everything back.
	broken := append([]string{}, coreMigrations[:14]...)
	broken = append(broken, tenantFoundationMigration+`; SELECT * FROM forced_missing_table;`)
	if err = migrateDB(s, "core", broken); err == nil {
		t.Fatal("injected failure succeeded")
	}
	var version, n int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 14 {
		t.Fatalf("schema advanced: %d %v", version, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('tenants','tenant_members','network_scopes','tenant_user_insert')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("partial migration: %d %v", n, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "core.db.bak-14")); err != nil {
		t.Fatal(err)
	}
	backup, err := openDB(filepath.Join(dir, "core.db.bak-14"))
	if err != nil {
		t.Fatal(err)
	}
	if err = backup.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 14 {
		t.Fatalf("backup version: %d %v", version, err)
	}
	if err = backup.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("backup users: %d %v", n, err)
	}
	backup.Close()
	if err = migrateDB(s, "core", coreMigrations); err != nil {
		t.Fatal(err)
	}
	if err = migrateDB(s, "core", coreMigrations); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM tenant_members`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("retry memberships: %d %v", n, err)
	}
}

func TestTenantMembershipImportDemotionAndReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{Tool: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if _, err = s.CreateUser("root", "secret123"); err != nil {
		t.Fatal(err)
	}
	uid, err := s.CreateUserHash("imported", HashPassword("secret123"), RoleOperator, []string{"restricted"})
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := s.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	ctx := TenantContext{TenantID: tenant.ID, UserID: uid}
	m, err := s.TenantMember(ctx)
	if err != nil || m.Role != RoleOperator || len(m.Areas) != 1 || m.Areas[0] != "restricted" {
		t.Fatalf("import escalated: %+v %v", m, err)
	}
	if err = s.SetRole(uid, RoleAdmin, nil); err != nil {
		t.Fatal(err)
	}
	scope, err := s.CreateNetworkScope(ctx, "private")
	if err != nil {
		t.Fatal(err)
	}
	const other TenantID = "dddddddddddddddddddddddddddddddd"
	if _, err = s.core.Exec(`INSERT INTO tenants(id,name) VALUES(?,'other')`, other); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`INSERT INTO tenant_members(tenant_id,user_id,role,disabled) VALUES(?,?,'admin',0)`, other, uid); err != nil {
		t.Fatal(err)
	}
	otherCtx := TenantContext{TenantID: other, UserID: uid}
	if err = s.SetRole(uid, RoleViewer, []string{"restricted"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []TenantContext{ctx, otherCtx} {
		m, err = s.TenantMember(c)
		if err != nil || m.Role != RoleViewer {
			t.Fatalf("stale elevated role: %+v %v", m, err)
		}
		if _, err = s.CreateNetworkScope(c, "denied"); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("stale write privilege: %v", err)
		}
	}
	if err = s.SetRole(uid, RoleAdmin, nil); err != nil {
		t.Fatal(err)
	}
	m, err = s.TenantMember(otherCtx)
	if err != nil || m.Role != RoleViewer {
		t.Fatalf("promotion leaked to other tenant: %+v %v", m, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, Options{Tool: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.NetworkScope(ctx, scope.ID)
	if err != nil || got != scope {
		t.Fatalf("scope not persisted: %+v %v", got, err)
	}
	m, err = s.TenantMember(otherCtx)
	if err != nil || m.Role != RoleViewer {
		t.Fatalf("reopened role: %+v %v", m, err)
	}
	// Account disabling is rechecked even if a membership row is stale.
	if _, err = s.core.Exec(`UPDATE users SET disabled=1 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`UPDATE tenant_members SET disabled=0 WHERE user_id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = s.TenantMember(ctx); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("stale enabled membership: %v", err)
	}
	if _, err = s.core.Exec(`UPDATE users SET disabled=0 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteUser(uid); err != nil {
		t.Fatal(err)
	}
	if _, err = s.TenantMember(ctx); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("deleted user: %v", err)
	}
	var n int
	if err = s.core.QueryRow(`SELECT COUNT(*) FROM tenant_members WHERE user_id=?`, uid).Scan(&n); err != nil || n != 0 {
		t.Fatalf("delete left memberships: %d %v", n, err)
	}
}

func TestTenantUserCreationAtomic(t *testing.T) {
	s, _, _ := open(t)
	if _, err := s.core.Exec(`CREATE TRIGGER reject_member BEFORE INSERT ON tenant_members BEGIN SELECT RAISE(ABORT,'injected membership failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUserRole("failed", "secret123", RoleViewer, nil); err == nil {
		t.Fatal("partial user creation succeeded")
	}
	if _, err := s.UserID("failed"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("orphan user: %v", err)
	}
	var role string
	if err := s.core.QueryRow(`SELECT role FROM tenant_members`).Scan(&role); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("orphan membership: %v", err)
	}
}
