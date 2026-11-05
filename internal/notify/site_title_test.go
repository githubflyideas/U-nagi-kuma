package notify

import (
	"strings"
	"testing"
)

// A cluster's master is named after the product by default; its messages
// must not carry the product name twice in the title.
func TestDefaultSiteNameNotRepeated(t *testing.T) {
	base := Message{Kind: "SYSTEM", Title: "Probe node Shanghai (sh) is offline", Lang: "en"}

	m := base
	m.Site = "unagikuma"
	title, _ := Render(&m)
	if strings.Count(title, "nagikuma") != 1 {
		t.Fatalf("default site repeats the product name: %q", title)
	}

	m = base
	m.Site = "bj"
	title, _ = Render(&m)
	if !strings.Contains(title, "[bj]") {
		t.Fatalf("a named site must still show: %q", title)
	}
}
