package app

import (
	"github.com/githubflyideas/unagikuma/internal/config"
	"strings"
	"testing"
)

func TestZoneExportSecretsAndDependencies(t *testing.T) {
	s, err := OpenZoneStore(t.TempDir(), "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	body := zoneTestBody + "; COMMENT_PRIVATE\nchat IN CHANNEL telegram token=INLINE_PRIVATE chat-id=123\n"
	z, err := s.Import([]byte(body), ZoneImportOptions{Name: "office"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Export(z.Header.ZoneID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Text, "INLINE_PRIVATE") || strings.Contains(out.Text, "COMMENT_PRIVATE") || strings.Contains(out.Text, "fingerprint") {
		t.Fatal("export leaked")
	}
	if len(out.Missing) == 0 || !strings.Contains(out.Text, "${secret:") {
		t.Fatalf("redaction dependencies not reported: %+v", out)
	}
	copy, err := s.Import([]byte(out.Text), ZoneImportOptions{Name: "copy"})
	if err != nil {
		t.Fatal(err)
	}
	if copy.Header.ZoneID == z.Header.ZoneID {
		t.Fatal("copy reused identity")
	}
	body = zoneTestBody + "chat IN CHANNEL telegram token=${secret:telegram} chat-id=123\n"
	z, err = s.Import([]byte(body), ZoneImportOptions{Name: "references"})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.Export(z.Header.ZoneID, func(ref string) bool { return ref == "secret:telegram" })
	if err != nil || !strings.Contains(out.Text, "${secret:telegram}") {
		t.Fatalf("reference lost %v", err)
	}
	for _, missing := range out.Missing {
		if missing == "secret:telegram" {
			t.Fatal("known ref missing")
		}
	}
}
func TestZoneExportReferenceSafety(t *testing.T) {
	s, err := OpenZoneStore(t.TempDir(), "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"${secret:telegram}", "${env:UNAGIKUMA_BOT}"} {
		z, err := s.Import([]byte(zoneTestBody+"chat IN CHANNEL telegram token="+value+" chat-id=123\n"), ZoneImportOptions{Name: "ref" + string(rune('a'+len(value)))})
		if err != nil {
			t.Fatal(err)
		}
		before := z.Revision()
		out, err := s.Export(z.Header.ZoneID, func(string) bool { return true })
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Text, value) {
			t.Fatal("reference lost")
		}
		for _, m := range out.Missing {
			if m == strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}") {
				t.Fatal("known reference marked missing")
			}
		}
		got, err := s.Get(z.Header.ZoneID)
		if err != nil || got.Revision() != before {
			t.Fatal("export mutated source")
		}
	}
	z, err := s.Import([]byte(zoneTestBody+"chat IN CHANNEL telegram token=PRIVATE${secret:telegram} chat-id=123\n"), ZoneImportOptions{Name: "mixed"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Export(z.Header.ZoneID, nil)
	if err == nil || out.Text != "" || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("mixed credential was not refused safely")
	}
}

func TestZoneExportRefusesOpaqueTargetCredentials(t *testing.T) {
	s, err := OpenZoneStore(t.TempDir(), "node", config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for i, target := range []string{"https://user:PRIVATE@example.com", "https://example.com/?token=PRIVATE", "https://example.com/PRIVATE"} {
		z, err := s.Import([]byte(strings.Replace(zoneTestBody, "https://example.com", target, 1)), ZoneImportOptions{Name: string(rune('a' + i))})
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.Export(z.Header.ZoneID, nil)
		if err == nil || out.Text != "" || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatalf("unsafe target exported: %+v %v", out, err)
		}
	}
}
