package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func TestWorkspaceExecutionConcurrentScopes(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	arrived := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- r.Header.Get("X-Token")
		select {
		case <-r.Context().Done():
			return
		case <-release:
			w.WriteHeader(200)
		}
	}))
	defer server.Close()
	defer once.Do(func() { close(release) })
	body := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s timeout=5s header.X-Token=${secret:same}\n", server.URL))
	ast, e := a.ApplyWorkspaceConfig(ca, body, 0)
	if e != nil {
		t.Fatal(e)
	}
	bst, e := a.ApplyWorkspaceConfig(cb, body, 0)
	if e != nil {
		t.Fatal(e)
	}
	adone, bdone := make(chan error, 1), make(chan error, 1)
	go func() {
		_, e := a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, ast.Generation)
		adone <- e
	}()
	go func() {
		_, e := a.RunWorkspaceChecks(context.Background(), cb.TenantID, cb.WorkspaceID, bst.Generation)
		bdone <- e
	}()
	tokens := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case token := <-arrived:
			tokens[token] = true
		case <-time.After(2 * time.Second):
			t.Fatal("two scopes did not execute concurrently")
		}
	}
	if !tokens["department-A"] || !tokens["department-B"] {
		t.Fatalf("auth scopes %v", tokens)
	}
	if _, e = a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, ast.Generation); !errors.Is(e, store.ErrWorkspaceRuntimeBusy) {
		t.Fatalf("duplicate admission: %v", e)
	}
	if e = a.store.RevokeWorkspaceSecretReader(ca); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-adone:
		if e == nil {
			t.Fatal("revoked A succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("A not joined")
	}
	select {
	case e := <-bdone:
		t.Fatalf("A revoke interrupted B: %v", e)
	default:
	}
	once.Do(func() { close(release) })
	select {
	case e := <-bdone:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("B did not complete")
	}
	repo, e := a.store.WorkspaceMonitors(cb)
	if e != nil {
		t.Fatal(e)
	}
	states, e := repo.States()
	if e != nil || len(states) != 1 || states[0].Status != "ok" {
		t.Fatalf("B state %+v %v", states, e)
	}
	t.Log("both scopes entered same localhost target concurrently with distinct auth; duplicate A denied; revoking A joined only A, B completed and persisted OK")
}

func TestWorkspaceExecutionAdmissionAndFreshSecrets(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	var tokens []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokens = append(tokens, r.Header.Get("X-Token"))
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer server.Close()
	body := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s header.X-Token=${secret:same}\n", server.URL))
	denied := ca
	denied.UserID = 0
	if _, e := a.ApplyWorkspaceConfig(denied, body, 0); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("unauthorized publication %v", e)
	}
	uid, e := a.store.CreateUserRole("department-admin", "password", store.RoleViewer, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.store.SetWorkspaceMember(ca, uid, store.RoleAdmin, false); e != nil {
		t.Fatal(e)
	}
	issuer := ca
	issuer.UserID = uid
	foreign := cb
	foreign.UserID = uid
	if _, e = a.ApplyWorkspaceConfig(foreign, body, 0); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("ungranted workspace apply: %v", e)
	}
	cfg, e := a.ApplyWorkspaceConfig(issuer, body, 0)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.store.RemoveWorkspaceMember(ca, uid); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ApplyWorkspaceConfig(issuer, body, cfg.Revision); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("revoked admin apply: %v", e)
	}
	for _, req := range []struct {
		tenant     store.TenantID
		workspace  store.WorkspaceID
		generation int64
	}{{ca.TenantID, cb.WorkspaceID, cfg.Generation}, {"", ca.WorkspaceID, cfg.Generation}, {ca.TenantID, ca.WorkspaceID, cfg.Generation + 1}} {
		if _, e = a.RunWorkspaceChecks(context.Background(), req.tenant, req.workspace, req.generation); !errors.Is(e, store.ErrTenantAccess) {
			t.Fatalf("wrong scope/generation %v", e)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = a.RunWorkspaceChecks(cancelled, ca.TenantID, ca.WorkspaceID, cfg.Generation); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancelled admission %v", e)
	}
	if _, e = a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation); e != nil {
		t.Fatal(e)
	}
	key, e := store.LoadOrCreateKey(a.opts.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	secrets, e := a.store.WorkspaceSecrets(ca, key)
	if e != nil {
		t.Fatal(e)
	}
	if e = secrets.Put("same", "rotated-A", nil); e != nil {
		t.Fatal(e)
	}
	if _, e = a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation); e != nil {
		t.Fatal(e)
	}
	if e = secrets.Delete("same"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation); e == nil {
		t.Fatal("missing secret reused cached plaintext")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(tokens) != 2 || tokens[0] != "department-A" || tokens[1] != "rotated-A" {
		t.Fatalf("real HTTP tokens %v", tokens)
	}
	t.Logf("real HTTP tokens=%v; unauthorized/foreign/stale/cancelled/missing-secret calls emitted no requests", tokens)
}

func TestWorkspaceExecutionUnsupportedIsBlocked(t *testing.T) {
	for _, monitor := range []string{"check IN TCP 127.0.0.1:1\n", "check IN HTTP http://127.0.0.1:1/ timeout=6s\n", "parent IN HTTP http://127.0.0.1:1/\ncheck IN HTTP http://127.0.0.1:1/ parent=parent\n"} {
		t.Run(monitor, func(t *testing.T) {
			a, ca, _ := workspaceConfigFixture(t)
			cfg, e := a.ApplyWorkspaceConfig(ca, []byte(runtimeSOA+monitor), 0)
			if e != nil {
				t.Fatal(e)
			}
			result, e := a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation)
			if !errors.Is(e, ErrWorkspaceExecutionUnsupported) || result.NotificationStatus != "blocked_unscoped_notifications" {
				t.Fatalf("unsupported %+v %v", result, e)
			}
			repo, e := a.store.WorkspaceMonitors(ca)
			if e != nil {
				t.Fatal(e)
			}
			ids, e := repo.MonitorIDs()
			if e != nil || len(ids) != 0 {
				t.Fatalf("unsupported registered %+v %v", ids, e)
			}
		})
	}
}
