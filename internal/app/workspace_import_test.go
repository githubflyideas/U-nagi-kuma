package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWorkspaceImportDestinationCAS(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	data := []byte(fmt.Sprintf("endpoints:\n  - name: imported\n    url: %s\n    client: {timeout: 1s}\n    conditions: ['[STATUS] == 200']\n", target.URL))
	preview, err := a.PreviewWorkspaceImport(ca, "gatus", data, 0)
	if err != nil || preview.Monitors != 1 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	before, _ := a.WorkspaceConfigState(ca)
	if before.Revision != 0 || hits.Load() != 0 {
		t.Fatal("preview mutated")
	}
	state, _, err := a.ApplyWorkspaceImport(ca, "gatus", data, 0)
	if err != nil || state.Revision != 1 {
		t.Fatalf("apply %+v %v", state, err)
	}
	if _, _, err = a.ApplyWorkspaceImport(ca, "gatus", data, 0); !errors.Is(err, store.ErrWorkspaceConfigConflict) {
		t.Fatalf("stale %v", err)
	}
	if _, err = a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, state.Generation); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatal("imported loopback check did not execute")
	}
	other, _ := a.WorkspaceConfigState(cb)
	if other.Revision != 0 || len(a.Config().Config.Monitors) != 0 {
		t.Fatal("cross-scope mutation")
	}
	cases := []struct{ source, data string }{
		{"kuma", fmt.Sprintf(`{"version":"1.23.0","monitorList":[{"id":1,"name":"kuma-local","type":"http","url":%q,"interval":1,"timeout":1,"active":true,"maxretries":0,"accepted_statuscodes":["200"]}],"notificationList":[]}`, target.URL)},
		{"nagios", fmt.Sprintf("define command {\n command_name local-http\n command_line /usr/lib/nagios/plugins/check_http -H 127.0.0.1 -p %s -t 1\n}\ndefine host {\n host_name nagios-local\n address 127.0.0.1\n check_command local-http\n}\n", strings.Split(target.URL, ":")[2])},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			ws, err := a.store.CreateWorkspace(cb.TenantContext, tc.source)
			if err != nil {
				t.Fatal(err)
			}
			destination := store.WorkspaceContext{TenantContext: cb.TenantContext, WorkspaceID: ws.ID}
			expected := int64(0)
			p, err := a.PreviewWorkspaceImport(destination, tc.source, []byte(tc.data), expected)
			if err != nil || p.Blocked || p.Monitors != 1 {
				t.Fatalf("preview %+v %v", p, err)
			}
			st, _, err := a.ApplyWorkspaceImport(destination, tc.source, []byte(tc.data), expected)
			if err != nil {
				t.Fatal(err)
			}
			count := hits.Load()
			if _, err = a.RunWorkspaceChecks(context.Background(), destination.TenantID, destination.WorkspaceID, st.Generation); err != nil {
				t.Fatal(err)
			}
			if hits.Load() <= count {
				t.Fatal("imported check did not execute")
			}
		})
	}
	forged := ca
	forged.TenantID = "foreign"
	if _, err = a.PreviewWorkspaceImport(forged, "gatus", data, 0); !errors.Is(err, store.ErrTenantAccess) {
		t.Fatal(err)
	}
}
