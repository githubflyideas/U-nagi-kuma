package server

import (
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/license"
)

type auditItem struct {
	Actor, Action, Target, Area string
	Detail                      map[string]any
}

func TestAuditLog(t *testing.T) {
	admin := newEnv(t, usersBind, nil)
	admin.setup()
	var created struct{ ID int64 }
	if c := admin.do("POST", "/api/v1/users", map[string]any{"username": "bjops", "password": "secret123", "role": "viewer"}, &created); c != 200 {
		t.Fatalf("create: %d", c)
	}
	if c := admin.do("PUT", "/api/v1/users/"+itoa(created.ID), map[string]any{"role": "operator", "areas": []string{"region:north"}}, nil); c != 200 {
		t.Fatalf("update: %d", c)
	}
	ops := admin.loginAs("bjops", "secret123")
	if c := ops.do("POST", "/api/v1/monitors", map[string]any{"record": map[string]any{"name": "bj-sw", "type": "PING", "target": "10.10.0.2"}}, nil); c != 200 {
		t.Fatalf("operator create: %d", c)
	}
	admin.do("POST", "/api/v1/session", map[string]string{"username": "bjops", "password": "nope-nope"}, nil)
	if c := ops.do("GET", "/api/v1/audit", nil, nil); c != 403 {
		t.Fatalf("operators may not read the audit log: %d", c)
	}
	// Recorded all along, readable once registered.
	if c := admin.do("GET", "/api/v1/audit", nil, nil); c != 402 {
		t.Fatalf("unregistered: %d", c)
	}
	admin.register(license.FeatAudit)

	var log struct{ Items []auditItem }
	admin.do("GET", "/api/v1/audit", nil, &log)
	find := func(action string) *auditItem {
		for i := range log.Items {
			if log.Items[i].Action == action {
				return &log.Items[i]
			}
		}
		t.Fatalf("no %s in %+v", action, log.Items)
		return nil
	}
	if e := find("user.create"); e.Target != "bjops" || e.Actor != "admin" || e.Detail["role"] != "viewer" {
		t.Fatalf("user.create: %+v", e)
	}
	e := find("user.update")
	before, _ := e.Detail["before"].(map[string]any)
	after, _ := e.Detail["after"].(map[string]any)
	if e.Target != "bjops" || before["role"] != "viewer" || after["role"] != "operator" {
		t.Fatalf("user.update: %+v", e)
	}
	if e := find("monitor.create"); e.Actor != "bjops" || e.Target != "bj-sw" || e.Area != "region:north" {
		t.Fatalf("monitor.create: %+v", e)
	}
	if e := find("config.apply"); e.Actor != "bjops" || e.Area != "region:north" {
		t.Fatalf("config.apply: %+v", e)
	}
	if e := find("session.login_failed"); e.Actor != "bjops" {
		t.Fatalf("login_failed: %+v", e)
	}
	// Filters.
	admin.do("GET", "/api/v1/audit?action=user", nil, &log)
	for _, e := range log.Items {
		if e.Action != "user.create" && e.Action != "user.update" {
			t.Fatalf("filter user: %+v", e)
		}
	}
	admin.do("GET", "/api/v1/audit?area=region:north&actor=bjops", nil, &log)
	if len(log.Items) != 2 {
		t.Fatalf("filter area+actor: %+v", log.Items)
	}
	// The configuration change carries the record before and after.
	admin.do("GET", "/api/v1/audit?action=config.apply&actor=bjops", nil, &log)
	diff, _ := log.Items[0].Detail["diff"].([]any)
	if len(diff) != 1 || diff[0].(map[string]any)["after"] == nil || !strings.Contains(diff[0].(map[string]any)["after"].(string), "10.10.0.2") {
		t.Fatalf("diff: %+v", log.Items[0].Detail)
	}
	// High-risk filter, verify, export.
	admin.do("GET", "/api/v1/audit?risk=high", nil, &log)
	for _, e := range log.Items {
		if e.Action == "monitor.create" {
			t.Fatalf("monitor.create is not high-risk: %+v", log.Items)
		}
	}
	var v struct {
		OK      bool
		Entries int
		Head    string
	}
	if c := admin.do("GET", "/api/v1/audit/verify", nil, &v); c != 200 || !v.OK || v.Entries < 5 || v.Head == "" {
		t.Fatalf("verify: %d %+v", c, v)
	}
	var rows []map[string]any
	if c := admin.do("GET", "/api/v1/audit/export?format=json&action=user", nil, &rows); c != 200 || len(rows) != 2 {
		t.Fatalf("export: %d %+v", c, rows)
	}
	ops.do("DELETE", "/api/v1/session", nil, nil)
	admin.do("GET", "/api/v1/audit?action=session.logout", nil, &log)
	if len(log.Items) != 1 || log.Items[0].Actor != "bjops" {
		t.Fatalf("logout: %+v", log.Items)
	}
	admin.do("GET", "/api/v1/audit?action=audit", nil, &log)
	if len(log.Items) != 1 || log.Items[0].Action != "audit.export" {
		t.Fatalf("the export itself is recorded: %+v", log.Items)
	}
}

func TestAuditActionNames(t *testing.T) {
	for pattern, want := range map[string]string{
		"POST /api/v1/users":                       "user.create",
		"PUT /api/v1/users/{id}":                   "user.update",
		"POST /api/v1/monitors/{name}/pause":       "monitor.pause",
		"DELETE /api/v1/nodes/{name}/token":        "node.token.delete",
		"POST /api/v1/incidents/{id}/ack":          "incident.ack",
		"PUT /api/v1/config":                       "config.update",
		"POST /api/v1/maintenance":                 "maintenance.create",
		"PUT /api/v1/secrets/{name}":               "secret.update",
		"POST /api/v1/cluster/join-codes":          "cluster.join-codes",
		"POST /api/v1/reports/daily/send":          "reports.daily.send",
		"DELETE /api/v1/incidents/{id}/ack":        "incident.ack.delete",
		"POST /api/v1/monitors/{name}/push/rotate": "monitor.push.rotate",
	} {
		if got := auditAction(pattern); got != want {
			t.Errorf("%s: %s, want %s", pattern, got, want)
		}
	}
}
