package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

// Build a real pre-foundation database by executing only the old migrations.
func legacyTenantStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range coreMigrations[:14] {
		if _, err = db.Exec(m); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO users(id,username,password,created_at,role,areas,disabled) VALUES
 (11,'admin','legacy',1,'admin','',0),(12,'ops','legacy',1,'operator','restricted',0),(13,'viewer','legacy',1,'viewer','',0),(14,'left','legacy',1,'admin','',1);
 INSERT INTO monitors(id,name,created_at) VALUES(71,'old monitor',1);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	hist, err := openDB(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range historyMigrations {
		if _, err = hist.Exec(m); err != nil {
			t.Fatal(err)
		}
		if _, err = hist.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = hist.Exec(`INSERT INTO rounds(mid,at,st,flags) VALUES(71,123456,1,0)`); err != nil {
		t.Fatal(err)
	}
	if err = hist.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestTenantLegacyUpgrade(t *testing.T) {
	dir := legacyTenantStore(t)
	var previous string
	for attempt := 0; attempt < 2; attempt++ {
		s, err := Open(dir, Options{Tool: true})
		if err != nil {
			t.Fatal(err)
		}
		var id string
		if err = s.core.QueryRow(`SELECT id FROM tenants WHERE is_default=1`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if previous != "" && previous != id {
			t.Fatal("tenant changed on reopen")
		}
		previous = id
		for _, x := range []struct {
			uid      int
			role     string
			disabled int
		}{{11, "admin", 0}, {12, "operator", 0}, {13, "viewer", 0}, {14, "admin", 1}} {
			var role string
			var disabled int
			if err = s.core.QueryRow(`SELECT role,disabled FROM tenant_members WHERE tenant_id=? AND user_id=?`, id, x.uid).Scan(&role, &disabled); err != nil {
				t.Fatal(err)
			}
			if role != x.role || disabled != x.disabled {
				t.Fatalf("membership changed: %d %s %d", x.uid, role, disabled)
			}
		}
		var mid, at int64
		if err = s.core.QueryRow(`SELECT id FROM monitors WHERE name='old monitor'`).Scan(&mid); err != nil || mid != 71 {
			t.Fatalf("monitor changed: %d %v", mid, err)
		}
		if err = s.hist.QueryRow(`SELECT mid,at FROM rounds`).Scan(&mid, &at); err != nil || mid != 71 || at != 123456 {
			t.Fatalf("history changed: %d %d %v", mid, at, err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTenantMembershipTracksUsers(t *testing.T) {
	s, _, _ := open(t)
	_, err := s.CreateUser("root", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := s.CreateUserRole("op", "secret123", RoleOperator, []string{"restricted"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(role string, disabled int) {
		t.Helper()
		var got string
		var d int
		if err := s.core.QueryRow(`SELECT role,disabled FROM tenant_members WHERE user_id=?`, uid).Scan(&got, &d); err != nil {
			t.Fatal(err)
		}
		if got != role || d != disabled {
			t.Fatalf("got %s/%d want %s/%d", got, d, role, disabled)
		}
	}
	check(RoleOperator, 0)
	if err = s.SetRole(uid, RoleViewer, nil); err != nil {
		t.Fatal(err)
	}
	check(RoleViewer, 0)
	if _, err = s.core.Exec(`UPDATE users SET disabled=1 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	check(RoleViewer, 1)
	if err = s.DeleteUser(uid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.core.QueryRow(`SELECT count(*) FROM tenant_members WHERE user_id=?`, uid).Scan(&n); err != nil || n != 0 {
		t.Fatalf("stale members: %d %v", n, err)
	}
}

func TestTenantContextAndNetworkRegistry(t *testing.T) {
	s, _, _ := open(t)
	uid, err := s.CreateUser("admin", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUserRole("viewer", "secret123", RoleViewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := s.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	ctx := TenantContext{TenantID: tenant.ID, UserID: uid}
	m, err := s.TenantMember(ctx)
	if err != nil || m.Role != RoleAdmin || m.UserID != uid || m.TenantID != tenant.ID {
		t.Fatalf("member: %+v %v", m, err)
	}
	for _, bad := range []TenantContext{{}, {TenantID: tenant.ID}, {TenantID: tenant.ID, UserID: 99999}, {TenantID: "unknown", UserID: uid}, {TenantID: TenantID(tenant.Name), UserID: uid}} {
		if _, err = s.TenantMember(bad); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("accepted invalid context %+v: %v", bad, err)
		}
		if _, err = s.CreateNetworkScope(bad, "private"); !errors.Is(err, ErrTenantAccess) {
			t.Fatalf("invalid create: %v", err)
		}
	}
	a, err := s.CreateNetworkScope(ctx, "private")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ID) != 32 || a.TenantID != tenant.ID {
		t.Fatalf("scope identity: %+v", a)
	}
	if _, err = s.CreateNetworkScope(ctx, "private"); err == nil {
		t.Fatal("duplicate tenant label accepted")
	}
	if _, err = s.CreateNetworkScope(ctx, "  "); err == nil {
		t.Fatal("empty label accepted")
	}
	if _, err = s.CreateNetworkScope(TenantContext{TenantID: tenant.ID, UserID: viewer}, "forbidden"); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("viewer write: %v", err)
	}
	// Additional tenants/grants are provisioned only by trusted SQL in this test.
	// There is deliberately no public tenant creation or membership mutation API.
	const other TenantID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err = s.core.Exec(`INSERT INTO tenants(id,name) VALUES(?,'Default')`, other); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`INSERT INTO tenant_members(tenant_id,user_id,role,disabled) VALUES(?,?,'admin',0)`, other, uid); err != nil {
		t.Fatal(err)
	}
	otherCtx := TenantContext{TenantID: other, UserID: uid}
	b, err := s.CreateNetworkScope(otherCtx, "private")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("same labels merged identity")
	}
	if _, err = s.NetworkScope(otherCtx, a.ID); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("cross tenant scope: %v", err)
	}
	if _, err = s.TenantMember(TenantContext{TenantID: other, UserID: viewer}); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("wrong user: %v", err)
	}
	if _, err = s.NetworkScope(TenantContext{TenantID: tenant.ID, UserID: 99999}, a.ID); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("wrong scope user: %v", err)
	}
	if _, err = s.core.Exec(`UPDATE tenants SET name='renamed' WHERE id=?`, tenant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`UPDATE network_scopes SET label='renamed' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.NetworkScope(ctx, a.ID)
	if err != nil || got.ID != a.ID || got.Label != "renamed" {
		t.Fatalf("rename lost identity: %+v %v", got, err)
	}
	if _, err = s.core.Exec(`UPDATE users SET disabled=1 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = s.TenantMember(ctx); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("disabled member: %v", err)
	}
	if _, err = s.NetworkScope(ctx, a.ID); !errors.Is(err, ErrTenantAccess) {
		t.Fatalf("stale context scope: %v", err)
	}
}

func TestTenantIdentityImmutable(t *testing.T) {
	s, _, _ := open(t)
	uid, err := s.CreateUser("root", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := s.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	scope, err := s.CreateNetworkScope(TenantContext{TenantID: tenant.ID, UserID: uid}, "private")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE tenants SET id='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`,
		`UPDATE tenants SET is_default=0`,
		`DELETE FROM tenants WHERE is_default=1`,
		`UPDATE network_scopes SET id='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`,
	} {
		if _, err = s.core.Exec(q); err == nil {
			t.Fatalf("identity mutation accepted: %s", q)
		}
	}
	if _, err = s.core.Exec(`INSERT INTO tenants(id,name) VALUES('cccccccccccccccccccccccccccccccc','other')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.core.Exec(`UPDATE network_scopes SET tenant_id='cccccccccccccccccccccccccccccccc' WHERE id=?`, scope.ID); err == nil {
		t.Fatal("scope moved between tenants")
	}
}

func TestTenantFreshSchema(t *testing.T) {
	s, _, _ := open(t)
	var id, name string
	if err := s.core.QueryRow(`SELECT id,name FROM tenants WHERE is_default=1`).Scan(&id, &name); err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 || name != "Default" {
		t.Fatalf("default identity: %q %q", id, name)
	}
	var n int
	if err := s.core.QueryRow(`SELECT COUNT(*) FROM tenant_members`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unexpected members: %d %v", n, err)
	}
}
