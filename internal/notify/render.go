package notify

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/githubflyideas/unagikuma/internal/i18n"
)

// detailCap bounds remote-controlled text (HTTP bodies, DNS answers, SNMP
// strings) carried into messages.
const detailCap = 512

// sanitize strips control characters (except newlines) and truncates.
func sanitize(s string, n int) string {
	var b strings.Builder
	count := 0
	for _, r := range s {
		if count >= n {
			b.WriteString("…")
			break
		}
		if unicode.IsControl(r) && r != '\n' {
			r = ' '
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

var kindIcons = map[string]string{
	"PROBLEM": "🔴", "REPEAT": "🔴", "REOPEN": "🔴", "RECOVERY": "🟢", "ESCALATE": "🚨",
	"DRILL": "🧪", "TEST": "🧪", "SYSTEM": "⚠️", "DIGEST": "📋", "REPORT": "📊", "CHANGE": "🛠",
}

// Render produces the default title and body of a message in m.Lang.
func Render(m *Message) (title, body string) {
	lang := m.Lang
	kind := i18n.T(lang, "msg.kind."+m.Kind)
	icon := kindIcons[m.Kind]
	if m.Priority == "low" && m.Kind != "RECOVERY" {
		icon = "🟡"
	}
	if m.Kind == "SYSTEM" && m.Status == "ok" {
		icon = kindIcons["RECOVERY"] // a system problem that is over
	}
	var prefix []string
	if m.Delayed > 0 {
		prefix = append(prefix, i18n.T(lang, "msg.delayed", "minutes", int(m.Delayed.Minutes())))
	}
	if m.DryRun {
		prefix = append(prefix, i18n.T(lang, "msg.drill"))
	}
	if m.Kind == "STATUS" {
		// A status page's message to its subscribers: their words only.
		return sanitize(m.Title, 200), sanitize(m.Body, 8000)
	}
	head := strings.Join(prefix, " ")
	if head != "" {
		head += " "
	}
	switch m.Kind {
	case "SYSTEM", "DIGEST", "REPORT", "TEST", "DRILL", "CHANGE":
		title = fmt.Sprintf("%s %s[Unagikuma] %s", icon, head, sanitize(m.Title, 200))
		body = sanitize(m.Body, 4000)
		if m.Kind == "TEST" && body == "" {
			body = i18n.T(lang, "msg.test_body", "instance", m.Instance)
		}
		if m.Instance != "" && m.Kind != "TEST" {
			body = strings.TrimSpace(body + "\n" + i18n.T(lang, "msg.instance", "instance", m.Instance))
		}
		return title, body
	}
	status := ""
	if m.Status != "" {
		status = i18n.T(lang, "status."+m.Status)
	}
	tkey := "msg.title"
	if m.Monitor == "" || strings.EqualFold(strings.TrimSpace(m.Title), m.Monitor) || strings.TrimSpace(m.Title) == "" {
		// A monitor without its own title would read "Problem: web (web)".
		tkey = "msg.title_plain"
	}
	t := sanitize(m.Title, 200)
	if strings.TrimSpace(t) == "" {
		t = m.Monitor
	}
	title = strings.TrimSpace(icon + " " + head + i18n.T(lang, tkey, "kind", kind, "title", t, "monitor", m.Monitor, "status", status))
	if m.Kind != "RECOVERY" && m.Count > 0 {
		if m.Max > 0 {
			title += " (" + i18n.T(lang, "msg.count", "count", m.Count, "max", m.Max) + ")"
		} else {
			title += " (" + i18n.T(lang, "msg.count_unlimited", "count", m.Count) + ")"
		}
	}
	var lines []string
	if m.Type != "" {
		lines = append(lines, i18n.T(lang, "msg.check", "type", m.Type, "target", m.Target, "depth", m.Depth))
	}
	if m.Kind == "RECOVERY" {
		if m.Body != "" {
			lines = append(lines, sanitize(m.Body, detailCap))
		}
		lines = append(lines, i18n.N(lang, "msg.down_for", m.Count, "duration", i18n.Duration(m.Duration, lang), "count", m.Count))
	} else {
		reason := i18n.T(lang, "reason."+m.Reason)
		if d := sanitize(i18n.Detail(lang, m.Detail), detailCap); d != "" {
			reason += " — " + d
		}
		lines = append(lines, i18n.T(lang, "msg.reason", "reason", reason))
		if m.Quorum != "" {
			lines = append(lines, i18n.T(lang, "msg.quorum", "quorum", m.Quorum))
		}
		start := m.Time.Add(-m.Duration)
		lines = append(lines, i18n.T(lang, "msg.started", "time", start.Format("2006-01-02 15:04:05"), "duration", i18n.Duration(m.Duration, lang)))
		if m.Body != "" {
			lines = append(lines, sanitize(m.Body, detailCap))
		}
	}
	if u := m.DetailURL; u != "" {
		lines = append(lines, i18n.T(lang, "msg.details", "url", u))
	}
	if m.Instance != "" {
		lines = append(lines, i18n.T(lang, "msg.instance", "instance", m.Instance))
	}
	return title, strings.Join(lines, "\n")
}
