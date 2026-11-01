package store

import (
	"bytes"
	"errors"
	"testing"
)

func secretWorkspaceFixture(t *testing.T, s *Store) (WorkspaceContext, WorkspaceContext, []byte) {
	t.Helper()
	uid, e := s.CreateUser("secret-owner", "password")
	if e != nil {
		t.Fatal(e)
	}
	tenant, e := s.DefaultTenant()
	if e != nil {
		t.Fatal(e)
	}
	owner := TenantContext{tenant.ID, uid}
	a, e := s.CreateWorkspace(owner, "secret-A")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CreateWorkspace(owner, "secret-B")
	if e != nil {
		t.Fatal(e)
	}
	return WorkspaceContext{owner, a.ID}, WorkspaceContext{owner, b.ID}, bytes.Repeat([]byte{7}, 32)
}

func TestWorkspaceSecretsRotation(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	y, _ := s.WorkspaceSecrets(b, key)
	legacy := s.Secrets(key)
	for _, e := range []error{x.Put("same", "a", nil), y.Put("same", "b", nil), legacy.Put("same", "default", nil)} {
		if e != nil {
			t.Fatal(e)
		}
	}
	next := bytes.Repeat([]byte{8}, 32)
	if e := legacy.Rotate(next); e != nil {
		t.Fatal(e)
	}
	x, _ = s.WorkspaceSecrets(a, next)
	y, _ = s.WorkspaceSecrets(b, next)
	for _, v := range []struct {
		x    *WorkspaceSecretRepository
		want string
	}{{x, "a"}, {y, "b"}} {
		got, e := v.x.Get("same")
		if e != nil || got != v.want {
			t.Fatalf("rotation: %q %v", got, e)
		}
	}
	if got, e := legacy.Get("same"); e != nil || got != "default" {
		t.Fatalf("default rotation: %q %v", got, e)
	}
}

func TestWorkspaceSecretsIsolation(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x, e := s.WorkspaceSecrets(a, key)
	if e != nil {
		t.Fatal(e)
	}
	y, e := s.WorkspaceSecrets(b, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.Put("credential", "A-value", nil); e != nil {
		t.Fatal(e)
	}
	if e = y.Put("credential", "B-value", nil); e != nil {
		t.Fatal(e)
	}
	for _, row := range []struct {
		x     *WorkspaceSecretRepository
		value string
	}{{x, "A-value"}, {y, "B-value"}} {
		got, e := row.x.Get("credential")
		if e != nil || got != row.value {
			t.Fatalf("value %q %v", got, e)
		}
	}
	legacy := s.Secrets(key)
	if _, e = legacy.Get("credential"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("legacy leaked %v", e)
	}
	if e = legacy.Put("credential", "default-value", nil); e != nil {
		t.Fatal(e)
	}
	if e = x.Delete("credential"); e != nil {
		t.Fatal(e)
	}
	if _, e = x.Get("credential"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("fallback %v", e)
	}
	got, e := y.Get("credential")
	if e != nil || got != "B-value" {
		t.Fatalf("delete crossed boundary %q %v", got, e)
	}
	got, e = legacy.Get("credential")
	if e != nil || got != "default-value" {
		t.Fatalf("legacy %q %v", got, e)
	}
}
