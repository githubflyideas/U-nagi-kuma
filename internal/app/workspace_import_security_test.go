package app

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/store"
	"testing"
)

func TestWorkspaceImportExistingScopedReferences(t *testing.T) {
	a, ctx, _ := workspaceConfigFixture(t)
	if _, err := a.ApplyWorkspaceConfig(ctx, []byte(workspaceBody), 0); err != nil {
		t.Fatal(err)
	}
	data := []byte("endpoints:\n  - name: appended\n    url: http://127.0.0.1:1\n    enabled: false\n    client: {timeout: 1s}\n    conditions: ['[STATUS] == 200']\n")
	if _, err := a.PreviewWorkspaceImport(ctx, "gatus", data, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.ApplyWorkspaceImport(ctx, "gatus", data, 1); err != nil {
		t.Fatal(err)
	}
	st, _ := a.WorkspaceConfigState(ctx)
	_, reader, err := a.RestoreWorkspaceConfig(ctx.TenantID, ctx.WorkspaceID, st.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := reader.Get("same"); err != nil || value != "department-A" {
		t.Fatal("destination reference changed")
	}
	bad := []byte("endpoints:\n  - name: credential\n    url: http://127.0.0.1:1\n    headers: {Authorization: 'Bearer private-value'}\n    conditions: ['[STATUS] == 200']\n")
	if _, _, err = a.ApplyWorkspaceImport(ctx, "gatus", bad, 2); !errors.Is(err, ErrWorkspaceImportCredentials) {
		t.Fatalf("credential safety boundary: %v", err)
	}
	after, _ := a.WorkspaceConfigState(ctx)
	if after != st {
		t.Fatal("rejected credential import mutated config")
	}
	revoked := ctx
	revoked.UserID = 0
	if _, err = a.PreviewWorkspaceImport(revoked, "gatus", data, 2); !errors.Is(err, store.ErrTenantAccess) {
		t.Fatal(err)
	}
}
