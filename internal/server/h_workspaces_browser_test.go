//go:build e2e

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
)

func TestWorkspaceRealBrowser(t *testing.T) {
 workspaceRealBrowser(t,"gatus,checkmate",2)
}
func TestWorkspaceGatusRealBrowser(t *testing.T) {
 workspaceRealBrowser(t,"gatus",1)
}
func workspaceRealBrowser(t *testing.T,sources string,minimum int64) {
	e := newEnv(t, "@ IN SOA unagikuma. admin.localhost. ( 1 1s 500ms 300ms 2 )\n", nil)
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	srv := New(e.app, Options{Web: os.DirFS("../../web/ui")})
	defer srv.Close()
	web := httptest.NewServer(srv.Handler())
	defer web.Close()
	out, err := exec.Command("node", "../../e2e/workspaces-live-ui.mjs", web.URL, target.URL,sources).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() < minimum {
		t.Fatalf("actual imported loopback checks: %d", hits.Load())
	}
}
