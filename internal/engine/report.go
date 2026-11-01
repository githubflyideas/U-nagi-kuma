package engine

import (
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/i18n"
	"sort"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/hostview"
	"github.com/githubflyideas/unagikuma/internal/notify"
)

// Report is the daily status report: what is down right now, what is
// fine, rendered in the configured language.
type Report struct {
	Title    string
	Body     string
	Policy   string
	OK       bool           // Legacy: no currently failing hosts/services; not proof of coverage.
	Snapshot ReportSnapshot `json:"snapshot"`
	Delivery string         `json:"delivery,omitempty"` // Queue acceptance only; never confirmation of delivery.
}

// ReportSnapshot describes one atomic engine view, never a historical window.
type ReportSnapshot struct {
	GeneratedAt   time.Time     `json:"generated_at"`
	Timezone      string        `json:"timezone"`
	Semantics     string        `json:"semantics"`
	State         string        `json:"state"` // empty | attention | incomplete | healthy
	Hosts         ReportCounts  `json:"hosts"`
	Services      ReportCounts  `json:"services"`
	Issues        []ReportIssue `json:"issues"`
	IssuesTotal   int           `json:"issues_total"`
	IssuesOmitted int           `json:"issues_omitted"`
}
type ReportCounts struct {
	Total  int            `json:"total"`
	States map[string]int `json:"states"`
}
type ReportIssue struct {
	Name    string     `json:"name"`
	Title   string     `json:"title"`
	Status  string     `json:"status"`
	Since   *time.Time `json:"since,omitempty"`
	Acked   bool       `json:"acked"`
	AckedBy string     `json:"acked_by,omitempty"`
	Detail  string     `json:"detail,omitempty"`
}

// scheduleReport computes the next daily status report.
func (e *Engine) scheduleReport(now time.Time) {
	h, mi := e.cfg.Tunables.TimeOfDay("notify.daily-report-at")
	e.nextReport = NextDaily(now, h, mi, e.cfg.Location)
}

// dailyReport sends the status report once a day at the configured time,
// also when everything is fine: silence is not proof the system works.
func (e *Engine) dailyReport(now time.Time) {
	if e.nextReport.IsZero() || now.Before(e.nextReport) {
		return
	}
	e.scheduleReport(now)
	if !e.cfg.Tunables.Bool("notify.daily-report") {
		return
	}
	e.sendReport(e.report(now), now)
}

func (e *Engine) sendReport(r Report, now time.Time) bool {
	return e.enqueue(&Intent{Kind: "REPORT", Policy: r.Policy, CreatedAt: now, DueAt: now, System: true,
		Message: notify.Message{Kind: "REPORT", Priority: "high", Title: r.Title, Body: r.Body, Time: now, Instance: e.cfg.Instance, Lang: e.cfg.Lang}})
}

// DailyReport renders the report as it would be sent now.
func (e *Engine) DailyReport() Report {
	var r Report
	e.do(func() { r = e.report(e.now()) })
	return r
}

// SendDailyReport attempts to queue the report now, outside its schedule.
// Delivery reports acceptance, independently of the legacy health OK field.
func (e *Engine) SendDailyReport() Report {
	r := Report{Delivery: "rejected"}
	e.do(func() {
		now := e.now()
		r = e.report(now)
		r.Delivery = "rejected"
		if e.sendReport(r, now) {
			r.Delivery = "queued"
		}
	})
	return r
}

