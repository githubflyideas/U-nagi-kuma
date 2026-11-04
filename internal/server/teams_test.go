package server

import (
	"strings"
	"testing"
)

// Teams are written to monitors.bind as TEAM records; a monitor joins one
// with team=. Only administrators change teams, and a team that still
// owns monitors is not deleted.
func TestTeamsAPI(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	// A team named like a monitor is refused: it must not replace it.
	if code := e.do("POST", "/api/v1/teams", map[string]any{"name": "web", "managers": []string{"admin"}}, nil); code != 409 {
		t.Fatalf("team named like a monitor: %d", code)
	}
	if e.app.Config().Config.FindMonitor("web") == nil {
		t.Fatal("the monitor was replaced")
	}
	body := map[string]any{"name": "webteam", "title": "Web team", "managers": []string{"admin", "li"}, "operators": []string{"zhang"}}
	if code := e.do("POST", "/api/v1/teams", body, nil); code != 200 {
		t.Fatalf("save: %d", code)
	}
	text := e.app.Config().Text
	if !strings.Contains(text, "webteam") || !strings.Contains(text, "IN TEAM") || !strings.Contains(text, "managers=admin,li") {
		t.Fatalf("record not written:\n%s", text)
	}
	var list struct {
		Items []struct {
			Name, Title       string
			Managers, Unknown []string
			Monitors          int
		}
	}
	e.do("GET", "/api/v1/teams", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Title != "Web team" || len(list.Items[0].Managers) != 2 {
		t.Fatalf("list: %+v", list)
	}
	if u := list.Items[0].Unknown; len(u) != 2 || u[0] != "li" || u[1] != "zhang" {
		t.Fatalf("members without an account: %v", u)
	}
	// A monitor joins the team.
	upd := map[string]any{"record": map[string]any{"type": "HTTP", "target": "http://192.168.1.10/", "opts": map[string]string{"title": "Web site", "group": "public", "team": "webteam"}}}
	var mons struct{ Items []struct{ Name, Team string } }
	e.do("GET", "/api/v1/monitors", nil, &mons)
	name := ""
	for _, m := range mons.Items {
		if m.Name == "web" {
			name = m.Name
		}
	}
	if name == "" {
		t.Fatal("monitor web not found")
	}
	if code := e.do("PUT", "/api/v1/monitors/web", upd, nil); code != 200 {
		t.Fatalf("join team: %d", code)
	}
	e.do("GET", "/api/v1/monitors?q=team:webteam", nil, &mons)
	if len(mons.Items) != 1 || mons.Items[0].Team != "webteam" {
		t.Fatalf("team:webteam: %+v", mons.Items)
	}
	if code := e.do("DELETE", "/api/v1/teams/webteam", nil, nil); code != 409 {
		t.Fatalf("deleting a team that owns monitors: %d", code)
	}
	// A monitor naming a team that does not exist is refused.
	bad := map[string]any{"record": map[string]any{"name": "x1", "type": "TCP", "target": "10.0.0.9:22", "opts": map[string]string{"team": "nope"}}}
	if code := e.do("POST", "/api/v1/monitors", bad, nil); code != 400 && code != 422 {
		t.Fatalf("unknown team: %d", code)
	}
	upd["record"].(map[string]any)["opts"] = map[string]string{"title": "Web site", "group": "public"}
	if code := e.do("PUT", "/api/v1/monitors/web", upd, nil); code != 200 {
		t.Fatalf("leave team: %d", code)
	}
	if code := e.do("DELETE", "/api/v1/teams/webteam", nil, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if e.app.Config().Config.FindTeam("webteam") != nil {
		t.Fatal("team still there")
	}
}

// Managers who cannot be told of shared failures are named: no contact
// linked to their login, or none of their channels chosen.
func TestTeamManagersReachable(t *testing.T) {
	e := newEnv(t, peopleBind, nil)
	e.setup()
	if c := e.do("POST", "/api/v1/teams", map[string]any{"name": "web", "managers": []string{"li", "wang"}}, nil); c != 200 {
		t.Fatalf("team: %d", c)
	}
	if c := e.do("POST", "/api/v1/people", map[string]any{"name": "Li", "handle": "li", "emails": []string{"li@example.com"}, "notify": []string{"mail"}}, nil); c != 200 {
		t.Fatalf("person: %d", c)
	}
	unreachable := func() []string {
		var list struct {
			Items []struct{ Unreachable []string }
		}
		e.do("GET", "/api/v1/teams", nil, &list)
		return list.Items[0].Unreachable
	}
	// Li has a contact, but it is not linked to the login "li" yet.
	if got := unreachable(); len(got) != 2 {
		t.Fatalf("before linking: %v", got)
	}
	if c := e.do("POST", "/api/v1/users", map[string]any{"username": "li", "password": "secret123", "role": "operator"}, nil); c != 200 {
		t.Fatalf("user: %d", c)
	}
	var dir struct{ People []struct{ ID int64 } }
	e.do("GET", "/api/v1/people", nil, &dir)
	if c := e.do("PUT", "/api/v1/people/"+itoa(dir.People[0].ID), map[string]any{"name": "Li", "handle": "li", "username": "li", "emails": []string{"li@example.com"}, "notify": []string{"mail"}}, nil); c != 200 {
		t.Fatalf("link: %d", c)
	}
	if got := unreachable(); len(got) != 1 || got[0] != "wang" {
		t.Fatalf("after linking li: %v", got)
	}
}
