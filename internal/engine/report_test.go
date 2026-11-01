package engine

import (
	"encoding/json"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/tunables"
	"strings"
	"testing"
	"time"
)

func TestReportSnapshotContract(t *testing.T) {
	d := newDepEnv(t)
	d.wait("both ok", func() bool { return d.status("gw") == "ok" && d.status("srv") == "ok" })
	b, _ := json.Marshal(d.e.DailyReport())
	var r map[string]any
	_ = json.Unmarshal(b, &r)
	if r["snapshot"] == nil {
		t.Fatalf("missing structured snapshot: %s", b)
	}
}

func TestReportEmptyIsNotHealthy(t *testing.T) {
	ts, _ := tunables.New(nil)
	e := New(Deps{})
	e.Apply(Config{Tunables: ts, Location: time.UTC})
	r := e.report(time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC))
	if !r.OK {
		t.Fatal("legacy OK semantics changed")
	}
	if strings.Contains(r.Title, "all") || !strings.Contains(r.Body, "2026-10-30") || !strings.Contains(r.Body, "snapshot") {
		t.Fatalf("misleading or undated empty report: %+v", r)
	}
}

// Exercise the real renderer deterministically, without scheduling probes or notifications.
func TestReportStructuredCountsOrderingAndBounds(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	ts, err := tunables.New(map[string]string{"notify.daily-report-max-items": "5"})
	if err != nil {
		t.Fatal(err)
	}
	e := New(Deps{Sink: newMemSink(), Clock: func() time.Time { return now }})
	names := []string{"critical-new", "critical-old", "unknown", "unreachable", "degraded-a", "degraded-b", "ok", "pending", "paused", "maintenance"}
	specs := make([]MonitorSpec, 0, len(names))
	for _, name := range names {
		specs = append(specs, MonitorSpec{Name: name, Title: name, Host: name, Paused: name == "paused"})
	}
	e.Apply(Config{Tunables: ts, Monitors: specs, Location: time.FixedZone("Asia/Tokyo", 9*3600)})
	for _, name := range names {
		rt := e.states[name]
		rt.State.Avail = StatusOK
		rt.LastChange = now.Add(-time.Hour)
		switch name {
		case "critical-new", "critical-old":
			rt.State.Avail = StatusCritical
		case "unknown":
			rt.State.Avail = StatusUnknown
			rt.LastChange = time.Time{}
		case "unreachable":
			rt.State.Avail = StatusCritical
			rt.Unreachable = true
		case "degraded-a", "degraded-b":
			rt.State.Degraded = true
		case "pending":
			rt.State.Avail = StatusPending
		case "maintenance":
			e.maintOf[name] = "window"
		}
	}
	e.states["critical-old"].LastChange = now.Add(-2 * time.Hour)
	e.states["critical-old"].LastDetail = strings.Repeat("診断 ", 300)
	inc := &Incident{ID: 1, Monitor: "critical-old", State: IncidentState{OpenedAt: now.Add(-2 * time.Hour), Acked: true, AckedBy: "Operator"}}
	e.incidents[1] = inc
	e.byMonitor["critical-old"] = inc
	e.lowByMon["degraded-a"] = &Incident{ID: 2, Monitor: "degraded-a", State: IncidentState{OpenedAt: now.Add(-time.Hour), Acked: true, AckedBy: "Latency operator"}}
	r := e.report(now)
	s := r.Snapshot
	if r.OK || s.State != "attention" || !s.GeneratedAt.Equal(now) || s.Semantics != "current_snapshot" {
		t.Fatalf("summary: %+v", r)
	}
	if s.Services.Total != 10 || s.Hosts.Total != 10 || s.Services.States["critical"] != 2 || s.Services.States["degraded"] != 2 {
		t.Fatalf("counts: %+v", s)
	}
	for _, state := range []string{"unknown", "unreachable", "ok", "pending", "paused", "maintenance"} {
		if s.Services.States[state] != 1 {
			t.Errorf("%s: %v", state, s.Services.States)
		}
	}
	for _, state := range []string{"pending", "paused", "maintenance"} {
		if s.Hosts.States[state] != 1 {
			t.Errorf("host %s: %v", state, s.Hosts.States)
		}
	}
	if s.IssuesTotal != 6 || len(s.Issues) != 5 || s.IssuesOmitted != 1 {
		t.Fatalf("bounds: %+v", s)
	}
	want := []string{"critical-old", "critical-new", "unknown", "unreachable", "degraded-a"}
	for i, n := range want {
		if s.Issues[i].Name != n {
			t.Errorf("issue %d: %s != %s", i, s.Issues[i].Name, n)
		}
	}
	if !s.Issues[4].Acked || s.Issues[4].AckedBy != "Latency operator" {
		t.Fatalf("degraded acknowledgement missing: %+v", s.Issues[4])
	}
	issue := s.Issues[0]
	if !issue.Acked || issue.AckedBy != "Operator" || issue.Since == nil || len([]rune(issue.Detail)) > 401 {
		t.Fatalf("issue evidence: %+v", issue)
	}
	if s.Issues[2].Since != nil {
		t.Fatal("invented since for unknown timestamp")
	}
	for _, text := range []string{"2026-10-30 18:00:00 +09:00", "Asia/Tokyo", "Operator", "and 1 more"} {
		if !strings.Contains(r.Body, text) {
			t.Errorf("body missing %q: %s", text, r.Body)
		}
	}
	if strings.Contains(r.Body, "map[") || strings.Contains(r.Body, "0001-") || strings.Contains(r.Body, "• degraded-b") {
		t.Fatalf("bad delivery body: %s", r.Body)
	}
}

