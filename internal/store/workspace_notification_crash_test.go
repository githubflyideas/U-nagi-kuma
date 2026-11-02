package store

import (
	"context"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/notify"
)

func TestWorkspaceNotificationAcknowledgementCrashWindow(t *testing.T) {
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
	ctx, finish, e := w.BeginExecution(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
	if _, e = s.core.Exec(`CREATE TRIGGER reject_delivery_ack BEFORE INSERT ON workspace_events WHEN NEW.kind='delivered_PROBLEM' BEGIN SELECT RAISE(ABORT,'injected ack crash'); END`); e != nil {
		t.Fatal(e)
	}
	deliveryIDs := []string{}
	send := func(_ context.Context, _ string, m *notify.Message) bool {
		deliveryIDs = append(deliveryIDs, m.DeliveryID)
		return true
	}
	if _, e = w.DispatchWorkspaceNotifications(ctx, send); e == nil {
		t.Fatal("ack failure hidden")
	}
	incidents, e := s.WorkspaceIncidents(scope, 100)
	if e != nil || len(incidents) != 1 || incidents[0].State.Told != 0 {
		t.Fatalf("partial ack %v %v", incidents, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER reject_delivery_ack`); e != nil {
		t.Fatal(e)
	}
	if pending, e := w.DispatchWorkspaceNotifications(ctx, send); e != nil || !pending || len(deliveryIDs) != 1 {
		t.Fatalf("lease replay %v %v %v", pending, deliveryIDs, e)
	}
	now := s.now()
	s.now = func() time.Time { return now.Add(36 * time.Second) }
	if pending, e := w.DispatchWorkspaceNotifications(ctx, send); e != nil || pending {
		t.Fatalf("lease recovery %v %v", pending, e)
	}
	if len(deliveryIDs) != 2 || deliveryIDs[0] != deliveryIDs[1] {
		t.Fatalf("stable receiver dedup ID %v", deliveryIDs)
	}
	if _, e = w.DispatchWorkspaceNotifications(ctx, send); e != nil || len(deliveryIDs) != 2 {
		t.Fatalf("acknowledged replay %v %v", deliveryIDs, e)
	}
	incidents, e = s.WorkspaceIncidents(scope, 100)
	if e != nil || incidents[0].State.Told != 1 {
		t.Fatalf("durable told %v %v", incidents, e)
	}
}
