package importer

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/config"
)

// kumaDB builds a database with the tables and columns Uptime Kuma uses.
func kumaDB(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kuma.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE user (id INTEGER PRIMARY KEY, username VARCHAR(255), password VARCHAR(255), active BOOLEAN, timezone VARCHAR(150))`,
		`CREATE TABLE monitor (id INTEGER PRIMARY KEY, name VARCHAR(150), active BOOLEAN, user_id INT, interval INT, url TEXT, type VARCHAR(20),
			hostname VARCHAR(255), port INT, keyword VARCHAR(255), maxretries INT, ignore_tls BOOLEAN, upside_down BOOLEAN,
			maxredirects INT, accepted_statuscodes_json TEXT, retry_interval INT, method TEXT, body TEXT, headers TEXT,
			basic_auth_user TEXT, basic_auth_pass TEXT, parent INT, description TEXT, invert_keyword BOOLEAN, json_path TEXT,
			expected_value VARCHAR(255), timeout DOUBLE, expiry_notification BOOLEAN, dns_resolve_type VARCHAR(5), dns_resolve_server VARCHAR(255))`,
		`CREATE TABLE notification (id INTEGER PRIMARY KEY, name VARCHAR(255), config TEXT, active BOOLEAN, user_id INT, is_default BOOLEAN)`,
		`CREATE TABLE monitor_notification (id INTEGER PRIMARY KEY, monitor_id INT, notification_id INT)`,
		`CREATE TABLE tag (id INTEGER PRIMARY KEY, name VARCHAR(255), color VARCHAR(255))`,
		`CREATE TABLE monitor_tag (id INTEGER PRIMARY KEY, monitor_id INT, tag_id INT, value TEXT)`,
		`INSERT INTO user VALUES (1, 'admin', '$2a$10$abcdefghijklmnopqrstuuJ6s9Wq1Zr7Qk8wq0Z0m5v0Z0m5v0Z0m', 1, 'Asia/Shanghai')`,
		`INSERT INTO user VALUES (2, 'old', '$2a$10$abcdefghijklmnopqrstuuJ6s9Wq1Zr7Qk8wq0Z0m5v0Z0m5v0Z0m', 0, 'UTC')`,
		`INSERT INTO monitor (id, name, active, interval, url, type, accepted_statuscodes_json, maxretries, retry_interval, timeout, method, keyword)
			VALUES (1, 'Shop', 1, 60, 'https://shop.example.com/', 'keyword', '["200-299"]', 2, 30, 24, 'GET', 'Welcome')`,
		`INSERT INTO monitor (id, name, active, interval, hostname, port, type) VALUES (2, 'DB', 0, 30, '10.0.0.5', 5432, 'port')`,
		`INSERT INTO notification VALUES (1, 'Ops', '{"type":"webhook","webhookURL":"https://hooks.example.com/x","webhookContentType":"json"}', 1, 1, 0)`,
		`INSERT INTO monitor_notification (monitor_id, notification_id) VALUES (1, 1)`,
		`INSERT INTO tag VALUES (1, 'env', '#fff')`,
		`INSERT INTO monitor_tag (monitor_id, tag_id, value) VALUES (1, 1, 'prod')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestKumaDatabase(t *testing.T) {
	data := kumaDB(t)
	base := "@ IN SOA m. a.b. ( 2026102801 1m 15s 5s 3 )\nbj IN NODE region=north\n"
	_, existing, _ := config.Load("t", []byte(base), config.Options{})
	res, err := Kuma(data, existing, Options{Area: "region:north"})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + res.Text)
	if res.Monitors != 2 || res.Channels != 1 || len(res.Users) != 2 || !res.Users[0].Active || res.Users[1].Active {
		t.Fatalf("monitors %d channels %d users %+v", res.Monitors, res.Channels, res.Users)
	}
	if !strings.HasPrefix(res.Users[0].Hash, "$2a$") {
		t.Fatal("the password hash must be carried over")
	}
	for _, want := range []string{"$SCOPE region:north", "keyword=Welcome", "10.0.0.5:5432", "disabled=yes", "tags=env:prod"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("text lacks %q", want)
		}
	}
	for _, r := range res.Records {
		if (r.Type == "CHANNEL" || r.Type == "POLICY") != (r.Area == "") {
			t.Errorf("%s %s in section %q", r.Type, r.Name, r.Area)
		}
	}
	known := func(name string) bool { _, ok := res.Secrets[name]; return ok }
	if _, _, diags := config.Load("t", []byte(base+res.Text), config.Options{SecretKnown: known}); config.HasErrors(diags) {
		t.Fatalf("%v\n%s", diags, res.Text)
	}
	if _, err := Kuma([]byte("SQLite format 3\x00garbage"), nil, Options{}); err == nil {
		t.Fatal("a broken database was accepted")
	}
}
