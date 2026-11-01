package store

import (
	"bytes"
	"testing"
)

func configValidator(body []byte, resolve func(string) (string, error)) error {
	_, e := resolve(string(body))
	return e
}

func TestWorkspaceConfigKeyRotation(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x := s.Secrets(key)
	states := []WorkspaceConfigState{}
	for _, ctx := range []WorkspaceContext{a, b} {
		repo, e := s.WorkspaceSecrets(ctx, key)
		if e != nil {
			t.Fatal(e)
		}
		if e = repo.Put("same", string(ctx.WorkspaceID), nil); e != nil {
			t.Fatal(e)
		}
		state, e := x.ApplyWorkspaceConfig(ctx, []byte("${secret:same}"), 0, configValidator)
		if e != nil {
			t.Fatal(e)
		}
		states = append(states, state)
	}
	next := bytes.Repeat([]byte{8}, 32)
	if e := x.Rotate(next); e != nil {
		t.Fatal(e)
	}
	for _, state := range states {
		body, r, e := x.RestoreWorkspaceConfig(state.TenantID, state.WorkspaceID, state.Generation, configValidator)
		if e != nil || string(body) != "${secret:same}" {
			t.Fatalf("restored after rotation: %v", e)
		}
		if got, e := r.Get("same"); e != nil || got != string(state.WorkspaceID) {
			t.Fatalf("rotation scope: %q %v", got, e)
		}
	}
}
