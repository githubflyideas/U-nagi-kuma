package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

// Workspace routes use installation authentication only to establish UserID.
// Company/department authority always comes from the live repository membership.
func (s *Server) workspaceRoutes() {
	s.handle("GET /api/v1/companies", "read", s.workspaceCompanies)
	root := "/api/v1/companies/{company}/workspaces"
	s.handle("GET "+root, "read", s.workspaceList)
	s.handle("POST "+root, "read", workspaceWrite(s.workspaceCreate))
	s.handle("GET "+root+"/{workspace}", "read", s.workspaceGet)
	s.handle("PUT "+root+"/{workspace}", "read", workspaceWrite(s.workspaceRename))
	s.handle("DELETE "+root+"/{workspace}", "read", workspaceWrite(s.workspaceDelete))
	s.handle("GET "+root+"/{workspace}/members", "read", s.workspaceMembers)
	s.handle("PUT "+root+"/{workspace}/members/{member}", "read", workspaceWrite(s.workspaceMemberPut))
	s.handle("DELETE "+root+"/{workspace}/members/{member}", "read", workspaceWrite(s.workspaceMemberDelete))
	s.handle("GET "+root+"/{workspace}/config", "read", s.workspaceConfigGet)
	s.handle("PUT "+root+"/{workspace}/config", "read", workspaceWrite(s.workspaceConfigPut))
	s.handle("POST "+root+"/{workspace}/config/preview", "read", workspaceWrite(s.workspaceConfigPut))
	s.handle("GET "+root+"/{workspace}/monitors", "read", s.workspaceMonitors)
	s.handle("GET "+root+"/{workspace}/events", "read", s.workspaceEvents)
	s.handle("GET "+root+"/{workspace}/incidents", "read", s.workspaceIncidents)
	s.handle("GET "+root+"/{workspace}/monitors/{monitor}", "read", s.workspaceMonitor)
	s.handle("POST "+root+"/{workspace}/import/{source}", "read", workspaceWrite(s.workspaceImport))
}
func workspaceWrite(next apiFunc) apiFunc {
	return func(w http.ResponseWriter, r *http.Request) (any, error) {
		ai := authOf(r)
		if ai == nil || (!ai.session && !ai.scopes["write"] && !ai.scopes["admin"]) {
			return nil, workspaceError(store.ErrTenantAccess)
		}
		return next(w, r)
	}
}
func workspaceContext(r *http.Request) store.WorkspaceContext {
	return store.WorkspaceContext{TenantContext: store.TenantContext{TenantID: store.TenantID(r.PathValue("company")), UserID: authOf(r).userID}, WorkspaceID: store.WorkspaceID(r.PathValue("workspace"))}
}
func workspaceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrTenantAccess):
		return Errorf(403, CodeForbidden, "company or workspace access denied")
	case errors.Is(err, store.ErrWorkspaceConfigConflict):
		return Errorf(412, CodeConflict, "workspace revision changed; refresh and preview again")
	default:
		return Errorf(400, CodeInvalid, "workspace operation rejected; check configuration, dependencies and ownership")
	}
}
func workspaceDTO(v store.Workspace) map[string]any {
	return map[string]any{"id": v.ID, "company_id": v.TenantID, "name": v.Name, "is_default": v.IsDefault}
}
func (s *Server) workspaceCompanies(w http.ResponseWriter, r *http.Request) (any, error) {
	// Existing default-company discovery only. An explicit company route is also
	// accepted, but never grants authority without membership.
	t, err := s.app.Store().DefaultTenant()
	if err != nil {
		return nil, workspaceError(err)
	}
	_, err = s.app.Store().TenantMember(store.TenantContext{TenantID: t.ID, UserID: authOf(r).userID})
	items := []map[string]any{}
	if err == nil {
		items = append(items, map[string]any{"id": t.ID, "name": t.Name})
	} else if !errors.Is(err, store.ErrTenantAccess) {
		return nil, workspaceError(err)
	}
	return map[string]any{"items": items, "discovery": "default_company_only"}, nil
}
func (s *Server) workspaceList(w http.ResponseWriter, r *http.Request) (any, error) {
	rows, err := s.app.Store().Workspaces(workspaceContext(r).TenantContext)
	if err != nil {
		return nil, workspaceError(err)
	}
	items := []map[string]any{}
	for _, v := range rows {
		items = append(items, workspaceDTO(v))
	}
	return map[string]any{"items": items}, nil
}
func (s *Server) workspaceCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, 4096, &req); err != nil {
		return nil, err
	}
	v, err := s.app.Store().CreateWorkspace(workspaceContext(r).TenantContext, req.Name)
	if err != nil {
		return nil, workspaceError(err)
	}
	return workspaceDTO(v), nil
}
func (s *Server) workspaceGet(w http.ResponseWriter, r *http.Request) (any, error) {
	v, err := s.app.Store().Workspace(workspaceContext(r))
	if err != nil {
		return nil, workspaceError(err)
	}
	return workspaceDTO(v), nil
}
func (s *Server) workspaceRename(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, 4096, &req); err != nil {
		return nil, err
	}
	return nil, workspaceError(s.app.Store().RenameWorkspace(workspaceContext(r), req.Name))
}
func (s *Server) workspaceDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	return nil, workspaceError(s.app.Store().DeleteWorkspace(workspaceContext(r)))
}
func (s *Server) workspaceMembers(w http.ResponseWriter, r *http.Request) (any, error) {
	rows, err := s.app.Store().WorkspaceMembers(workspaceContext(r))
	if err != nil {
		return nil, workspaceError(err)
	}
	items := []map[string]any{}
	for _, v := range rows {
		items = append(items, map[string]any{"user_id": v.UserID, "role": v.Role, "disabled": v.Disabled})
	}
	return map[string]any{"items": items}, nil
}
func (s *Server) workspaceMemberPut(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeJSON(r, 4096, &req); err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(r.PathValue("member"), 10, 64)
	if err != nil || id <= 0 {
		return nil, workspaceError(store.ErrTenantAccess)
	}
	return nil, workspaceError(s.app.Store().SetWorkspaceMember(workspaceContext(r), id, req.Role, req.Disabled))
}
func (s *Server) workspaceMemberDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("member"), 10, 64)
	if err != nil || id <= 0 {
		return nil, workspaceError(store.ErrTenantAccess)
	}
	return nil, workspaceError(s.app.Store().RemoveWorkspaceMember(workspaceContext(r), id))
}
func workspaceRevision(r *http.Request) (int64, error) {
	value := r.Header.Get("If-Match")
	if value == "" {
		return 0, Errorf(428, CodeConflict, "If-Match workspace revision required")
	}
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, Errorf(400, CodeInvalid, "expected a quoted workspace revision")
	}
	raw := value[1 : len(value)-1]
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != raw {
		return 0, Errorf(400, CodeInvalid, "invalid workspace revision")
	}
	return n, nil
}
func (s *Server) workspaceConfigGet(w http.ResponseWriter, r *http.Request) (any, error) {
	state, err := s.app.WorkspaceConfigState(workspaceContext(r))
	if err != nil {
		return nil, workspaceError(err)
	}
	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", state.Revision))
	return state, nil
}
func (s *Server) workspaceConfigPut(w http.ResponseWriter, r *http.Request) (any, error) {
	ctx := workspaceContext(r)
	// Refuse inaccessible scope before parsing configuration or returning CAS info.
	if _, err := s.app.WorkspaceConfigState(ctx); err != nil {
		return nil, workspaceError(err)
	}
	owned, err := s.app.Store().Workspace(ctx)
	if err != nil {
		return nil, workspaceError(err)
	}
	if owned.IsDefault {
		return nil, Errorf(400, CodeInvalid, "default Workspace uses the legacy configuration API")
	}
	revision, err := workspaceRevision(r)
	if err != nil {
		return nil, err
	}
	var req struct {
		Text string `json:"text"`
	}
	if err = decodeJSON(r, 1<<20, &req); err != nil {
		return nil, err
	}
	var state store.WorkspaceConfigState
	if strings.HasSuffix(r.URL.Path, "/preview") {
		state, err = s.app.PreviewWorkspaceConfig(ctx, []byte(req.Text), revision)
	} else {
		state, err = s.app.ApplyWorkspaceConfig(ctx, []byte(req.Text), revision)
	}
	if err != nil {
		return nil, workspaceError(err)
	}
	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", state.Revision))
	return state, nil
}

