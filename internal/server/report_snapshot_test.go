package server

import (
	"testing"
	"time"
)

func TestDailyReportAcceptedLogChannelAPI(t *testing.T) {
	e := newEnv(t, testBind+`report-log IN CHANNEL log
report-policy IN POLICY chain=report-log
$SET notify.daily-report-policy=report-policy
`, nil)
	e.setup()
	var got map[string]any
	if code := e.do("POST", "/api/v1/reports/daily/send", map[string]any{}, &got); code != 200 {
		t.Fatalf("send status: %d", code)
	}
	if got["delivery"] != "queued" || got["policy"] != "report-policy" {
		t.Fatalf("log policy must accept queueing without claiming delivery: %v", got)
	}
	if got["ok"] != true {
		t.Fatalf("legacy health OK changed: %v", got)
	}
	var preview map[string]any
	if code := e.do("GET", "/api/v1/reports/daily", nil, &preview); code != 200 {
		t.Fatalf("preview status: %d", code)
	}
	if preview["delivery"] != nil {
		t.Fatalf("preview inherited send outcome: %v", preview)
	}
}

func TestDailyReportSnapshotAPI(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	for _, method := range []string{"GET", "POST"} {
		path := "/api/v1/reports/daily"
		var body any
		if method == "POST" {
			path += "/send"
			body = map[string]any{}
		}
		var got map[string]any
		if code := e.do(method, path, body, &got); code != 200 {
			t.Fatalf("%s: %d", method, code)
		}
		for _, key := range []string{"title", "body", "policy", "ok", "enabled", "at", "snapshot", "generated_at", "timezone"} {
			if _, ok := got[key]; !ok {
				t.Fatalf("%s missing %s: %v", method, key, got)
			}
		}
		if _, err := time.Parse(time.RFC3339Nano, got["generated_at"].(string)); err != nil {
			t.Fatal(err)
		}
		s := got["snapshot"].(map[string]any)
		if s["semantics"] != "current_snapshot" || s["services"].(map[string]any)["total"] != float64(3) {
			t.Fatalf("snapshot: %v", s)
		}
		if method == "POST" && got["delivery"] != "rejected" {
			t.Fatalf("no-channel policy must reject queueing: %v", got)
		}
		if method == "GET" && got["delivery"] != nil {
			t.Fatalf("preview must not claim delivery: %v", got)
		}
	}
}
