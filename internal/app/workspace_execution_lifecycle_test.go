package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

func TestWorkspaceExecutionRestartAndDisable(t *testing.T) {
	a, ca, cb := workspaceConfigFixture(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("X-Token") == "department-A" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	body := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s header.X-Token=${secret:same}\n", server.URL))
	cfgs := map[store.WorkspaceID]store.WorkspaceConfigState{}
	ids := map[store.WorkspaceID]int64{}
	for _, c := range []store.WorkspaceContext{ca, cb} {
		cfg, e := a.ApplyWorkspaceConfig(c, body, 0)
		if e != nil {
			t.Fatal(e)
		}
		cfgs[c.WorkspaceID] = cfg
		batch, e := a.RunWorkspaceChecks(context.Background(), c.TenantID, c.WorkspaceID, cfg.Generation)
		if e != nil {
			t.Fatal(e)
		}
		ids[c.WorkspaceID] = batch.Checks[0].ID
	}
	opts := a.opts
	if e := a.store.Close(); e != nil {
		t.Fatal(e)
	}
	a, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	defer a.store.Close()
	for _, c := range []store.WorkspaceContext{ca, cb} {
		batch, e := a.RunWorkspaceChecks(context.Background(), c.TenantID, c.WorkspaceID, cfgs[c.WorkspaceID].Generation)
		if e != nil {
			t.Fatal(e)
		}
		if batch.Checks[0].ID != ids[c.WorkspaceID] {
			t.Fatal("restart changed stable ID")
		}
		repo, e := a.store.WorkspaceMonitors(c)
		if e != nil {
			t.Fatal(e)
		}
		rows, e := repo.Rounds(ids[c.WorkspaceID], time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 10)
		if e != nil || len(rows) != 2 {
			t.Fatalf("restart history %+v %v", rows, e)
		}
		t.Logf("restart scope=%s id=%d status=%s rounds=%d", c.WorkspaceID, ids[c.WorkspaceID], batch.Checks[0].Status, len(rows))
	}
	paused := []byte(runtimeSOA + fmt.Sprintf("check IN HTTP %s disabled=yes header.X-Token=${secret:same}\n", server.URL))
	cfg, e := a.ApplyWorkspaceConfig(ca, paused, cfgs[ca.WorkspaceID].Revision)
	if e != nil {
		t.Fatal(e)
	}
	batch, e := a.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation)
	if e != nil || batch.Checks[0].Status != "paused" {
		t.Fatalf("disabled %+v %v", batch, e)
	}
	repo, e := a.store.WorkspaceMonitors(ca)
	if e != nil {
		t.Fatal(e)
	}
	states, e := repo.States()
	if e != nil || len(states) != 1 || states[0].Status != "paused" {
		t.Fatalf("disabled persisted state %+v %v", states, e)
	}
	if hits.Load() != 4 {
		t.Fatalf("disabled monitor probed: hits=%d", hits.Load())
	}
	rows, e := repo.Rounds(ids[ca.WorkspaceID], time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 10)
	if e != nil || len(rows) != 2 {
		t.Fatalf("pause invented history %+v %v", rows, e)
	}
	if e = a.store.SetWorkspaceRuntimeActive(ca, false); e != nil {
		t.Fatal(e)
	}
	if e = a.store.RevokeWorkspaceSecretReader(cb); e != nil {
		t.Fatal(e)
	}
	if e = a.store.Close(); e != nil {
		t.Fatal(e)
	}
	again, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	defer again.store.Close()
	for _, c := range []store.WorkspaceContext{ca, cb} {
		if _, e = again.RunWorkspaceChecks(context.Background(), c.TenantID, c.WorkspaceID, cfgs[c.WorkspaceID].Generation); !errors.Is(e, store.ErrTenantAccess) {
			t.Fatalf("restart resurrected revoked scope %v", e)
		}
	}
	if e = again.store.SetWorkspaceRuntimeActive(ca, true); e != nil {
		t.Fatal(e)
	}
	if _, e = again.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, cfg.Generation); !errors.Is(e, store.ErrTenantAccess) {
		t.Fatalf("enable resurrected credential: %v", e)
	}
	newCfg, e := again.ApplyWorkspaceConfig(ca, body, cfg.Revision)
	if e != nil {
		t.Fatal(e)
	}
	batch, e = again.RunWorkspaceChecks(context.Background(), ca.TenantID, ca.WorkspaceID, newCfg.Generation)
	if e != nil || batch.Checks[0].ID != ids[ca.WorkspaceID] {
		t.Fatalf("republish %+v %v", batch, e)
	}
	if hits.Load() != 5 {
		t.Fatalf("actual requests=%d", hits.Load())
	}
	t.Log("disabled monitor: persisted paused, zero additional probes/history; inactive/revoked denied after restart; enable requires new publication; republish probes with same stable ID")
}
