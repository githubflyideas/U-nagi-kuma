package app

import (
	"errors"

	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/importer"
	"github.com/githubflyideas/unagikuma/internal/store"
)

var ErrWorkspaceImportCredentials = errors.New("workspace credential import requires atomic secret publication")

// WorkspaceImportPreview intentionally excludes target text, arbitrary importer
// notes and source IDs: even error descriptions can contain supplied credentials.
type WorkspaceImportPreview struct {
	Monitors int   `json:"monitors"`
	Channels int   `json:"channels"`
	Blocked  bool  `json:"blocked"`
	Revision int64 `json:"revision"`
}

const workspaceImportSOA = "@ IN SOA unagikuma. admin.localhost. ( 1 30s 5s 1s 2 )\n"

// workspaceImport builds against the destination only. No global config, users,
// resolver, secret inventory or $SCOPE area is consulted. Caller holds applyMu.
func (a *App) workspaceImport(ctx store.WorkspaceContext, source string, data []byte, expected int64) ([]byte, WorkspaceImportPreview, error) {
	var preview WorkspaceImportPreview
	state, err := a.store.WorkspaceConfigState(ctx)
	if err != nil {
		return nil, preview, err
	}
	owned, err := a.store.Workspace(ctx)
	if err != nil {
		return nil, preview, err
	}
	if owned.IsDefault || len(data) == 0 || len(data) > 1<<20 {
		return nil, preview, store.ErrWorkspaceConfigInvalid
	}
	if expected < 0 || state.Revision != expected {
		return nil, preview, store.ErrWorkspaceConfigConflict
	}
	preview.Revision = state.Revision
	raw := []byte(workspaceImportSOA)
	if state.Revision > 0 {
		raw, _, err = a.secrets.RestoreWorkspaceConfig(ctx.TenantID, ctx.WorkspaceID, state.Generation, a.validateWorkspaceConfig)
		if err != nil {
			return nil, preview, err
		}
	}
	opt := a.validators()
	opt.SecretKnown = nil
	opt.AllowEnv = nil
	f, c, ds := config.Load("workspace.bind", raw, opt)
	if config.HasErrors(ds) {
		return nil, preview, store.ErrWorkspaceConfigInvalid
	}
	var result *importer.Result
	options := importer.Options{File: "workspace upload"}
	switch source {
	case "gatus":
		result, err = importer.Gatus(data, c, options)
	case "checkmate":
		result, err = importer.Checkmate(data, c, options)
	case "kuma":
		result, err = importer.Kuma(data, c, options)
	case "nagios":
		result, err = importer.Nagios(data, c, options)
	default:
		return nil, preview, store.ErrWorkspaceConfigInvalid
	}
	if err != nil {
		return nil, preview, store.ErrWorkspaceConfigInvalid
	}
	preview.Monitors, preview.Channels, preview.Blocked = result.Monitors, result.Channels, result.ActivationError() != nil
	if preview.Blocked {
		return nil, preview, nil
	}
	// Never use the legacy importer's write-secret/undo pattern: a stale or invalid
	// config must leave destination credentials unchanged. Until atomic ingestion
	// is available, refuse generated credentials rather than broadening authority.
	if len(result.Secrets) > 0 {
		return nil, preview, ErrWorkspaceImportCredentials
	}
	if err = f.AppendBlock(result.Render(f.EndArea())); err != nil {
		return nil, preview, store.ErrWorkspaceConfigInvalid
	}
	return f.Bytes(), preview, nil
}
func (a *App) PreviewWorkspaceImport(ctx store.WorkspaceContext, source string, data []byte, expected int64) (WorkspaceImportPreview, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return WorkspaceImportPreview{}, ErrSuperseded
	}
	raw, preview, err := a.workspaceImport(ctx, source, data, expected)
	if err == nil && !preview.Blocked {
		_, err = a.secrets.PreviewWorkspaceConfig(ctx, raw, expected, a.validateWorkspaceConfig)
	}
	return preview, err
}
func (a *App) ApplyWorkspaceImport(ctx store.WorkspaceContext, source string, data []byte, expected int64) (store.WorkspaceConfigState, WorkspaceImportPreview, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	if a.SupersededBy() != nil {
		return store.WorkspaceConfigState{}, WorkspaceImportPreview{}, ErrSuperseded
	}
	raw, preview, err := a.workspaceImport(ctx, source, data, expected)
	if err != nil {
		return store.WorkspaceConfigState{}, preview, err
	}
	if preview.Blocked {
		return store.WorkspaceConfigState{}, preview, store.ErrWorkspaceConfigInvalid
	}
	state, err := a.secrets.ApplyWorkspaceConfig(ctx, raw, expected, a.validateWorkspaceConfig)
	return state, preview, err
}
