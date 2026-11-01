package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func TestWorkspaceConfigRejectedApplyPreservesAuthority(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	uid, e := a.store.CreateUserRole("department-admin", "password", store.RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.store.SetWorkspaceMember(ca, uid, store.RoleAdmin, false); e != nil {
		t.Fatal(e)
	}
	if e = a.store.SetWorkspaceMember(cb, uid, store.RoleViewer, false); e != nil {
		t.Fatal(e)
	}
	admin := ca
	admin.UserID = uid
	before, e := a.ApplyWorkspaceConfig(admin, []byte(workspaceBody), 0)
	if e != nil {
		t.Fatal(e)
	}
	_, old, e := a.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, before.Generation)
	if e != nil {
		t.Fatal(e)
	}
	key, e := store.LoadOrCreateKey(a.opts.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	other, e := a.store.WorkspaceSecrets(cb, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = other.Put("foreign-only", "FOREIGN-SECRET", nil); e != nil {
		t.Fatal(e)
	}
	t.Setenv("UNAGIKUMA_SCOPED_TEST", "AMBIENT-SECRET")
	foreign := cb
	foreign.UserID = uid
	cases := []struct {
		name     string
		ctx      store.WorkspaceContext
		raw      string
		revision int64
		want     error
	}{
		{"stale", admin, workspaceBody, 0, store.ErrWorkspaceConfigConflict},
		{"missing", admin, strings.ReplaceAll(workspaceBody, "secret:same", "secret:absent"), 1, store.ErrNotFound},
		{"other-department-secret", admin, strings.ReplaceAll(workspaceBody, "secret:same", "secret:foreign-only"), 1, store.ErrNotFound},
		{"environment", admin, strings.ReplaceAll(workspaceBody, "secret:same", "env:UNAGIKUMA_SCOPED_TEST"), 1, store.ErrSecretReference},
		{"malformed-ref", admin, workspaceBody + "; ${secret:unfinished\n", 1, store.ErrSecretReference},
		{"wrong-workspace", foreign, workspaceBody, 0, store.ErrTenantAccess},
		{"zero-principal", store.WorkspaceContext{}, workspaceBody, 0, store.ErrTenantAccess},
		{"invalid-config", admin, "INVALID CONFIG", 1, store.ErrWorkspaceConfigInvalid},
		{"missing-parent", admin, workspaceBody + "child IN HTTP http://127.0.0.1/ parent=foreign\n", 1, store.ErrWorkspaceConfigInvalid},
		{"missing-policy", admin, workspaceBody + "child IN HTTP http://127.0.0.1/ policy=foreign\n", 1, store.ErrWorkspaceConfigInvalid},
		{"missing-channel", admin, workspaceBody + "p IN POLICY chain=foreign\n", 1, store.ErrWorkspaceConfigInvalid},
		{"missing-period", admin, workspaceBody + "out IN CHANNEL log period=foreign\n", 1, store.ErrWorkspaceConfigInvalid},
		{"missing-group-member", admin, workspaceBody + "out IN CHANNEL group members=foreign\n", 1, store.ErrWorkspaceConfigInvalid},
		{"missing-escalation", admin, workspaceBody + "out IN CHANNEL log\np IN POLICY chain=out escalate=1m:foreign\n", 1, store.ErrWorkspaceConfigInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, publish := range []bool{false, true} {
				var err error
				if publish {
					_, err = a.ApplyWorkspaceConfig(tc.ctx, []byte(tc.raw), tc.revision)
				} else {
					_, err = a.PreviewWorkspaceConfig(tc.ctx, []byte(tc.raw), tc.revision)
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("publish=%v: want %v got %v", publish, tc.want, err)
				}
				after, e := a.WorkspaceConfigState(ca)
				if e != nil || after != before {
					t.Fatalf("rejected config changed state: %+v %v", after, e)
				}
				if got, e := old.Get("same"); e != nil || got != "department-A" {
					t.Fatalf("rejected config revoked prior authority %q %v", got, e)
				}
				c, _, e := a.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, before.Generation)
				if e != nil || len(c.Monitors) != 1 || c.Monitors[0].Name != "check" {
					t.Fatalf("previous snapshot lost: %v", e)
				}
			}
		})
	}
	// Authorization is checked again, not cached from preview. Durable authority is
	// independent of the interactive administrator after a successful publication.
	if e = a.store.RemoveWorkspaceMember(ca, uid); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ApplyWorkspaceConfig(admin, []byte(workspaceBody), 1); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("revoked admin %v", e)
	}
	if _, _, e = a.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, before.Generation); e != nil {
		t.Fatalf("captured admin identity: %v", e)
	}
	next, e := a.ApplyWorkspaceConfig(ca, []byte(workspaceBody), 1)
	if e != nil || next.Revision != 2 || next.Generation <= before.Generation {
		t.Fatalf("next generation %+v %v", next, e)
	}
	if _, e = old.Get("same"); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("old reader %v", e)
	}
	if _, _, e = a.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, before.Generation); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("old recovery %v", e)
	}
}

func TestWorkspaceConfigRevocationSurvivesRestart(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	sa, e := a.ApplyWorkspaceConfig(ca, []byte(workspaceBody), 0)
	if e != nil {
		t.Fatal(e)
	}
	sb, e := a.ApplyWorkspaceConfig(cb, []byte(workspaceBody), 0)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.store.RevokeWorkspaceSecretReader(ca); e != nil {
		t.Fatal(e)
	}
	if e = a.store.SetWorkspaceRuntimeActive(cb, false); e != nil {
		t.Fatal(e)
	}
	if e = a.store.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := New(a.opts)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.store.Close()
	for _, s := range []store.WorkspaceConfigState{sa, sb} {
		if _, _, e = reopened.RestoreWorkspaceConfig(s.TenantID, s.WorkspaceID, s.Generation); !errors.Is(e, store.ErrTenantAccess) {
			t.Fatalf("restart resurrected: %v", e)
		}
	}
	if _, e = reopened.ApplyWorkspaceConfig(cb, []byte(workspaceBody), sb.Revision); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("inactive apply %v", e)
	}
	if e = reopened.store.SetWorkspaceRuntimeActive(cb, true); e != nil {
		t.Fatal(e)
	}
	if _, _, e = reopened.RestoreWorkspaceConfig(cb.TenantID, cb.WorkspaceID, sb.Generation); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("enable resurrected %v", e)
	}
	if _, e = reopened.ApplyWorkspaceConfig(cb, []byte(workspaceBody), sb.Revision); e != nil {
		t.Fatal(e)
	}
}
