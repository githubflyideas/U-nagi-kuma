package config

import (
	"errors"
	"strings"
	"testing"
)

// A team lists its people by user name, one role each; a person can be
// in several teams and manage several. Monitors join with team= (also
// through $DEFAULT); the rest are shared infrastructure.
func TestTeams(t *testing.T) {
	const head = "$FILE revision=1\nc IN CHANNEL log\nops IN POLICY chain=c\n"
	_, cfg, ds := load(t, head+`web  IN TEAM title="Web team" managers=li,wang operators=zhang viewers=zhao policy=ops
db   IN TEAM managers=li viewers=zhang
shop IN HTTP https://shop.example.com/ team=web
$DEFAULT team=db
pg   IN TCP 10.0.0.5:5432
$DEFAULT
gw   IN PING 10.0.0.1
`)
	if e := errorsOf(ds); len(e) > 0 {
		t.Fatal(e)
	}
	web, db := cfg.FindTeam("web"), cfg.FindTeam("db")
	if web == nil || db == nil || web.Title != "Web team" || db.Title != "db" || web.Policy != "ops" {
		t.Fatalf("teams: %+v %+v", web, db)
	}
	for _, c := range []struct {
		team *Team
		user string
		role string
	}{
		{web, "li", "manager"}, {db, "li", "manager"}, // manages two teams
		{web, "zhang", "operator"}, {db, "zhang", "viewer"}, // a different role in each
		{web, "zhao", "viewer"}, {db, "zhao", ""}, {web, "nobody", ""},
	} {
		if got := c.team.Role(c.user); got != c.role {
			t.Errorf("%s in %s: %q, want %q", c.user, c.team.Name, got, c.role)
		}
	}
	for name, team := range map[string]string{"shop": "web", "pg": "db", "gw": ""} {
		if m := cfg.FindMonitor(name); m.Team != team {
			t.Errorf("%s: team %q, want %q", name, m.Team, team)
		}
	}
	sel, _ := ParseSelector("team:web")
	if !sel.Match(cfg.FindMonitor("shop")) || sel.Match(cfg.FindMonitor("gw")) {
		t.Error("team: selector")
	}

	for src, code := range map[string]string{
		"x IN HTTP https://x/ team=nope\n":                  "unknown_team",
		"web IN TEAM policy=nope\n":                         "unknown_policy",
		"web IN TEAM managers=li operators=li\n":            "invalid_value",
		"web IN TEAM colour=red\n":                          "unknown_option",
		"web IN TEAM somewhere\n":                           "syntax",
		"web IN TEAM managers=li\nweb IN HTTP https://x/\n": "duplicate_name",
	} {
		_, _, ds := load(t, head+src)
		if !strings.Contains(errCodes(ds), code) {
			t.Errorf("%q: want %s, got %v", src, code, ds)
		}
	}
}

func errCodes(ds []Diagnostic) string {
	var b strings.Builder
	for _, d := range errorsOf(ds) {
		b.WriteString(d.Code + " ")
	}
	return b.String()
}

// Saving a record never replaces one of another kind with the same name:
// a team or policy named like a monitor is refused, while a monitor may
// change its check type.
func TestUpsertKeepsOtherKinds(t *testing.T) {
	src := "$FILE revision=1\nc IN CHANNEL log\nweb IN HTTP https://example.com/\n"
	f, _ := Parse("x", []byte(src))
	for _, typ := range []string{"TEAM", "POLICY", "CHANNEL"} {
		err := f.UpsertRecord(RecordSpec{Name: "web", Type: typ, Opts: []Opt{{Key: "chain", Value: "c"}}})
		if !errors.Is(err, ErrNameTaken) {
			t.Errorf("%s named like a monitor: %v", typ, err)
		}
	}
	if string(f.Bytes()) != src {
		t.Fatalf("file changed:\n%s", f.Bytes())
	}
	if err := f.UpsertRecord(RecordSpec{Name: "web", Type: "TCP", Target: "example.com:443"}); err != nil {
		t.Fatalf("a monitor changing its type: %v", err)
	}
	if err := f.UpsertRecord(RecordSpec{Name: "c", Type: "channel", Target: "log"}); err != nil {
		t.Fatalf("same kind, other case: %v", err)
	}
}
