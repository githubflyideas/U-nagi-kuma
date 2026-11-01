package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestWorkspaceSecretRotationRollback(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	y, _ := s.WorkspaceSecrets(b, key)
	legacy := s.Secrets(key)
	for _, e := range []error{x.Put("same", "A", nil), y.Put("same", "B", nil), legacy.Put("same", "default", nil)} {
		if e != nil {
			t.Fatal(e)
		}
	}
	// Fail after at least one rewrap has been attempted, inside the same transaction.
	if _, e := s.core.Exec(`CREATE TABLE rotation_count(n INTEGER); INSERT INTO rotation_count VALUES(0); CREATE TRIGGER rotation_fail AFTER UPDATE OF dek ON secrets BEGIN UPDATE rotation_count SET n=n+1; SELECT CASE WHEN (SELECT n FROM rotation_count)=2 THEN RAISE(ABORT,'injected rotation') END; END;`); e != nil {
		t.Fatal(e)
	}
	// Use persistent test-only schema so every pooled connection sees the injection.
	if e := legacy.Rotate(bytes.Repeat([]byte{9}, 32)); e == nil {
		t.Fatal("rotation accepted")
	}
	for _, v := range []struct {
		x    *WorkspaceSecretRepository
		want string
	}{{x, "A"}, {y, "B"}} {
		got, e := v.x.Get("same")
		if e != nil || got != v.want {
			t.Fatalf("rotation rollback %q %v", got, e)
		}
	}
	if got, e := legacy.Get("same"); e != nil || got != "default" {
		t.Fatalf("default rollback %q %v", got, e)
	}
}

func TestWorkspaceServiceGenerationAndDeletion(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	x, _ := s.WorkspaceSecrets(a, key)
	y, _ := s.WorkspaceSecrets(b, key)
	if e := x.Put("same", "A", nil); e != nil {
		t.Fatal(e)
	}
	if e := y.Put("same", "B", nil); e != nil {
		t.Fatal(e)
	}
	old, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.RuntimeSecrets(old, key)
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.IssueWorkspaceSecretReader(a)
	if e != nil {
		t.Fatal(e)
	}
	if next == old {
		t.Fatal("credential reused")
	}
	if _, e = r.Get("same"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("old generation %v", e)
	}
	if _, e = s.RuntimeSecrets(old, key); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("old credential %v", e)
	}
	token, e := s.IssueWorkspaceSecretReader(b)
	if e != nil {
		t.Fatal(e)
	}
	other, e := s.RuntimeSecrets(token, key)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := other.Get("same"); e != nil || got != "B" {
		t.Fatalf("scope B %q %v", got, e)
	}
	dto, e := json.Marshal(other)
	if e != nil || string(dto) != "{}" {
		t.Fatalf("reader DTO %s %v", dto, e)
	}
	var hash string
	if e = s.core.QueryRow(`SELECT token_hash FROM workspace_secret_readers WHERE workspace_id=?`, b.WorkspaceID).Scan(&hash); e != nil || hash == token {
		t.Fatalf("raw credential stored %v", e)
	}
	if e = s.DeleteWorkspace(b); e == nil {
		t.Fatal("deleted owned secrets")
	}
	if e = y.Delete("same"); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteWorkspace(b); e != nil {
		t.Fatal(e)
	}
	if _, e = other.Get("same"); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("deleted workspace %v", e)
	}
	if _, e = s.RuntimeSecrets(token, key); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("deleted credential %v", e)
	}
}