// Explicit safe fields: probe reason/detail may include credentials echoed by a
// target and must not be exposed by these scoped APIs.
func (s *Server) workspaceMonitorRows(r *http.Request) ([]map[string]any, error) {
	repo, err := s.app.Store().WorkspaceMonitors(workspaceContext(r))
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(r.PathValue("monitor"), 10, 64)
	selected := ""
	if r.PathValue("monitor") != "" {
		if err != nil || id <= 0 {
			return nil, store.ErrTenantAccess
		}
		selected, err = repo.MonitorName(id)
		if err != nil {
			return nil, err
		}
	}
	ids, err := repo.MonitorIDs()
	if err != nil {
		return nil, err
	}
	states, err := repo.States()
	if err != nil {
		return nil, err
	}
	byName := map[string]map[string]any{}
	for name, id := range ids {
		if selected == "" || selected == name {
			byName[name] = map[string]any{"id": id, "name": workspaceMonitorDisplay(workspaceContext(r).WorkspaceID,name), "status": "pending", "last_check": nil}
		}
	}
	for _, state := range states {
		if v := byName[state.Name]; v != nil {
			v["status"] = state.Status
			v["latency_ms"] = state.LatencyMS
			if !state.LastCheck.IsZero() {
				v["last_check"] = state.LastCheck.UTC().Format(time.RFC3339Nano)
			}
		}
	}
	rows := []map[string]any{}
	for _, v := range byName {
		rows = append(rows, v)
	}
	return rows, nil
}
func workspaceMonitorDisplay(workspace store.WorkspaceID, key string) string {
 prefix:=store.WorkspaceMonitorKey(workspace, "")
 if strings.HasPrefix(key,prefix) {if raw,err:=base64.RawURLEncoding.DecodeString(strings.TrimPrefix(key,prefix));err==nil{return string(raw)}}
 return key
}

func (s *Server) workspaceMonitors(w http.ResponseWriter, r *http.Request) (any, error) {
	rows, err := s.workspaceMonitorRows(r)
	if err != nil {
		return nil, workspaceError(err)
	}
	status, err := s.app.WorkspaceRuntimeStatus(workspaceContext(r))
	if err != nil {
		return nil, workspaceError(err)
	}
	return map[string]any{"items": rows, "runtime": status}, nil
}
func (s *Server) workspaceMonitor(w http.ResponseWriter, r *http.Request) (any, error) {
	rows, err := s.workspaceMonitorRows(r)
	if err != nil {
		return nil, workspaceError(err)
	}
	if len(rows) != 1 {
		return nil, workspaceError(store.ErrTenantAccess)
	}
	return rows[0], nil
}
