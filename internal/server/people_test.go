package server

import (
	"fmt"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/store"
)

const peopleBind = `@ IN SOA unagikuma. admin.localhost. ( 2026102801 1m 15s 5s 2 )
$SET api.login-rate=100/s
mail    IN CHANNEL email host=127.0.0.1 port=25 tls=none from=mon@example.com to="Li Lei <li@example.com>, wang@example.com"
sms     IN CHANNEL aliyun-sms access-key-id=a access-key-secret=b sign-name=c template-code=d phones=13800000002
default IN POLICY chain=mail,sms
`

// Addresses outside the directory are flagged, an import clears that,
// and marking someone as having left flags the channels that still name
// them and disables their login.
func TestContactDirectory(t *testing.T) {
	e := newEnv(t, peopleBind, nil)
	e.setup()
	type ch struct {
		Name   string
		Issues []struct{ Code, Address string }
	}
	issues := func() map[string][]string {
		var list []ch
		e.do("GET", "/api/v1/channels", nil, &list)
		out := map[string][]string{}
		for _, c := range list {
			for _, i := range c.Issues {
				out[c.Name] = append(out[c.Name], i.Code)
			}
		}
		return out
	}
	if got := issues(); len(got["mail"]) != 2 || len(got["sms"]) != 1 {
		t.Fatalf("before import: %v", got)
	}
	var pol []struct {
		Name          string
		IssueChannels []string `json:"issue_channels"`
	}
	e.do("GET", "/api/v1/policies", nil, &pol)
	if len(pol) == 0 || len(pol[0].IssueChannels) != 2 {
		t.Fatalf("policy red dot: %+v", pol)
	}
	var imp struct{ Created int }
	if c := e.do("POST", "/api/v1/people/import", map[string]any{}, &imp); c != 200 || imp.Created != 3 {
		t.Fatalf("import: %d %+v", c, imp)
	}
	if got := issues(); len(got) != 0 {
		t.Fatalf("after import: %v", got)
	}
	var dir struct {
		People []struct {
			ID       int64
			Handle   string
			Name     string
			Channels []string
		}
	}
	e.do("GET", "/api/v1/people", nil, &dir)
	var li int64
	for _, p := range dir.People {
		if p.Handle == "li" {
			li = p.ID
			if p.Name != "Li Lei" || len(p.Channels) != 1 {
				t.Fatalf("imported %+v", p)
			}
		}
	}
	// Link Li to a login, then Li leaves.
	if c := e.do("POST", "/api/v1/users", map[string]any{"username": "li", "password": "secret123", "role": "operator"}, nil); c != 200 {
		t.Fatalf("user: %d", c)
	}
	if c := e.do("PUT", fmt.Sprintf("/api/v1/people/%d", li), map[string]any{"handle": "li", "name": "Li Lei", "username": "li", "emails": []string{"li@example.com"}, "notify": []string{"mail"}}, nil); c != 200 {
		t.Fatalf("link: %d", c)
	}
	liWeb := e.loginAs("li", "secret123")
	if c := e.do("POST", fmt.Sprintf("/api/v1/people/%d/left", li), map[string]any{"left": true}, nil); c != 204 && c != 200 {
		t.Fatalf("left: %d", c)
	}
	if got := issues(); len(got["mail"]) != 1 || got["mail"][0] != "left" {
		t.Fatalf("after leaving: %v", got)
	}
	if c := liWeb.do("GET", "/api/v1/overview", nil, nil); c != 401 {
		t.Fatalf("session of someone who left: %d", c)
	}
	if c := e.do("POST", "/api/v1/session", map[string]string{"username": "li", "password": "secret123"}, nil); c == 200 {
		t.Fatal("someone who left can still log in")
	}
	// The channel form's check before saving.
	var lk struct {
		Unknown []string
		Issues  []struct{ Code string }
	}
	e.do("POST", "/api/v1/people/lookup", map[string]any{"kind": "email", "list": "li@example.com, new@example.com"}, &lk)
	if len(lk.Unknown) != 1 || lk.Unknown[0] != "new@example.com" || len(lk.Issues) != 2 {
		t.Fatalf("lookup: %+v", lk)
	}
}

// A contact can always be reached: at least one valid address, and the
// channels to reach them through, which must suit their addresses.
func TestContactReachable(t *testing.T) {
	e := newEnv(t, peopleBind, nil)
	e.setup()
	save := func(body map[string]any) int { return e.do("POST", "/api/v1/people", body, nil) }
	for name, c := range map[string]struct {
		body map[string]any
		code int
	}{
		"no address":          {map[string]any{"name": "Zhao", "handle": "zhao", "notify": []string{"mail"}}, 400},
		"bad e-mail":          {map[string]any{"name": "Zhao", "handle": "zhao", "emails": []string{"zhao@"}, "notify": []string{"mail"}}, 400},
		"bad phone":           {map[string]any{"name": "Zhao", "handle": "zhao", "phones": []string{"12"}, "notify": []string{"sms"}}, 400},
		"no channel":          {map[string]any{"name": "Zhao", "handle": "zhao", "emails": []string{"zhao@example.com"}}, 400},
		"unknown channel":     {map[string]any{"name": "Zhao", "handle": "zhao", "emails": []string{"zhao@example.com"}, "notify": []string{"pager"}}, 400},
		"sms without a phone": {map[string]any{"name": "Zhao", "handle": "zhao", "emails": []string{"zhao@example.com"}, "notify": []string{"sms"}}, 400},
	} {
		if got := save(c.body); got != c.code {
			t.Errorf("%s: %d, want %d", name, got, c.code)
		}
	}
	if c := save(map[string]any{"name": "Zhao", "handle": "zhao", "emails": []string{"Zhao <zhao@example.com>"}, "phones": []string{"+86 138 0000 0009"}, "notify": []string{"mail", "sms"}}); c != 200 {
		t.Fatalf("valid contact: %d", c)
	}
	// Someone saved before this rule, with no way chosen: flagged.
	if _, err := e.app.Store().SavePerson(store.Person{Handle: "old", Name: "Old", Emails: []string{"old@example.com"}}); err != nil {
		t.Fatal(err)
	}
	e.app.DirectoryChanged()
	var dir struct {
		People []struct {
			Handle string
			Notify []string
			Reach  string
		}
		Personal []struct{ Name, Contact string } `json:"personal_channels"`
	}
	e.do("GET", "/api/v1/people", nil, &dir)
	reach := map[string]string{}
	for _, p := range dir.People {
		reach[p.Handle] = p.Reach
	}
	if reach["zhao"] != "" || reach["old"] != "no_channel" {
		t.Errorf("reach: %v", reach)
	}
	if len(dir.Personal) != 2 {
		t.Errorf("personal channels: %+v", dir.Personal)
	}
	// Importing the addresses the channels use reaches each through them.
	var imp struct{ Created int }
	e.do("POST", "/api/v1/people/import", map[string]any{}, &imp)
	e.do("GET", "/api/v1/people", nil, &dir)
	for _, p := range dir.People {
		if p.Handle == "li" && (len(p.Notify) != 1 || p.Notify[0] != "mail" || p.Reach != "") {
			t.Errorf("imported li: %+v", p)
		}
	}
}
