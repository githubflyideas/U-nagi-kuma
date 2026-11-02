package server

import (
	"errors"
	"github.com/githubflyideas/unagikuma/internal/app"
	"io"
	"net/http"
)

func (s *Server) workspaceImport(w http.ResponseWriter, r *http.Request) (any, error) {
	ctx := workspaceContext(r)
	if _, err := s.app.WorkspaceConfigState(ctx); err != nil {
		return nil, workspaceError(err)
	}
	revision, err := workspaceRevision(r)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return nil, Errorf(413, CodeTooLarge, "workspace import exceeds 1 MiB")
	}
	if r.URL.Query().Get("apply") == "1" {
		state, preview, err := s.app.ApplyWorkspaceImport(ctx, r.PathValue("source"), data, revision)
		if err != nil {
			return nil, workspaceImportError(err)
		}
		return map[string]any{"state": state, "monitors": preview.Monitors, "channels": preview.Channels}, nil
	}
	preview, err := s.app.PreviewWorkspaceImport(ctx, r.PathValue("source"), data, revision)
	if err != nil {
		return nil, workspaceImportError(err)
	}
	return preview, nil
}
func workspaceImportError(err error) error {
	if errors.Is(err, app.ErrWorkspaceImportCredentials) {
		return Errorf(400, CodeInvalid, "credential ingestion is blocked until atomic scoped secret publication is available; use scoped configuration with existing secret references")
	}
	return workspaceError(err)
}
