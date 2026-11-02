package server

import (
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/store"
	"testing"
)

func TestWorkspaceHTTPManagement(t *testing.T) {
	e := newEnv(t, testBind, nil)
	if code := e.do("GET", "/api/v1/companies", nil, nil); code != 401 {
		t.Fatalf("anonymous %d", code)
	}
	e.setup()
	tenant, _ := e.app.Store().DefaultTenant()
	root := "/api/v1/companies/" + string(tenant.ID) + "/workspaces"
	var created struct {
		ID string `json:"id"`
	}
	if code := e.do("POST", root, map[string]string{"name": "Department A"}, &created); code != 200 || created.ID == "" {
		t.Fatalf("create %d %+v", code, created)
	}
	path := root + "/" + created.ID
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if code := e.do("GET", root, nil, &list); code != 200 || len(list.Items) != 2 {
		t.Fatalf("list %d %+v", code, list)
	}
	if code := e.do("GET", "/api/v1/companies/foreign/workspaces/"+created.ID, nil, nil); code != 403 {
		t.Fatalf("cross-owner %d", code)
	}
	var state store.WorkspaceConfigState
	if code := e.do("GET", path+"/config", nil, &state); code != 200 || state.Revision != 0 {
		t.Fatalf("state %d %+v", code, state)
	}
	body := map[string]string{"text": "@ IN SOA unagikuma. admin.localhost. ( 1 1s 500ms 300ms 2 )\nlocal IN HTTP http://127.0.0.1:1/ disabled=yes\n"}
	if code := e.do("PUT", path+"/config", body, nil); code != 428 {
		t.Fatalf("missing CAS %d", code)
	}
	if code := e.do("PUT", path+"/config", body, &state, "If-Match", "\"0\""); code != 200 || state.Revision != 1 {
		t.Fatalf("apply %d %+v", code, state)
	}
	if code := e.do("PUT", path+"/config", body, nil, "If-Match", "\"0\""); code != 412 {
		t.Fatalf("stale CAS %d", code)
	}
	if e.app.Config().Config.FindMonitor("local") != nil {
		t.Fatal("scoped config leaked into global")
	}
	defaults, _ := e.app.Store().Workspaces(store.TenantContext{TenantID: tenant.ID, UserID: 1})
	for _, ws := range defaults {
		if ws.IsDefault {
			if code := e.do("PUT", root+"/"+string(ws.ID)+"/config", body, nil, "If-Match", "\"0\""); code != 400 {
				t.Fatalf("default scoped publication %d", code)
			}
		}
	}
	if code := e.do("PUT", path, map[string]string{"name": "Renamed"}, nil); code != 204 {
		t.Fatalf("rename %d", code)
	}
	uid, err := e.app.Store().CreateUser("reader", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.app.Store().SetRole(uid, store.RoleViewer, nil); err != nil {
		t.Fatal(err)
	}
	if code := e.do("PUT", fmt.Sprintf("%s/members/%d", path, uid), map[string]any{"role": "viewer", "disabled": false}, nil); code != 204 {
		t.Fatalf("grant %d", code)
	}
	var login struct{ CSRF string }
	if code := e.do("POST", "/api/v1/session", map[string]string{"username": "reader", "password": "secret123"}, &login); code != 200 {
		t.Fatal(code)
	}
	e.csrf = login.CSRF
	if code := e.do("GET", path+"/monitors", nil, nil); code != 200 {
		t.Fatalf("viewer monitors %d", code)
	}
	if code := e.do("PUT", path+"/config", body, nil, "If-Match", "\"1\""); code != 403 {
		t.Fatalf("viewer write %d", code)
	}
	if code := e.do("POST", root, map[string]any{"name": "Spoof", "user_id": 1}, nil); code != 400 {
		t.Fatalf("untrusted user body %d", code)
	}
	adminID, _ := e.app.Store().UserID("admin")
	if err = e.app.Store().RemoveWorkspaceMember(store.WorkspaceContext{TenantContext: store.TenantContext{TenantID: tenant.ID, UserID: adminID}, WorkspaceID: store.WorkspaceID(created.ID)}, uid); err != nil {
		t.Fatal(err)
	}
	if code := e.do("GET", path+"/monitors", nil, nil); code != 403 {
		t.Fatalf("revoked read %d", code)
	}
}