func (e *Engine) report(now time.Time) Report {
	max := e.cfg.Tunables.Int("notify.daily-report-max-items")
	if max < 1 {
		max = 1
	}
	loc := e.cfg.Location
	if loc == nil {
		loc = time.UTC
	}
	snap := ReportSnapshot{GeneratedAt: now, Timezone: loc.String(), Semantics: "current_snapshot", Issues: []ReportIssue{},
		Hosts:    ReportCounts{States: map[string]int{"up": 0, "down": 0, "unreachable": 0, "pending": 0, "paused": 0, "maintenance": 0}},
		Services: ReportCounts{States: map[string]int{"ok": 0, "critical": 0, "degraded": 0, "unknown": 0, "unreachable": 0, "pending": 0, "paused": 0, "maintenance": 0}}}
	rows := map[string]MonitorRow{}
	var svcs []hostview.Service
	for _, n := range e.sortedMonitorNames() {
		r, ok := e.row(n, now)
		if !ok {
			continue
		}
		rows[n] = r
		m := e.monitors[n]
		host := m.Host
		if host == "" {
			host = n
		}
		svcs = append(svcs, hostview.Service{Name: n, Host: host, Address: m.Address, Area: m.Area, Group: m.Group, Ping: m.Ping, Status: r.Status})
	}
	hosts := hostview.Build(svcs)
	byState := map[string][]string{}
	for _, h := range hosts {
		byState[h.Status] = append(byState[h.Status], h.Name)
	}
	counts := snap.Services.States
	var bad []MonitorRow
	for _, s := range svcs {
		counts[s.Status]++
		switch s.Status {
		case StatusCritical, StatusDegraded, StatusUnknown, StatusUnreachable:
			bad = append(bad, rows[s.Name])
		}
	}
	// Worst first, then the longest standing.
	sort.SliceStable(bad, func(i, j int) bool {
		a, b := hostview.Severity[bad[i].Status], hostview.Severity[bad[j].Status]
		if a != b {
			return a > b
		}
		return bad[i].LastChange.Before(bad[j].LastChange)
	})

	list := func(names []string) string {
		if len(names) > max {
			return strings.Join(names[:max], ", ") + " " + e.text("report_more", len(names)-max)
		}
		return strings.Join(names, ", ")
	}
	snap.Hosts.Total, snap.Services.Total = len(hosts), len(svcs)
	for state, names := range byState {
		snap.Hosts.States[state] = len(names)
	}
	snap.IssuesTotal = len(bad)
	snap.State = "healthy"
	switch {
	case len(svcs) == 0:
		snap.State = "empty"
	case len(bad) > 0:
		snap.State = "attention"
	case counts["ok"] != len(svcs):
		snap.State = "incomplete"
	}
	var b []string
	b = append(b, i18n.T(e.cfg.Lang, "ui.report.snapshot"), now.In(loc).Format("2006-01-02 15:04:05 -07:00")+" · "+loc.String(),
		i18n.T(e.cfg.Lang, "ui.report.state."+snap.State))
	if len(svcs) > 0 {
		b = append(b, e.text("report_summary", len(hosts), len(svcs), counts))
	}
	b = append(b, "")
	section := func(key string, names []string) {
		if len(names) > 0 {
			b = append(b, e.text(key, len(names), list(names)))
		}
	}
	section("report_hosts_down", byState["down"])
	section("report_hosts_unreachable", byState["unreachable"])
	if len(bad) > 0 {
		b = append(b, "", e.text("report_services", len(bad)))
		for i, r := range bad {
			if i == max {
				b = append(b, e.text("report_more", len(bad)-max))
				break
			}
			title := r.Title
			if title == "" {
				title = r.Name
			}
			who, acked := "", false
			inc := e.incidents[r.IncidentID]
			if inc == nil && r.Status == StatusDegraded {
				inc = e.lowByMon[r.Name]
				if inc == nil {
					inc = e.incidents[r.RegionalID]
				}
			}
			if inc != nil && inc.State.Open() && inc.State.Acked {
				who, acked = inc.State.AckedBy, true
			}
			issue := ReportIssue{Name: r.Name, Title: title, Status: r.Status, Acked: acked, AckedBy: who, Detail: reportSnippet(r.Detail, 400)}
			if !r.LastChange.IsZero() {
				since := r.LastChange
				issue.Since = &since
			}
			snap.Issues = append(snap.Issues, issue)
			line := "• " + reportSnippet(title, 160) + ": " + i18n.T(e.cfg.Lang, "status."+r.Status)
			if issue.Since != nil {
				line += " · " + issue.Since.In(loc).Format("2006-01-02 15:04:05 -07:00")
			}
			if issue.Acked {
				line += " · " + i18n.T(e.cfg.Lang, "ui.report.acked")
				if who != "" {
					line += ": " + reportSnippet(who, 80)
				}
			}
			if issue.Detail != "" {
				line += " — " + issue.Detail
			}
			b = append(b, line)
		}
		b = append(b, "")
	}
	section("report_hosts_maintenance", byState["maintenance"])
	section("report_hosts_paused", byState["paused"])
	section("report_hosts_pending", byState["pending"])
	section("report_hosts_up", byState["up"])
	snap.IssuesOmitted = snap.IssuesTotal - len(snap.Issues)

	r := Report{Policy: e.reportPolicy(), Snapshot: snap}
	down := len(byState["down"]) + len(byState["unreachable"])
	r.OK = down == 0 && len(bad) == 0
	if snap.State == "healthy" {
		r.Title = e.text("report_title_ok", len(hosts), len(svcs))
	} else if snap.State == "empty" || snap.State == "incomplete" {
		r.Title = i18n.T(e.cfg.Lang, "ui.report.tab") + ": " + i18n.T(e.cfg.Lang, "ui.report.state."+snap.State)
	} else {
		r.Title = e.text("report_title", down, len(hosts), len(bad), len(svcs))
	}
	// One blank line between blocks, however many sections were empty.
	var lines []string
	for _, l := range b {
		if l == "" && (len(lines) == 0 || lines[len(lines)-1] == "") {
			continue
		}
		lines = append(lines, l)
	}
	r.Body = strings.TrimSpace(strings.Join(lines, "\n"))
	return r
}

// Keep one check's verbose output from dominating a notification.
func reportSnippet(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > limit {
		return fmt.Sprintf("%s…", string(r[:limit]))
	}
	return s
}

func (e *Engine) reportPolicy() string {
	if p := strings.TrimSpace(e.cfg.Tunables.Raw("notify.daily-report-policy")); p != "" {
		if _, ok := e.cfg.Policies[p]; ok {
			return p
		}
	}
	return e.policy("system").Name
}

// Announce sends a free-form notice (a planned change, its reminders and
// its outcome) through a policy, or to the given channels at once. It is
// delivered and recorded like any other notification.
func (e *Engine) Announce(kind, policy string, channels []string, title, body string) {
	e.do(func() {
		now := e.now()
		pol := policy
		if pol == "" {
			pol = "system"
		}
		e.enqueue(&Intent{Kind: kind, Policy: pol, Chain: append([]string(nil), channels...), CreatedAt: now, DueAt: now, System: true,
			Message: notify.Message{Kind: kind, Priority: "high", Title: title, Body: body, Time: now, Instance: e.cfg.Instance, Lang: e.cfg.Lang}})
	})
}
