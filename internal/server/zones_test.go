package server

import (
	"encoding/json"
	"strings"
	"testing"
)

const zoneAPIBind = "@ IN SOA local. admin.localhost. ( 1 1s 500ms 300ms 2 )\nweb IN HTTP http://127.0.0.1/ header.Authorization=private-zone-token\n"

func TestZonesRuntimeReads(t *testing.T) {
	e := newEnv(t, testBind, nil)
	e.setup()
	raw := strings.Replace(zoneAPIBind, "http://127.0.0.1/", "http://user:private-url-token@127.0.0.1/", 1) + "out IN CHANNEL log\n"
	var z map[string]any
	if code := e.do("POST", "/api/v1/zones/import", map[string]string{"name": "reads", "text": raw}, &z); code != 200 {
		t.Fatalf("import %d %+v", code, z)
	}
	id := z["id"].(string)
	name := "zone-" + id + ".web"
	if code := e.do("POST", "/api/v1/zones/"+id+"/apply", map[string]any{}, nil, "If-Match", z["revision"].(string)); code != 200 {
		t.Fatalf("apply %d", code)
	}
	var detail map[string]any
	if code := e.do("GET", "/api/v1/monitors/"+name, nil, &detail); code != 200 {
		t.Fatalf("detail %d", code)
	}
	row := detail["row"].(map[string]any)
	if row["interval_s"] == float64(0) || detail["readonly"] != true {
		t.Fatalf("missing runtime metadata %+v", detail)
	}
	var overview map[string]any
	e.do("GET", "/api/v1/overview", nil, &overview)
	overviewJSON, _ := json.Marshal(overview)
	if !strings.Contains(string(overviewJSON), "zone-"+id+".out") {
		t.Fatalf("applied zone channel missing from health: %s", overviewJSON)
	}
	for _, path := range []string{"/api/v1/hosts", "/api/v1/monitors", "/api/v1/monitors/" + name, "/api/v1/reports/availability"} {
		var out map[string]any
		if code := e.do("GET", path, nil, &out); code != 200 {
			t.Fatalf("%s %d", path, code)
		}
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "private-url-token") || strings.Contains(string(b), "private-zone-token") {
			t.Fatalf("secret in %s: %s", path, b)
		}
		if !strings.Contains(string(b), name) {
			t.Fatalf("runtime missing in %s: %s", path, b)
		}
	}
}

func TestZonesPermissionsLimitsAndDependencies(t *testing.T) {
	e := newEnv(t, testBind+"$SET api.login-rate=100/s\n", nil)
	e.setup()
	for _, role := range []string{"viewer", "operator"} {
		if code := e.do("POST", "/api/v1/users", map[string]any{"username": role, "password": "secret123", "role": role}, nil); code != 200 {
			t.Fatalf("user %d", code)
		}
		u := e.loginAs(role, "secret123")
		for _, path := range []string{"/zones/preview", "/zones/import", "/zones/nope/apply", "/zones/nope/rebind"} {
			if code := u.do("POST", "/api/v1"+path, map[string]any{}, nil); code != 403 {
				t.Fatalf("%s %s %d", role, path, code)
			}
		}
		if code := u.do("GET", "/api/v1/zones", nil, nil); code != 403 {
			t.Fatalf("list role %d", code)
		}
	}
	req := map[string]string{"name": "deps", "text": strings.Replace(zoneAPIBind, "private-zone-token", "${secret:zone-missing}", 1)}
	csrf := e.csrf
	e.csrf = ""
	if code := e.do("POST", "/api/v1/zones/import", req, nil); code != 403 {
		t.Fatalf("csrf %d", code)
	}
	e.csrf = csrf
	var p map[string]any
	if code := e.do("POST", "/api/v1/zones/preview", req, &p); code != 200 {
		t.Fatalf("preview %d", code)
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), "secret:zone-missing") {
		t.Fatalf("missing preview %s", b)
	}
	var z map[string]any
	e.do("POST", "/api/v1/zones/import", req, &z)
	var failure map[string]any
	if code := e.do("POST", "/api/v1/zones/"+z["id"].(string)+"/apply", map[string]any{}, &failure, "If-Match", z["revision"].(string)); code != 400 {
		t.Fatalf("dependencies %d", code)
	}
	b, _ = json.Marshal(failure)
	if !strings.Contains(string(b), "secret:zone-missing") {
		t.Fatalf("missing feedback %s", b)
	}
	if len(e.app.RuntimeConfig().Monitors) != len(e.app.Config().Config.Monitors) {
		t.Fatal("failed apply mutated runtime")
	}
	for _, path := range []string{"preview", "import"} {
		if code := e.do("POST", "/api/v1/zones/"+path, map[string]string{"text": strings.Repeat("x", 2<<20)}, nil); code != 413 {
			t.Fatalf("body limit %s %d", path, code)
		}
		if code := e.do("POST", "/api/v1/zones/"+path, map[string]string{"text": "private-parser-secret", "name": "bad"}, &failure); code != 400 {
			t.Fatalf("invalid %d", code)
		}
		b, _ = json.Marshal(failure)
		if strings.Contains(string(b), "private-parser-secret") {
			t.Fatal("parser error leaked")
		}
	}
}

