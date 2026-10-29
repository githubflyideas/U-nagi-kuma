package importer

import (
	"os"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

func TestKumaImport(t *testing.T) {
	data, err := os.ReadFile("testdata/kuma.json")
	if err != nil {
		t.Fatal(err)
	}
	base := "@ IN SOA m. a.b. ( 2026102801 1m 15s 5s 3 )\nweb IN PING 10.0.0.1\n"
	_, existing, _ := config.Load("t", []byte(base), config.Options{})
	res, err := Kuma(data, existing, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + res.Text)
	for _, n := range res.Notes {
		t.Log("note: ", n)
	}
	if res.Monitors != 11 || res.Channels != 2 { // Mail and Pager are used by no monitor
		t.Fatalf("monitors %d channels %d", res.Monitors, res.Channels)
	}
	// Credentials are referenced, never written.
	if strings.Contains(res.Text, "123:ABC") || strings.Contains(res.Text, "hook/abc") || strings.Contains(res.Text, "pw") {
		t.Fatal("a credential is in the records")
	}
	if len(res.Secrets) != 2 {
		t.Fatalf("secrets: %v", res.Secrets)
	}
	// The result validates once the secrets exist.
	known := func(name string) bool { _, ok := res.Secrets[name]; return ok }
	_, cfg, diags := config.Load("t", []byte(base+res.Text), config.Options{SecretKnown: known})
	if config.HasErrors(diags) {
		t.Fatalf("%v\n%s", diags, res.Text)
	}
	site := cfg.FindMonitor("keyword") // 官网 has no ASCII name: named after its type
	if site == nil || site.Title != "官网" || site.Group != "Core" || site.Opts["keyword"] != "Welcome" || site.Attempts != 3 || site.Retry.Seconds() != 30 || site.Opts["cert-days"] != "14" {
		t.Fatalf("keyword monitor: %+v", site)
	}
	if p := cfg.FindPolicy(site.Policy); p == nil || strings.Join(p.Chain, ",") != "channel,tg-ops" || p.Mode != "all" {
		t.Fatalf("policy: %+v", p)
	}
	if cfg.FindMonitor("postgres").Policy != site.Policy {
		t.Fatal("monitors with the same channels share a policy")
	}
	api := cfg.FindMonitor("api-health")
	if api.Opts["json-path"] != "$.status" || api.Opts["header.x-env"] != "prod" || api.Opts["header.authorization"] != "" {
		t.Fatalf("json monitor: %+v", api.Opts)
	}
	if gw := cfg.FindMonitor("gateway"); !gw.Disabled || gw.Attempts != 4 {
		t.Fatalf("gateway: %+v", gw)
	}
	if dns := cfg.FindMonitor("resolver"); dns.Type != "UDP" || dns.Opts["query"] != "corp.example.com" || dns.Opts["qtype"] != "MX" {
		t.Fatalf("dns: %+v", dns)
	}
	if w := cfg.FindMonitor("web-2"); w == nil || w.Title != "web" || w.Opts["method"] != "POST" || w.Opts["insecure"] != "yes" || w.Opts["no-follow"] != "yes" {
		t.Fatalf("renamed http monitor: %+v", w)
	}
	notes := strings.Join(res.Notes, "\n")
	// Service checks keep the address, user and database, not the password.
	if r := cfg.FindMonitor("redis"); r == nil || r.Written != "REDIS" || r.Target != "10.0.0.7:6379" || r.Opts["password"] != "" {
		t.Fatalf("redis: %+v", r)
	}
	if m := cfg.FindMonitor("orders-db"); m == nil || m.Written != "MYSQL" || m.Opts["user"] != "mon" || m.Opts["database"] != "orders" {
		t.Fatalf("mysql: %+v", m)
	}
	if d := cfg.FindMonitor("nginx"); d == nil || d.Type != "DOCKER" || d.Target != "nginx" {
		t.Fatalf("docker: %+v", d)
	}
	if g := cfg.FindMonitor("search"); g == nil || g.Written != "GRPC" || g.Opts["service"] != "search.v1" {
		t.Fatalf("grpc: %+v", g)
	}
	for _, want := range []string{"mongodb", "push URL", "upside-down", "Authorization", "password was not imported", "health check"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
	// Monitors that exist already are skipped; a backup with nothing new
	// is refused.
	_, again, _ := config.Load("t", []byte(base+res.Text), config.Options{})
	if _, err := Kuma(data, again, Options{}); err == nil || !strings.Contains(err.Error(), "nothing to import") {
		t.Fatalf("re-import: %v", err)
	}
	if _, err := Kuma([]byte(`{"foo":1}`), nil, Options{}); err == nil {
		t.Fatal("not a backup accepted")
	}
}
