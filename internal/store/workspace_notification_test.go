package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/probe"
)

func notificationRecord(t *testing.T, w *WorkspaceRuntimeWriter, id int64) error {
	t.Helper()
	_, e := w.RecordNotification(id, probe.Result{Vote: probe.VoteFail, Latency: time.Millisecond}, time.Second, engine.StepParams{Attempts: 1}, WorkspaceNotificationPlan{Channel: "route", Name: "check", Policy: engine.NotifyPolicy{Max: 3, Recovery: true, ReopenWindow: time.Hour}})
	return e
}
func TestWorkspaceNotificationRequiresExecutionLease(t *testing.T) {
	s, _, _ := open(t)
	scope, _, key := secretWorkspaceFixture(t, s)
	w := executionWriter(t, s, scope, key)
	ids, e := w.Register([]MonitorDef{{"check", "hash"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = notificationRecord(t, w, ids["check"]); e != nil {
		t.Fatal(e)
	}
	called := false
	_, e = w.DispatchWorkspaceNotifications(context.Background(), func(context.Context, string, *notify.Message) bool { called = true; return true })
	if !errors.Is(e, ErrTenantAccess) || called {
		t.Fatalf("dispatch without cancellation lease: %v called=%v", e, called)
	}
}

func TestWorkspaceNotificationAtomicRollback(t *testing.T) {
	s, _, _ := open(t)
	scope, _, key := secretWorkspaceFixture(t, s)
	w := executionWriter(t, s, scope, key)
	ids, e := w.Register([]MonitorDef{{"check", "hash"}})
	if e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"workspace_incidents", "workspace_events", "workspace_notification_intents"} {
		t.Run(table, func(t *testing.T) {
			if _, e = s.core.Exec(`CREATE TRIGGER reject_notification BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
				t.Fatal(e)
			}
			if e = notificationRecord(t, w, ids["check"]); e == nil {
				t.Fatal("late insert accepted")
			}
			if _, e = s.core.Exec(`DROP TRIGGER reject_notification`); e != nil {
				t.Fatal(e)
			}
			for _, tbl := range []string{"workspace_incidents", "workspace_events", "workspace_notification_intents", "monitor_state"} {
				var n int
				if e = s.core.QueryRow(`SELECT count(*) FROM ` + tbl).Scan(&n); e != nil || n != 0 {
					t.Fatalf("partial %s: %d %v", tbl, n, e)
				}
			}
		})
	}
	if e = notificationRecord(t, w, ids["check"]); e != nil {
		t.Fatal(e)
	}
	if e = notificationRecord(t, w, ids["check"]); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.core.QueryRow(`SELECT count(*) FROM workspace_notification_intents`).Scan(&n); e != nil || n != 1 {
		t.Fatalf("dedup %d %v", n, e)
	}
}
func TestWorkspaceNotificationScopeAuthority(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	wa, wb := executionWriter(t, s, a, key), executionWriter(t, s, b, key)
	ia, e := wa.Register([]MonitorDef{{"check", "hash"}})
	if e != nil {
		t.Fatal(e)
	}
	ib, e := wb.Register([]MonitorDef{{"check", "hash"}})
	if e != nil {
		t.Fatal(e)
	}
	if e = notificationRecord(t, wa, ib["check"]); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("cross enqueue %v", e)
	}
	if e = notificationRecord(t, wa, ia["check"]); e != nil {
		t.Fatal(e)
	}
	foreignCompany := a
	foreignCompany.TenantID = TenantID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, e = s.WorkspaceEvents(foreignCompany, 0, 100); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("global admin foreign company %v", e)
	}
	if _, e = s.WorkspaceEvents(b, ia["check"], 100); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("cross ID read %v", e)
	}
	if got, e := s.WorkspaceIncidents(b, 100); e != nil || len(got) != 0 {
		t.Fatalf("cross list %v %v", got, e)
	}
	uid, e := s.CreateUser("member", "password")
	if e != nil {
		t.Fatal(e)
	}
	// Scope the user as a company viewer; installation admin is not an implicit
	// nondefault company grant (all reads still use authorizeWorkspace).
	if _, e = s.core.Exec(`UPDATE tenant_members SET role='viewer' WHERE tenant_id=? AND user_id=?`, a.TenantID, uid); e != nil {
		t.Fatal(e)
	}
	member := a
	member.UserID = uid
	if _, e = s.WorkspaceEvents(member, 0, 100); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("ungranted member %v", e)
	}
	if e = s.SetWorkspaceMember(a, uid, RoleViewer, false); e != nil {
		t.Fatal(e)
	}
	if got, e := s.WorkspaceEvents(member, 0, 100); e != nil || len(got) != 1 {
		t.Fatalf("member read %v %v", got, e)
	}
	if e = s.RemoveWorkspaceMember(a, uid); e != nil {
		t.Fatal(e)
	}
	if _, e = s.WorkspaceIncidents(member, 100); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("revoked member %v", e)
	}
	if _, e = s.core.Exec(`UPDATE workspace_incidents SET workspace_id=?`, b.WorkspaceID); e == nil {
		t.Fatal("incident owner mutable")
	}
	if _, e = s.core.Exec(`UPDATE workspace_notification_intents SET monitor_id=?`, ib["check"]); e == nil {
		t.Fatal("intent owner mutable")
	}
	ctx, finish, e := wa.BeginExecution(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
	if e = s.RevokeWorkspaceSecretReader(a); e != nil {
		t.Fatal(e)
	}
	sent := false
	if _, e = wa.DispatchWorkspaceNotifications(ctx, func(context.Context, string, *notify.Message) bool { sent = true; return true }); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("revoked dispatch %v", e)
	}
	if sent {
		t.Fatal("revoked network call")
	}
	if e = notificationRecord(t, wa, ia["check"]); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("revoked enqueue %v", e)
	}
}
