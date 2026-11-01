//go:build e2e && windows

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This is an explicit subprocess supervisor, NOT Windows SCM validation.
// It exercises the real server's exit-3 contract and a real second startup.
func TestStandbyWindowsSupervisedRecovery(t *testing.T) {
	a := start(t, "supervised-master", fmt.Sprintf(soa, "a"))
	a.setup("admin", "master-pass-1")
	b := start(t, "supervised-standby", fmt.Sprintf(soa, "b"))
	b.setup("local", "standby-pass-1")
	var jc struct{ Code string }
	if code := a.call("POST", "/api/v1/cluster/join-codes", map[string]string{"server": fmt.Sprintf("https://127.0.0.1:%d", a.port)}, &jc); code != 200 {
		t.Fatalf("join code: %d", code)
	}
	if code := b.call("POST", "/api/v1/cluster/join", map[string]any{"code": jc.Code, "node": "bk"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	if code := a.call("PUT", "/api/v1/nodes/bk", map[string]any{"standby": fmt.Sprintf("https://127.0.0.1:%d", b.port)}, nil); code != 200 {
		t.Fatalf("standby: %d", code)
	}
	b.waitFor("snapshot", func() bool {
		var r struct{ Standby struct{ Size int64 } }
		b.call("GET", "/api/v1/cluster/agent", nil, &r)
		return r.Standby.Size > 0
	})
	a.stop() // Fence this fixture's source before explicit disaster promotion.
	b.waitFor("master disconnected", func() bool {
		var r struct{ Status *struct{ Connected bool } }
		b.call("GET", "/api/v1/cluster/agent", nil, &r)
		return r.Status != nil && !r.Status.Connected
	})
	if code := b.call("POST", "/api/v1/cluster/promote", map[string]any{}, nil); code != 200 {
		t.Fatalf("promote: %d", code)
	}
	exited := make(chan error, 1)
	go func() { exited <- b.cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil || b.cmd.ProcessState.ExitCode() != 3 {
			t.Fatalf("expected restart exit 3, got %v (%v)", err, b.cmd.ProcessState)
		}
	case <-time.After(20 * time.Second):
		_ = b.cmd.Process.Kill()
		<-exited
		t.Fatal("promotion did not exit for supervisor")
	}
	if _, err := os.Stat(filepath.Join(b.dir, "promote.json")); err != nil {
		t.Fatalf("restart intent missing: %v", err)
	}
	b.run() // The test supervisor deliberately restarts once, on exit 3 only.
	if !b.login("admin", "master-pass-1") {
		t.Fatal("restarted master did not restore source credentials")
	}
	var ov struct{ Term uint64 }
	if code := b.call("GET", "/api/v1/overview", nil, &ov); code != 200 || ov.Term < 1 {
		t.Fatalf("promoted overview: code=%d term=%d", code, ov.Term)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "promote.json")); !os.IsNotExist(err) {
		t.Fatalf("restart intent not consumed: %v", err)
	}
	t.Log("Real exit 3 and supervised restart/restore passed; SCM installation/recovery not tested")
}
