package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func TestWorkspaceConfigStrictReferenceGrammar(t *testing.T) {
	a, ca, _ := workspaceConfigFixture(t)
	key, e := store.LoadOrCreateKey(a.opts.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	repo, e := a.store.WorkspaceSecrets(ca, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = repo.Put("bad!name", "NOT-A-VALID-REFERENCE", nil); e != nil {
		t.Fatal(e)
	}
	raw := strings.ReplaceAll(workspaceBody, "secret:same", "secret:bad!name")
	if _, e = a.ApplyWorkspaceConfig(ca, []byte(raw), 0); !errors.Is(e, store.ErrSecretReference) {
		t.Fatalf("malformed existing reference accepted: %v", e)
	}
}

func TestWorkspaceConfigTypedAndResolvedValidation(t *testing.T) {
	a, ca, _ := workspaceConfigFixture(t)
	raw := workspaceBody + `office IN PERIOD "mon-fri 09:00-18:00"
 out IN CHANNEL log period=!office
 team IN CHANNEL group members=out
 pager IN POLICY chain=team network=out escalate=30m:out
 child IN HTTP http://127.0.0.1:1/ parent=check policy=pager
 window IN MAINT select="name:child" every=sun start=02:00 duration=1h
 `
	state, e := a.ApplyWorkspaceConfig(ca, []byte(raw), 0)
	if e != nil {
		t.Fatal(e)
	}
	model, r, e := a.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, state.Generation)
	if e != nil || model.Monitors[1].Parents[0] != "check" || model.Policies[0].Chain[0] != "team" || model.Channels[1].Members[0] != "out" || model.Maints[0].Monitors[0] != "child" {
		t.Fatalf("typed config: %v", e)
	}
	key, e := store.LoadOrCreateKey(a.opts.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	repo, e := a.store.WorkspaceSecrets(ca, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = repo.Put("bad-url", "PRIVATE-INVALID-URL", nil); e != nil {
		t.Fatal(e)
	}
	_, e = a.ApplyWorkspaceConfig(ca, []byte(runtimeSOA+"bad IN HTTP ${secret:bad-url}\n"), 1)
	if !errors.Is(e, store.ErrWorkspaceConfigInvalid) || strings.Contains(e.Error(), "PRIVATE-INVALID-URL") {
		t.Fatalf("resolved validation: %v", e)
	}
	if got, e := a.WorkspaceConfigState(ca); e != nil || got != state {
		t.Fatal("resolved validation changed snapshot")
	}
	if _, e = r.Get("same"); e != nil {
		t.Fatal("resolved validation revoked reader")
	}
	// A default value never repairs a missing owned dependency at restart.
	if e = a.secrets.Put("same", "DEFAULT-SECRET", nil); e != nil {
		t.Fatal(e)
	}
	if e = repo.Delete("same"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = a.RestoreWorkspaceConfig(ca.TenantID, ca.WorkspaceID, state.Generation); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("default fallback: %v", e)
	}
	if got, e := a.WorkspaceConfigState(ca); e != nil || got != state {
		t.Fatal("failed recovery changed state")
	}
}
