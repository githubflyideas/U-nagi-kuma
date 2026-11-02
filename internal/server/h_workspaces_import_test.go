package server

import (
	"encoding/json"
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceHTTPImports(t *testing.T) {
	e := newEnv(t, "@ IN SOA unagikuma. admin.localhost. ( 1 1s 500ms 300ms 2 )\n", nil)
	e.setup()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer target.Close()
	tenant, _ := e.app.Store().DefaultTenant()
	uid, _ := e.app.Store().UserID("admin")
	owner := store.TenantContext{TenantID: tenant.ID, UserID: uid}
	cases := map[string]string{
		"gatus":     fmt.Sprintf("endpoints:\n  - name: imported\n    url: %s\n    client: {timeout: 1s}\n    conditions: ['[STATUS] == 200']\n", target.URL),
		"checkmate": fmt.Sprintf(`[{"name":"imported","type":"http","url":%q,"method":"GET","interval":1000,"isActive":true,"proxyMode":"none","statusWindowSize":1,"statusWindowThreshold":100}]`, target.URL),
		"kuma":      fmt.Sprintf(`{"version":"1.23.0","monitorList":[{"id":1,"name":"imported","type":"http","url":%q,"interval":1,"timeout":1,"active":true,"maxretries":0,"accepted_statuscodes":["200"]}],"notificationList":[]}`, target.URL),
		"nagios":    fmt.Sprintf("define command {\n command_name local-http\n command_line /usr/lib/nagios/plugins/check_http -H 127.0.0.1 -p %s -t 1\n}\ndefine host {\n host_name imported\n address 127.0.0.1\n check_command local-http\n}\n", strings.Split(target.URL, ":")[2]),
	}
	for source, data := range cases {
		t.Run(source, func(t *testing.T) {
			ws, err := e.app.Store().CreateWorkspace(owner, source)
			if err != nil {
				t.Fatal(err)
			}
			root := "/api/v1/companies/" + string(tenant.ID) + "/workspaces/" + string(ws.ID)
			request := func(body, query, csrf, revision string) (int, string) {
				req, _ := http.NewRequest("POST", e.ts.URL+root+"/import/"+source+query, strings.NewReader(body))
				req.Header.Set("X-Unagikuma-CSRF", csrf)
				if revision != "" {
					req.Header.Set("If-Match", revision)
				}
				response, err := e.c.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				raw, _ := io.ReadAll(response.Body)
				return response.StatusCode, string(raw)
			}
			if code, _ := request(data, "", "", `"0"`); code != 403 {
				t.Fatalf("CSRF %d", code)
			}
			if code, _ := request(data, "", e.csrf, ""); code != 428 {
				t.Fatalf("CAS %d", code)
			}
			code, raw := request(data, "", e.csrf, `"0"`)
			if code != 200 {
				t.Fatalf("preview %d %s", code, raw)
			}
			var preview struct {
				Monitors int
				Blocked  bool
			}
			if json.Unmarshal([]byte(raw), &preview) != nil || preview.Monitors != 1 || preview.Blocked {
				t.Fatal(raw)
			}
			ctx := store.WorkspaceContext{TenantContext: owner, WorkspaceID: ws.ID}
			st, _ := e.app.WorkspaceConfigState(ctx)
			if st.Revision != 0 {
				t.Fatal("preview mutated")
			}
			code, raw = request(data, "?apply=1", e.csrf, `"0"`)
			if code != 200 {
				t.Fatalf("apply %d %s", code, raw)
			}
			if code, _ = request(data, "?apply=1", e.csrf, `"0"`); code != 412 {
				t.Fatalf("stale %d", code)
			}
			if source != "checkmate" {
				deadline := time.Now().Add(5 * time.Second)
				var rows struct {
					Items []struct {
						ID     int64
						Status string
					}
				}
				for time.Now().Before(deadline) {
					if code := e.do("GET", root+"/monitors", nil, &rows); code != 200 {
						t.Fatal(code)
					}
					if len(rows.Items) > 0 && rows.Items[0].Status == "ok" {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}
				if len(rows.Items) != 1 || rows.Items[0].Status != "ok" {
					t.Fatalf("real loopback runtime %+v", rows)
				}
				foreign, err := e.app.Store().CreateWorkspace(owner, "foreign-"+source)
				if err != nil {
					t.Fatal(err)
				}
				foreignPath := "/api/v1/companies/" + string(tenant.ID) + "/workspaces/" + string(foreign.ID) + fmt.Sprintf("/monitors/%d", rows.Items[0].ID)
				if code := e.do("GET", foreignPath, nil, nil); code != 403 {
					t.Fatalf("cross-Workspace monitor ID %d", code)
				}
			}
		})
	}
	if len(e.app.Config().Config.Monitors) != 0 {
		t.Fatal("legacy config mutated")
	}
}
