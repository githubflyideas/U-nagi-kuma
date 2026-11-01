package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func workspaceConfigFixture(t *testing.T) (*App, store.WorkspaceContext, store.WorkspaceContext) {
	t.Helper()
	a := runtimeApp(t)
	uid, err := a.store.CreateUser("workspace-owner", "password")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := a.store.DefaultTenant()
	if err != nil {
		t.Fatal(err)
	}
	owner := store.TenantContext{TenantID: tenant.ID, UserID: uid}
	wa, err := a.store.CreateWorkspace(owner, "A")
	if err != nil {
		t.Fatal(err)
	}
	wb, err := a.store.CreateWorkspace(owner, "B")
	if err != nil {
		t.Fatal(err)
	}
	ca, cb := store.WorkspaceContext{TenantContext: owner, WorkspaceID: wa.ID}, store.WorkspaceContext{TenantContext: owner, WorkspaceID: wb.ID}
	key, err := store.LoadOrCreateKey(a.opts.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		ctx   store.WorkspaceContext
		value string
	}{{ca, "department-A"}, {cb, "department-B"}} {
		repo, err := a.store.WorkspaceSecrets(row.ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Put("same", row.value, nil); err != nil {
			t.Fatal(err)
		}
	}
	return a, ca, cb
}

const workspaceBody = runtimeSOA + "check IN HTTP http://127.0.0.1:1/ header.X-Token=${secret:same}\n"

func TestWorkspaceConfigPublishRecovery(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	for _, row := range []struct {
		ctx   store.WorkspaceContext
		value string
	}{{ca, "department-A"}, {cb, "department-B"}} {
		preview, err := a.PreviewWorkspaceConfig(row.ctx, []byte(workspaceBody), 0)
		if err != nil || preview.Revision != 0 {
			t.Fatalf("preview: %+v %v", preview, err)
		}
		state, err := a.ApplyWorkspaceConfig(row.ctx, []byte(workspaceBody), 0)
		if err != nil || state.Revision != 1 || state.Generation < 1 {
			t.Fatalf("apply: %+v %v", state, err)
		}
		got, err := a.WorkspaceConfigState(row.ctx)
		if err != nil || got != state {
			t.Fatalf("readback: %+v %v", got, err)
		}
		c, reader, err := a.RestoreWorkspaceConfig(row.ctx.TenantID, row.ctx.WorkspaceID, state.Generation)
		if err != nil || len(c.Monitors) != 1 {
			t.Fatalf("restore: %v", err)
		}
		value, err := reader.Resolve("${secret:same}")
		if err != nil || value != row.value {
			t.Fatalf("scoped value: %q %v", value, err)
		}
		dto, err := json.Marshal(state)
		if err != nil || strings.Contains(string(dto), row.value) || strings.Contains(string(dto), "credential") {
			t.Fatal("unsafe metadata")
		}
	}
	if len(a.RuntimeConfig().Monitors) != 0 {
		t.Fatal("staging activated legacy scheduler")
	}
	dir := a.opts.DataDir
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(a.opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.store.Close()
	state, err := reopened.WorkspaceConfigState(ca)
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := reopened.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, state.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if v, e := reader.Get("same"); e != nil || v != "department-A" {
		t.Fatalf("restart: %q %v", v, e)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "monitors.bind"))
	if err != nil || string(raw) != runtimeSOA {
		t.Fatal("changed default config")
	}
}
