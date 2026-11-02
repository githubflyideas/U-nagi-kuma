package app

import (
	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/probe"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// PreviewWorkspaceConfig validates an owned config with the same live admin,
// CAS and strict scoped dependency checks as apply, without issuing credentials.
// The caller MUST supply UserID from authenticated identity, not request data.
func (a *App) PreviewWorkspaceConfig(ctx store.WorkspaceContext, raw []byte, expected int64) (store.WorkspaceConfigState, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return store.WorkspaceConfigState{}, ErrSuperseded
	}
	return a.secrets.PreviewWorkspaceConfig(ctx, raw, expected, a.validateWorkspaceConfig)
}

// ApplyWorkspaceConfig publishes durable owned configuration and an encrypted
// supervisor credential together. It does NOT register monitors, apply the global
// engine, send notifications, or claim that a department scheduler is running.
func (a *App) ApplyWorkspaceConfig(ctx store.WorkspaceContext, raw []byte, expected int64) (store.WorkspaceConfigState, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return store.WorkspaceConfigState{}, ErrSuperseded
	}
	return a.secrets.ApplyWorkspaceConfig(ctx, raw, expected, a.validateWorkspaceConfig)
}
func (a *App) WorkspaceConfigState(ctx store.WorkspaceContext) (store.WorkspaceConfigState, error) {
	return a.store.WorkspaceConfigState(ctx)
}

// RestoreWorkspaceConfig is an installation-supervisor hook, NOT an HTTP API.
// It restores only the exact current scope/generation and revalidates dependencies.
// The model is private supervisor input (may contain literal user configuration);
// never serialize it as a DTO. The credential itself is never returned.
// The caller must cancel/join stale execution and gate each future execution on
// reader authority; this hook alone does not make the legacy engine scoped.
func (a *App) RestoreWorkspaceConfig(tenant store.TenantID, workspace store.WorkspaceID, generation int64) (*config.Config, *store.RuntimeSecretReader, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return nil, nil, ErrSuperseded
	}
	var model *config.Config
	_, reader, err := a.secrets.RestoreWorkspaceConfig(tenant, workspace, generation, func(raw []byte, resolve func(string) (string, error)) error {
		var err error
		model, err = a.parseWorkspaceConfig(raw, resolve)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return model, reader, nil
}

func (a *App) validateWorkspaceConfig(raw []byte, resolve func(string) (string, error)) error {
	_, err := a.parseWorkspaceConfig(raw, resolve)
	return err
}
func (a *App) parseWorkspaceConfig(raw []byte, resolve func(string) (string, error)) (*config.Config, error) {
	// Load validates typed parent/channel/policy/period/maintenance references within
	// this model. Never merge in default config or use the legacy global resolver.
	if len(raw) == 0 || len(raw) > MaxZoneBytes {
		return nil, store.ErrWorkspaceConfigInvalid
	}
	opt := a.validators()
	opt.SecretKnown = nil
	opt.AllowEnv = nil
	_, c, ds := config.Load("workspace.bind", raw, opt)
	if config.HasErrors(ds) {
		return nil, store.ErrWorkspaceConfigInvalid
	}
	// Distributed NODE routing and public status pages are not owned runtime paths.
	if len(c.Nodes) > 0 || len(c.StatusPages) > 0 {
		return nil, store.ErrWorkspaceConfigInvalid
	}
	for _, m := range c.Monitors {
		// Host commands and rename aliases have instance-global effects, not scope.
		if m.Opts["handler"] != "" || m.Opts["was"] != "" {
			return nil, store.ErrWorkspaceConfigInvalid
		}
		spec, err := ProbeSpec(m, resolve)
		if err != nil {
			return nil, store.ErrWorkspaceConfigInvalid
		}
		if probe.Validate(spec) != nil {
			return nil, store.ErrWorkspaceConfigInvalid
		}
	}
	for _, ch := range c.Channels {
		opts := make(map[string]string, len(ch.Opts))
		for k, v := range ch.Opts {
			rv, err := resolve(v)
			if err != nil {
				return nil, store.ErrWorkspaceConfigInvalid
			}
			opts[k] = rv
		}
		if ch.Kind != "group" && ValidateChannel(ch.Kind, opts) != nil {
			return nil, store.ErrWorkspaceConfigInvalid
		}
	}
	if err := ValidateWorkspaceNotifications(c); err != nil {
		return nil, err
	}
	return c, nil
}
