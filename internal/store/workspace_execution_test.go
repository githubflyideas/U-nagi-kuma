package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"testing"
	"time"
)

func executionWriter(t *testing.T, s *Store, c WorkspaceContext, key []byte) *WorkspaceRuntimeWriter {
	t.Helper()
	x := s.Secrets(key)
	state, e := x.ApplyWorkspaceConfig(c, []byte("check"), 0, configValidator)
	if e != nil {
		t.Fatal(e)
	}
	_, reader, e := x.RestoreWorkspaceConfig(c.TenantID, c.WorkspaceID, state.Generation, configValidator)
	if e != nil {
		t.Fatal(e)
	}
	w, e := s.BindWorkspaceRuntime(reader)
	if e != nil {
		t.Fatal(e)
	}
	return w
}
func TestWorkspaceExecutionWriterAuthority(t *testing.T) {
	s, _, _ := open(t)
	a, b, key := secretWorkspaceFixture(t, s)
	wa, wb := executionWriter(t, s, a, key), executionWriter(t, s, b, key)
	ia, e := wa.Register([]MonitorDef{{"check", "hash"}})
	if e != nil {
		t.Fatal(e)
	}
	ib, e := wb.Register([]MonitorDef{{"check", "hash"}})
	if e != nil || ia["check"] == ib["check"] {
		t.Fatalf("identity %v %v %v", ia, ib, e)
	}
	ctx, finish, e := wa.BeginExecution(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
	if _, _, e = wa.BeginExecution(context.Background()); !errors.Is(e, ErrWorkspaceRuntimeBusy) {
		t.Fatalf("concurrent admission: %v", e)
	}
	bctx, bfinish, e := wb.BeginExecution(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer bfinish()
	result := probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Detail: "must not persist secret details"}
	if _, e = wa.Record(ib["check"], result, time.Second, engine.StepParams{}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("cross workspace write %v", e)
	}
	if _, e = (&WorkspaceRuntimeWriter{}).Record(ia["check"], result, time.Second, engine.StepParams{}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("zero writer %v", e)
	}
	if _, e = s.BindWorkspaceRuntime(&RuntimeSecretReader{}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("forged reader %v", e)
	}
	unauthorized := a
	unauthorized.UserID = 0
	if e = s.RevokeWorkspaceSecretReader(unauthorized); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("unauthorized revoke %v", e)
	}
	if ctx.Err() != nil {
		t.Fatal("unauthorized revoke cancelled scope")
	}
	if _, e = s.core.Exec(`CREATE TRIGGER reject_runtime_revoke BEFORE UPDATE ON workspace_secret_readers BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if e = s.RevokeWorkspaceSecretReader(a); e == nil {
		t.Fatal("injected revoke succeeded")
	}
	if ctx.Err() != nil {
		t.Fatal("failed transaction cancelled scope")
	}
	if _, e = s.core.Exec(`DROP TRIGGER reject_runtime_revoke`); e != nil {
		t.Fatal(e)
	}
	if e = s.RevokeWorkspaceSecretReader(a); e != nil {
		t.Fatal(e)
	}
	if ctx.Err() == nil || bctx.Err() != nil {
		t.Fatal("revocation cancellation crossed scope or absent")
	}
	if _, e = wa.Register([]MonitorDef{{"late", "hash"}}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("stale registration %v", e)
	}
	if _, e = wa.Record(ia["check"], result, time.Second, engine.StepParams{}); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("stale result %v", e)
	}
	if _, e = wb.Record(ib["check"], result, time.Second, engine.StepParams{}); e != nil {
		t.Fatal(e)
	}
	repo, e := s.WorkspaceMonitors(b)
	if e != nil {
		t.Fatal(e)
	}
	states, e := repo.States()
	if e != nil || len(states) != 1 || states[0].Detail != "" {
		t.Fatalf("state %+v %v", states, e)
	}
	if _, e = repo.Rounds(ia["check"], time.Time{}, time.Now().Add(time.Hour), 10); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("foreign history read %v", e)
	}
	if _, e = repo.MonitorName(ia["check"]); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("foreign identity read %v", e)
	}
	token, e := s.IssueWorkspaceSecretReader(b)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := s.RuntimeSecrets(token, key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BindWorkspaceRuntime(reader); !errors.Is(e, ErrTenantAccess) {
		t.Fatalf("secret-only reader gained execution %v", e)
	}
}

func TestWorkspaceExecutionCapacity(t *testing.T) {
	s, _, _ := open(t)
	a, _, key := secretWorkspaceFixture(t, s)
	var writers []*WorkspaceRuntimeWriter
	var finishes []func()
	defer func() {
		for _, finish := range finishes {
			finish()
		}
	}()
	for i := 0; i < 9; i++ {
		ws, e := s.CreateWorkspace(a.TenantContext, fmt.Sprintf("capacity-%d", i))
		if e != nil {
			t.Fatal(e)
		}
		w := executionWriter(t, s, WorkspaceContext{a.TenantContext, ws.ID}, key)
		writers = append(writers, w)
		_, finish, e := w.BeginExecution(context.Background())
		if i == 8 {
			if !errors.Is(e, ErrWorkspaceRuntimeBusy) {
				t.Fatalf("over capacity %v", e)
			}
		} else {
			if e != nil {
				t.Fatal(e)
			}
			finishes = append(finishes, finish)
		}
	}
	finishes[0]()
	_, finish, e := writers[8].BeginExecution(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer finish()
}

func TestWorkspaceExecutionWriteRollback(t *testing.T) {
	s, _, _ := open(t)
	a, _, key := secretWorkspaceFixture(t, s)
	w := executionWriter(t, s, a, key)
	ids, e := w.Register([]MonitorDef{{"check", "old"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`CREATE TRIGGER reject_runtime_retire BEFORE UPDATE OF retired_at ON monitors BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = w.Register([]MonitorDef{{"replacement", "new"}}); e == nil {
		t.Fatal("late registry failure accepted")
	}
	repo, e := s.WorkspaceMonitors(a)
	if e != nil {
		t.Fatal(e)
	}
	got, e := repo.MonitorIDs()
	if e != nil || len(got) != 1 || got[WorkspaceMonitorKey(a.WorkspaceID, "check")] != ids["check"] {
		t.Fatalf("registration rollback %+v %v", got, e)
	}
	if _, e = s.core.Exec(`DROP TRIGGER reject_runtime_retire`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.core.Exec(`CREATE TRIGGER reject_runtime_state BEFORE INSERT ON monitor_state BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = w.Record(ids["check"], probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Latency: time.Millisecond}, time.Second, engine.StepParams{}); e == nil {
		t.Fatal("state write error hidden")
	}
	if _, e = s.core.Exec(`DROP TRIGGER reject_runtime_state`); e != nil {
		t.Fatal(e)
	}
	if rows, e := repo.Rounds(ids["check"], time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 10); e != nil || len(rows) != 0 {
		t.Fatalf("failed state wrote history %+v %v", rows, e)
	}
	for _, table := range []string{"rounds", "agg5m", "hourly"} {
		t.Run(table, func(t *testing.T) {
			if _, e = s.hist.Exec(`CREATE TRIGGER reject_runtime_history BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
				t.Fatal(e)
			}
			_, e = w.Record(ids["check"], probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Latency: time.Millisecond}, time.Second, engine.StepParams{})
			if e == nil {
				t.Fatal("history error hidden")
			}
			if _, e = s.hist.Exec(`DROP TRIGGER reject_runtime_history`); e != nil {
				t.Fatal(e)
			}
			states, e := repo.States()
			if e != nil || len(states) != 0 {
				t.Fatalf("state rollback %+v %v", states, e)
			}
			rows, e := repo.Rounds(ids["check"], time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 10)
			if e != nil || len(rows) != 0 {
				t.Fatalf("history rollback %+v %v", rows, e)
			}
		})
	}
	if _, e = w.Record(ids["check"], probe.Result{Vote: probe.VoteOK, Reason: probe.ReasonOK, Latency: time.Millisecond}, time.Second, engine.StepParams{}); e != nil {
		t.Fatal(e)
	}
}
