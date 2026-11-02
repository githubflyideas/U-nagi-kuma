package notify

import (
	"strings"
	"testing"
)

// Every site sends its own messages; each one names the site.
func TestSiteInMessages(t *testing.T) {
	m := &Message{Kind: "PROBLEM", Monitor: "web", Status: "critical", Reason: "timeout", Lang: "en", Site: "Tokyo DC"}
	title, body := Render(m)
	if !strings.Contains(title, "[Tokyo DC]") || !strings.Contains(body, "Site: Tokyo DC") {
		t.Fatalf("site missing: %q / %q", title, body)
	}
	m = &Message{Kind: "SYSTEM", Title: "x", Body: "y", Lang: "en", Site: "Osaka"}
	if title, body = Render(m); !strings.Contains(title, "[Osaka]") || !strings.Contains(body, "Site: Osaka") {
		t.Fatalf("system site missing: %q / %q", title, body)
	}
	m = &Message{Kind: "PROBLEM", Monitor: "web", Lang: "en"}
	if title, body = Render(m); strings.Count(title, "[") > 1 || strings.Contains(body, "Site:") {
		t.Fatalf("single machine shows a site: %q / %q", title, body)
	}
}