func TestZonesAuthorizedDraftLifecycle(t *testing.T) {
	e := newEnv(t, testBind, nil)
	if code := e.do("GET", "/api/v1/zones", nil, nil); code != 401 {
		t.Fatalf("unauthorized list %d", code)
	}
	e.setup()
	base := e.app.Config().Text
	var preview struct{ Monitors, Missing []string }
	req := map[string]string{"name": "demo", "text": zoneAPIBind}
	if code := e.do("POST", "/api/v1/zones/preview", req, &preview); code != 200 {
		t.Fatalf("preview %d", code)
	}
	if len(preview.Monitors) != 1 {
		t.Fatalf("preview %+v", preview)
	}
	var z map[string]any
	if code := e.do("POST", "/api/v1/zones/import", req, &z); code != 200 {
		t.Fatalf("import %d: %+v", code, z)
	}
	b, _ := json.Marshal(z)
	if strings.Contains(string(b), "private-zone-token") || z["body"] != nil {
		t.Fatalf("unsafe response %s", b)
	}
	id, rev := z["id"].(string), z["revision"].(string)
	name := "zone-" + id + ".web"
	if e.app.RuntimeConfig().FindMonitor(name) != nil {
		t.Fatal("import applied implicitly")
	}
	if code := e.do("POST", "/api/v1/zones/import", req, nil); code != 409 {
		t.Fatalf("duplicate %d", code)
	}
	if code := e.do("POST", "/api/v1/zones/"+id+"/apply", map[string]any{}, nil); code != 428 {
		t.Fatalf("missing precondition %d", code)
	}
	if code := e.do("POST", "/api/v1/zones/"+id+"/apply", map[string]any{}, nil, "If-Match", "\"stale\""); code != 412 {
		t.Fatalf("stale %d", code)
	}
	if code := e.do("POST", "/api/v1/zones/"+id+"/apply", map[string]any{}, nil, "If-Match", rev); code != 200 {
		t.Fatalf("apply %d", code)
	}
	if e.app.Config().Text != base || e.app.Config().Config.FindMonitor(name) != nil || e.app.RuntimeConfig().FindMonitor(name) == nil {
		t.Fatal("base/draft isolation failed")
	}
	var ex map[string]any
	if code := e.do("GET", "/api/v1/zones/"+id+"/export", nil, &ex); code != 200 {
		t.Fatalf("export %d %+v", code, ex)
	}
	b, _ = json.Marshal(ex)
	if strings.Contains(string(b), "private-zone-token") {
		t.Fatalf("unsafe export %s", b)
	}
	var list map[string]any
	if code := e.do("GET", "/api/v1/zones", nil, &list); code != 200 {
		t.Fatalf("list %d", code)
	}
	b, _ = json.Marshal(list)
	if strings.Contains(string(b), "private-zone-token") || !strings.Contains(string(b), "applied_revision") {
		t.Fatalf("unsafe/incomplete list %s", b)
	}
	binding := map[string]any{"source_node": "upstream", "fingerprint": strings.Repeat("a", 64), "confirm": true}
	if code := e.do("POST", "/api/v1/zones/"+id+"/rebind", binding, &z, "If-Match", rev); code != 200 {
		t.Fatalf("rebind %d %+v", code, z)
	}
	if z["role"] != "secondary" || z["sync_status"] != "not_connected" {
		t.Fatalf("misleading binding %+v", z)
	}
	if code := e.do("POST", "/api/v1/zones/"+id+"/apply", map[string]any{}, nil, "If-Match", rev); code != 412 {
		t.Fatalf("stale after rebind %d", code)
	}
}
