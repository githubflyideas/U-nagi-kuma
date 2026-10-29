package importer

import (
	"os"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

const soaLine = "@ IN SOA master. me.example.com. ( 2026102101 1m 15s 5s 3 )\n"

func TestNagiosImport(t *testing.T) {
	data, err := os.ReadFile("testdata/nagios.cfg")
	if err != nil {
		t.Fatal(err)
	}
	_, existing, _ := config.Load("monitors.bind", []byte(soaLine+"old-db1 IN TCP 10.0.0.20:22\n"), config.Options{})
	res, err := Nagios(data, existing, Options{})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Text
	want := []string{
		`gw 300s IN PING 10.0.0.1 retry=60s attempts=10 title="Core Gateway" device=gw policy=`,
		`web1 300s IN PING 10.0.0.10 retry=60s attempts=10 device=web1 group=web-servers parent=gw policy=`,
		`web2-http 60s IN HTTP http://10.0.0.11/health expect=200 attempts=3 title="web2 / HTTP" device=web2 group=web-servers handler="restart-httpd $SERVICESTATE$ $SERVICESTATETYPE$ $SERVICEATTEMPT$" parent=web2`,
		`web1-http 60s IN HTTP http://10.0.0.10/health`,
		`web1-site 60s IN HTTP https://www.example.com/login keyword="Sign in" cert-days=21 cert-critical-days=7`,
		`web1-ssh 60s IN TCP 10.0.0.10:22`,
		`gw-dns 60s IN DNS 10.0.0.1 query=example.com expect=93.184.216.34`,
		`db1-mysql 60s IN MYSQL 10.0.0.20:3306 user=monitor`,
		`db1-load 60s IN EXEC "check_nrpe -H 10.0.0.20 -c check_load" address=10.0.0.20`,
		`disabled=yes`,
		`db1-redis 60s IN TCP 10.0.0.20:6379`,
		`workhours IN PERIOD "mon 09:00-17:00; tue 09:00-17:00; wed 09:00-17:00; thu 09:00-17:00; fri 09:00-12:00; fri 13:00-17:00"`,
		`alice IN CHANNEL email host=localhost port=25 tls=none from=unagikuma@localhost to=alice@example.com period=workhours`,
		`nagiosadmin IN CHANNEL email`,
		`admins IN CHANNEL group members=nagiosadmin,alice`,
		`IN POLICY chain=admins mode=all repeat=7200s`,
		`IN POLICY chain=admins mode=all max=1`,
		`IN POLICY chain=alice mode=all max=1`,
	}
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("missing %q", w)
		}
	}
	for _, bad := range []string{"s3cret", "web2-ssh", "db1-ssh", "bob", "24x7", "$USER1$", "/usr/lib"} {
		if strings.Contains(text, bad) {
			t.Errorf("must not contain %q", bad)
		}
	}
	notes := strings.Join(res.Notes, "\n")
	for _, w := range []string{"already monitored as old-db1", "passwords were not imported", "copy check_nrpe", "copy restart-httpd",
		"bob has no e-mail", "pager number", `"december"`, "servicedependency definitions", "service groups", "contact group dbas is not defined"} {
		if !strings.Contains(notes, w) {
			t.Errorf("notes lack %q:\n%s", w, notes)
		}
	}
	// The result is a valid configuration.
	cfg := soaLine + "old-db1 IN TCP 10.0.0.20:22\n" + text
	if _, _, diags := config.Load("monitors.bind", []byte(cfg), config.Options{}); config.HasErrors(diags) {
		t.Fatalf("%v\n%s", diags, cfg)
	}
	if res.Monitors != 12 { // db1-ssh exists
		t.Errorf("monitors %d", res.Monitors)
	}
}

func TestNagiosParseErrors(t *testing.T) {
	for _, s := range []string{"define host {\n host_name x\n", "define {\n}\n", "nothing here\n"} {
		if _, err := Nagios([]byte(s), nil, Options{}); err == nil {
			t.Errorf("%q: no error", s)
		}
	}
}