func TestReportNonReportingStatesPreserveLegacyOK(t *testing.T) {
	for _, state := range []string{"paused", "pending", "maintenance"} {
		t.Run(state, func(t *testing.T) {
			ts, _ := tunables.New(nil)
			e := New(Deps{Sink: newMemSink()})
			e.Apply(Config{Tunables: ts, Location: time.UTC, Monitors: []MonitorSpec{{Name: "one", Paused: state == "paused"}}})
			if state == "maintenance" {
				e.maintOf["one"] = "window"
			}
			r := e.report(time.Now())
			if !r.OK || r.Snapshot.State != "incomplete" || r.Snapshot.Services.States[state] != 1 || strings.Contains(r.Title, "all") {
				t.Fatalf("%s: %+v", state, r)
			}
		})
	}
}

func TestReportQueueAcceptance(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		name := "no-channels"
		var chain []string
		if accepted {
			name = "log-channel"
			chain = []string{"report-log"}
		}
		t.Run(name, func(t *testing.T) {
			ts, _ := tunables.New(nil)
			e := New(Deps{Sink: newMemSink()})
			e.Apply(Config{Tunables: ts, Location: time.UTC,
				Channels: map[string]ChannelSpec{"report-log": {Channel: notify.Channel{Name: "report-log", Kind: "log"}}},
				Policies: map[string]PolicySpec{"system": {Name: "system", Chain: chain}},
			})
			now := time.Now()
			if got := e.sendReport(e.report(now), now); got != accepted {
				t.Fatalf("acceptance = %v, want %v", got, accepted)
			}
			want := 0
			if accepted {
				want = 1
			}
			if len(e.intents) != want {
				t.Fatalf("queued intents = %d, want %d", len(e.intents), want)
			}
			for _, in := range e.intents {
				if in.State != IntentPending || in.Kind != "REPORT" {
					t.Fatalf("must only be pending: %+v", in)
				}
			}
		})
	}
}

func TestSendDailyReportRejectedWithoutChannels(t *testing.T) {
	d := newDepEnv(t)
	r := d.e.SendDailyReport()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["delivery"] != "rejected" {
		t.Fatalf("no-channel send must be rejected: %s", b)
	}
	if !r.OK {
		t.Fatal("legacy health OK must remain independent of queue acceptance")
	}
}

func TestDailyReport(t *testing.T) {
	d := newDepEnv(t)
	d.wait("both ok", func() bool { return d.status("gw") == "ok" && d.status("srv") == "ok" })
	if r := d.e.DailyReport(); !r.OK || !strings.Contains(r.Title, "2") || !strings.Contains(r.Body, "gw, srv") {
		t.Fatalf("all up: %+v", r)
	}
	d.set("gw", true)
	d.set("srv", true)
	d.wait("gw down", func() bool { return d.status("gw") == StatusCritical && d.status("srv") == StatusUnreachable })
	r := d.e.DailyReport()
	if r.OK || r.Policy != "system" {
		t.Fatalf("report: %+v", r)
	}
	for _, want := range []string{"DOWN (1): gw", "UNREACHABLE (1): srv", "CRITICAL", "Services not OK (2)"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, r.Body)
		}
	}
	// The critical service is listed before the unreachable one.
	if strings.Index(r.Body, "• gw") > strings.Index(r.Body, "• srv") {
		t.Errorf("order:\n%s", r.Body)
	}
}
