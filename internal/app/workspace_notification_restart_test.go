package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestWorkspaceNotificationRestartPendingAndAcknowledged(t *testing.T) {
	a, scope, _ := workspaceConfigFixture(t)
	var mu sync.Mutex
	accept, healthy := false, false
	accepted := []string{}
	ids := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/check" {
			if !healthy {
				w.WriteHeader(503)
			}
			return
		}
		var m struct {
			Kind       string `json:"kind"`
			DeliveryID string `json:"delivery_id"`
		}
		if e := json.NewDecoder(r.Body).Decode(&m); e != nil {
			t.Error(e)
		}
		if !accept {
			w.WriteHeader(503)
			return
		}
		accepted = append(accepted, m.Kind)
		ids = append(ids, m.DeliveryID)
	}))
	defer server.Close()
	cfg, e := a.ApplyWorkspaceConfig(scope, []byte(runtimeSOA+fmt.Sprintf("check IN HTTP %s/check\nroute IN CHANNEL webhook url=%s/notify\ndefault IN POLICY chain=route\n", server.URL, server.URL)), 0)
	if e != nil {
		t.Fatal(e)
	}
	batch, e := a.RunWorkspaceChecks(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation)
	if e != nil || batch.NotificationStatus != "pending_delivery" {
		t.Fatalf("pending %v %v", batch, e)
	}
	if e = a.store.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := New(a.opts)
	if e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	accept = true
	mu.Unlock()
	for i := 0; i < 2; i++ {
		if status, e := b.DispatchWorkspaceNotifications(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation); e != nil || status != "ready" {
			t.Fatalf("dispatch %s %v", status, e)
		}
	}
	if e = b.store.Close(); e != nil {
		t.Fatal(e)
	}
	c, e := New(a.opts)
	if e != nil {
		t.Fatal(e)
	}
	defer c.store.Close()
	before := c.store.Stats()
	if _, e = c.RunWorkspaceChecks(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	healthy = true
	mu.Unlock()
	for i := 0; i < 2; i++ {
		if _, e = c.RunWorkspaceChecks(context.Background(), scope.TenantID, scope.WorkspaceID, cfg.Generation); e != nil {
			t.Fatal(e)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(accepted) != 2 || accepted[0] != "PROBLEM" || accepted[1] != "RECOVERY" || ids[0] == "" || ids[0] == ids[1] {
		t.Fatalf("restart delivery %v %v", accepted, ids)
	}
	incidents, e := c.WorkspaceIncidents(scope, 100)
	if e != nil || len(incidents) != 1 || incidents[0].State.Told != 1 || incidents[0].State.Open() {
		t.Fatalf("incidents %v %v", incidents, e)
	}
	events, e := c.WorkspaceEvents(scope, incidents[0].MonitorID, 100)
	if e != nil || len(events) != 4 {
		t.Fatalf("events %v %v", events, e)
	}
	for _, table := range []string{"incidents", "intents", "events", "deliveries"} {
		if c.store.Stats().Rows[table] != before.Rows[table] {
			t.Fatalf("global %s contaminated", table)
		}
	}
}
