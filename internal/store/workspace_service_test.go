package store

import (
	"errors"
	"testing"
)

func TestWorkspaceRuntimeResolver(t *testing.T) {
	s, _, _ := open(t)
	a, _, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	if e := x.Put("ref", "private", nil); e != nil {
		t.Fatal(e)
	}
	token, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.RuntimeSecrets(token, key)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := r.Resolve("Bearer ${secret:ref}"); e != nil || got != "Bearer private" {
		t.Fatalf("resolved %q %v", got, e)
	}
	t.Setenv("UNAGIKUMA_TEST_FALLBACK", "ambient")
	for _, input := range []string{"${env:UNAGIKUMA_TEST_FALLBACK}", "${secret:missing}", "${secret:ref} ${env:UNAGIKUMA_TEST_FALLBACK}", "${secret:}", "${secret:ref", "${unknown:ref}"} {
		if got, e := r.Resolve(input); e == nil || got != "" {
			t.Fatalf("unsafe resolution %q %q %v", input, got, e)
		}
	}
	if e = s.RevokeWorkspaceSecretReader(a); e != nil {
		t.Fatal(e)
	}
	if got, e := r.Resolve("literal"); !errors.Is(e, ErrTenantAccess) || got != "" {
		t.Fatalf("revoked literal %q %v", got, e)
	}
}

func TestWorkspaceRuntimeSecretAuthority(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	y, _ := s.WorkspaceSecrets(b, key)
	if e := x.Put("same", "a", nil); e != nil {
		t.Fatal(e)
	}
	if e := y.Put("same", "b", nil); e != nil {
		t.Fatal(e)
	}
	credential, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.RuntimeSecrets(credential, key)
	if e != nil {
		t.Fatal(e)
	}
	got, e := r.Get("same")
	if e != nil || got != "a" {
		t.Fatalf("bound read %q %v", got, e)
	}
	if _, e = s.RuntimeSecrets(string(b.WorkspaceID), key); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("guessed ID %v", e)
	}
	t.Setenv("ONLY_ENV", "ambient")
	if e = s.Secrets(key).Put("ONLY_ENV", "default", nil); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Get("ONLY_ENV"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("missing fallback %v", e)
	}
	// Runtime authority is not a cached interactive administrator.
	if _, e = s.core.Exec(`UPDATE users SET disabled=1 WHERE id=?`, a.UserID); e != nil {
		t.Fatal(e)
	}
	if got, e = r.Get("same"); e != nil || got != "a" {
		t.Fatalf("captured admin %q %v", got, e)
	}
	if _, e = s.core.Exec(`UPDATE users SET disabled=0 WHERE id=?`, a.UserID); e != nil {
		t.Fatal(e)
	}
	if e = s.RevokeWorkspaceSecretReader(a); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Get("same"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("revoked handle %v", e)
	}
	if _, e = s.RuntimeSecrets(credential, key); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("revoked credential %v", e)
	}
	next, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	r, e = s.RuntimeSecrets(next, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetWorkspaceRuntimeActive(a, false); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Get("same"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("inactive %v", e)
	}
	if _, e = s.IssueWorkspaceSecretReader(a); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("inactive issuance %v", e)
	}
	if e = s.SetWorkspaceRuntimeActive(a, true); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Get("same"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("resurrected %v", e)
	}
}
