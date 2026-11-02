package app

import (
	"context"
	"errors"
	"time"

	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/notify"
	"github.com/githubflyideas/unagikuma/internal/store"
)

var ErrWorkspaceNotificationUnsupported = errors.New("unsupported workspace notification configuration")

// ValidateWorkspaceNotifications is also callable by publication/import admission.
// This bounded executor supports one direct log/webhook route per policy. Reject
// global contact directory, on-call, fallback/group, rate/period, escalation and
// degraded-notification semantics rather than silently changing their meaning.
func ValidateWorkspaceNotifications(c *config.Config) error {
	for _, ch := range c.Channels {
		if (ch.Kind != "webhook" && ch.Kind != "log") || ch.Period != "" || ch.Rate != "" || ch.Retries != 0 || len(ch.Members) > 0 {
			return ErrWorkspaceNotificationUnsupported
		}
	}
	for _, p := range c.Policies {
		if len(p.Chain) > 1 || len(p.Network) > 0 || len(p.Escalate) > 0 || p.NotifyDegraded {
			return ErrWorkspaceNotificationUnsupported
		}
	}
	return nil
}
func workspaceNotificationPlan(c *config.Config, m *config.Monitor) store.WorkspaceNotificationPlan {
	plan := store.WorkspaceNotificationPlan{Policy: engine.DefaultNotifyPolicy(c.Tunables), Name: m.Name, Title: m.Title}
	p := c.FindPolicy(m.Policy)
	if p == nil {
		p = c.FindPolicy("default")
	}
	if p != nil {
		plan.Policy = engine.NotifyPolicy{Max: p.Max, Repeat: p.Repeat, Delay: p.Delay, ReopenWindow: p.ReopenWindow, Recovery: p.Recovery, NotifyDegraded: p.NotifyDegraded, AckExpire: p.AckExpire, AckClearOnChange: p.AckClearOnChange, RecoveryAfterAck: p.RecoveryAfterAck}
		if len(p.Chain) == 1 {
			plan.Channel = p.Chain[0]
		}
	}
	return plan
}
func (a *App) dispatchWorkspaceNotifications(ctx context.Context, c *config.Config, reader *store.RuntimeSecretReader, writer *store.WorkspaceRuntimeWriter) (string, error) {
	pending, err := writer.DispatchWorkspaceNotifications(ctx, func(ctx context.Context, name string, m *notify.Message) bool {
		ch := c.FindChannel(name)
		if ch == nil {
			return false
		}
		opts := map[string]string{}
		for k, v := range ch.Opts {
			s, e := reader.Resolve(v)
			if e != nil {
				return false
			}
			opts[k] = s
		}
		if writer.CheckNotificationAuthority() != nil {
			return false
		}
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		sender := notify.Sender{}
		return sender.Send(sendCtx, notify.Channel{Name: ch.Name, Kind: ch.Kind, Opts: opts}, m).OK
	})
	if err != nil {
		return "error", err
	}
	if pending {
		return "pending_delivery", nil
	}
	return "ready", nil
}

// DispatchWorkspaceNotifications is a trusted supervisor hook, not a user API.
// It drains restart-restored pending intents without running another probe.
func (a *App) DispatchWorkspaceNotifications(ctx context.Context, tenant store.TenantID, workspace store.WorkspaceID, generation int64) (string, error) {
	c, reader, err := a.RestoreWorkspaceConfig(tenant, workspace, generation)
	if err != nil {
		return "error", err
	}
	if err = ValidateWorkspaceNotifications(c); err != nil {
		return "blocked_unsupported_notifications", err
	}
	writer, err := a.store.BindWorkspaceRuntime(reader)
	if err != nil {
		return "error", err
	}
	ctx, finish, err := writer.BeginExecution(ctx)
	if err != nil {
		return "error", err
	}
	defer finish()
	return a.dispatchWorkspaceNotifications(ctx, c, reader, writer)
}
func (a *App) WorkspaceEvents(ctx store.WorkspaceContext, monitorID int64, limit int) ([]store.WorkspaceEvent, error) {
	return a.store.WorkspaceEvents(ctx, monitorID, limit)
}
func (a *App) WorkspaceIncidents(ctx store.WorkspaceContext, limit int) ([]store.WorkspaceIncident, error) {
	return a.store.WorkspaceIncidents(ctx, limit)
}
