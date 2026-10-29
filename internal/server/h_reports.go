package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
)

// tactical summarises hosts and services by state, split into unhandled
// and handled (acknowledged, silenced, in maintenance) like an operations
// console needs it.
func (s *Server) tactical() map[string]any {
	cfg := s.app.Config().Config
	incs := s.incidentIndex()
	type split struct {
		Total     int `json:"total"`
		Unhandled int `json:"unhandled"`
		Handled   int `json:"handled"`
	}
	services := map[string]*split{}
	for _, r := range s.app.Engine().Rows() {
		row := s.rowBase(r, cfg, incs)
		sp := services[row.Status]
		if sp == nil {
			sp = &split{}
			services[row.Status] = sp
		}
		sp.Total++
		handled := row.Incident != nil && (row.Incident.Acked || row.Incident.SilencedUntil != nil)
		if handled {
			sp.Handled++
		} else {
			sp.Unhandled++
		}
	}
	hosts := map[string]int{}
	for _, h := range s.hostList() {
		hosts[h.Status]++
	}
	return map[string]any{"services": services, "hosts": hosts}
}

// availability reports uptime and downtime per monitor over a range.
func (s *Server) availability(w http.ResponseWriter, r *http.Request) (any, error) {
	key, d := rangeOf(r)
	if key == "24h" && r.URL.Query().Get("range") == "" {
		key, d = "30d", ranges["30d"]
	}
	now := s.now()
	from := now.Add(-d)
	type row struct {
		Name      string   `json:"name"`
		Title     string   `json:"title"`
		Host      string   `json:"host"`
		Group     string   `json:"group,omitempty"`
		Area      string   `json:"area,omitempty"`
		Status    string   `json:"status"`
		Ratio     *float64 `json:"ratio"`
		Coverage  float64  `json:"coverage"`
		DegradedS float64  `json:"degraded_s"`
		DownS     float64  `json:"down_s"`
		ExcludedS float64  `json:"excluded_s"`
		Incidents int      `json:"incidents"`
	}
	cfg := s.app.Config().Config
	counts, _ := s.app.Store().IncidentCounts(from, now)
	status := map[string]string{}
	for _, r := range s.app.Engine().Rows() {
		status[r.Name] = r.Status
	}
	out := []row{}
	for _, m := range cfg.Monitors {
		u, err := s.app.Store().UptimeOf(m.Name, from, now)
		if err != nil {
			return nil, err
		}
		out = append(out, row{Name: m.Name, Title: m.Title, Host: m.HostKey(), Group: m.Group, Area: m.Area, Status: status[m.Name],
			Ratio: s.shownRatio(u), Coverage: u.Coverage, DegradedS: u.DegradedS, DownS: u.DownS, ExcludedS: u.ExcludedS, Incidents: counts[m.Name]})
	}
	// Worst first: that is what a report is read for.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := pf(out[i].Ratio), pf(out[j].Ratio)
		if a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	return map[string]any{"range": key, "from": from, "to": now, "items": out, "generated": now.Format(time.RFC3339)}, nil
}

type dailyJSON struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Policy  string `json:"policy"`
	OK      bool   `json:"ok"`
	Enabled bool   `json:"enabled"`
	At      string `json:"at"`
}

func (s *Server) daily(r engine.Report) dailyJSON {
	ts := s.app.Config().Config.Tunables
	return dailyJSON{Title: r.Title, Body: r.Body, Policy: r.Policy, OK: r.OK, Enabled: ts.Bool("notify.daily-report"), At: ts.Raw("notify.daily-report-at")}
}

// dailyReport previews the daily status report as it would be sent now.
func (s *Server) dailyReport(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.daily(s.app.Engine().DailyReport()), nil
}

// sendDailyReport sends the daily status report now.
func (s *Server) sendDailyReport(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.daily(s.app.Engine().SendDailyReport()), nil
}

// expiry lists every certificate and domain registration the checks
// watch, soonest first.
func (s *Server) expiry(w http.ResponseWriter, r *http.Request) (any, error) {
	type row struct {
		Monitor string  `json:"monitor"`
		Title   string  `json:"title"`
		What    string  `json:"what"` // cert | domain
		Target  string  `json:"target"`
		Days    float64 `json:"days"`
		Status  string  `json:"status"`
	}
	out := []row{}
	for _, m := range s.app.Engine().Rows() {
		for what, key := range map[string]string{"cert": "cert.days", "domain": "domain.days"} {
			if d, ok := m.Metrics[key]; ok {
				out = append(out, row{m.Name, m.Title, what, m.Target, d, m.Status})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Days < out[j].Days })
	return map[string]any{"items": out}, nil
}
