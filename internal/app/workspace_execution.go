package app

import (
	"context"
	"errors"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/store"
)

var ErrWorkspaceExecutionUnsupported = errors.New("unsupported workspace check execution configuration")

type WorkspaceCheck struct {
	ID     int64
	Name   string
	Status string
}
type WorkspaceCheckBatch struct {
	NotificationStatus string
	Checks             []WorkspaceCheck
}

// RunWorkspaceChecks is a trusted installation-supervisor hook, NOT an HTTP/user
// API. It performs one bounded pass in this App/shared Store, not a second engine
// or tenant App. The caller owns scheduling and must cancel/join before closing
// the App. No model or resolved probe is retained between passes.
func (a *App) RunWorkspaceChecks(ctx context.Context, tenant store.TenantID, workspace store.WorkspaceID, generation int64) (WorkspaceCheckBatch, error) {
	return a.runWorkspaceChecks(ctx, tenant, workspace, generation, nil)
}

// due is private supervisor state: nil means a manual full pass. Full definitions
// are always reconciled before selecting due checks, never a partial registry.
func (a *App) runWorkspaceChecks(ctx context.Context, tenant store.TenantID, workspace store.WorkspaceID, generation int64, due map[string]time.Time) (out WorkspaceCheckBatch, err error) {
	out.NotificationStatus = "blocked_unscoped_notifications"
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, reader, err := a.RestoreWorkspaceConfig(tenant, workspace, generation)
	if err != nil {
		return out, err
	}
	// Fail closed for semantics this check-only executor cannot honor. Notification
	// configuration is retained but explicitly blocked, never compiled/dispatched.
	if len(c.Monitors) > 64 || len(c.Maints) > 0 {
		return out, ErrWorkspaceExecutionUnsupported
	}
	for _, m := range c.Monitors {
		if m.Type != "HTTP" || len(m.Parents) > 0 || len(m.Nodes) > 0 || m.Timeout > 5*time.Second {
			return out, ErrWorkspaceExecutionUnsupported
		}
	}
	writer, err := a.store.BindWorkspaceRuntime(reader)
	if err != nil {
		return out, err
	}
	ctx, finish, err := writer.BeginExecution(ctx)
	if err != nil {
		return out, err
	}
	defer finish()
	defs := make([]store.MonitorDef, 0, len(c.Monitors))
	for _, m := range c.Monitors {
		defs = append(defs, store.MonitorDef{Name: m.Name, DefHash: m.DefHash})
	}
	ids, err := writer.Register(defs)
	if err != nil {
		return out, err
	}
	for _, m := range c.Monitors {
		if due != nil && time.Now().Before(due[m.Name]) {
			continue
		}
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if _, err = reader.Resolve(""); err != nil {
			return out, err
		}
		if m.Disabled {
			if err = writer.Pause(ids[m.Name]); err != nil {
				return out, err
			}
			out.Checks = append(out.Checks, WorkspaceCheck{ids[m.Name], m.Name, engine.StatusPaused})
			if due != nil {
				due[m.Name] = time.Now().Add(m.Interval)
			}
			continue
		}
		spec, e := ProbeSpec(m, reader.Resolve)
		if e != nil {
			return out, e
		}
		spec.Name = store.WorkspaceMonitorKey(workspace, m.Name)
		certDefaults(&spec, c.Tunables)
		if _, err = reader.Resolve(""); err != nil {
			return out, err
		}
		result := probe.Run(ctx, spec)
		if err = ctx.Err(); err != nil {
			return out, err
		}
		state, e := writer.Record(ids[m.Name], result, m.Interval, engine.StepParams{Attempts: m.Attempts, RecoverAttempts: m.RecoverAttempts, OtherDegraded: result.Degraded, Slow: m.Degraded > 0 && result.Latency > m.Degraded})
		if e != nil {
			return out, e
		}
		out.Checks = append(out.Checks, WorkspaceCheck{ids[m.Name], m.Name, state.Status})
		if due != nil {
			due[m.Name] = time.Now().Add(m.Interval)
		}
	}
	return out, nil
}
