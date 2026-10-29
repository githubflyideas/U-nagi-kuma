package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/app"
	"github.com/githubflyideas/unagikuma/internal/cluster"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// joinCode issues a one-time join code for another machine.
func (s *Server) joinCode(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Server string `json:"server"`
	}
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	server := strings.TrimRight(strings.TrimSpace(req.Server), "/")
	if server == "" {
		server = agentServerURL(r)
	}
	if !strings.HasPrefix(server, "https://") {
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "the master address must start with https://")
	}
	code, exp, err := s.app.CreateJoinCode(server)
	if err != nil {
		return nil, err
	}
	return map[string]any{"code": code, "expires": exp, "server": server, "fingerprint": s.app.PKI().Fingerprint()}, nil
}

// agentJoin is called by a machine joining with a code (no session: the
// code is the credential). TLS only, since the answer carries a token.
func (s *Server) agentJoin(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		writeError(w, Errorf(http.StatusForbidden, CodeForbidden, "joining requires TLS"))
		return
	}
	if ok, wait := s.login.Allow(s.ip(r), s.now()); !ok {
		writeError(w, rateLimited(wait))
		return
	}
	var req cluster.JoinRequest
	if err := decodeJSON(r, s.maxBody()*4, &req); err != nil {
		writeError(w, err)
		return
	}
	resp, err := s.app.AcceptJoin(req)
	switch {
	case errors.Is(err, store.ErrJoinCode):
		s.log.Warn("join refused", "ip", s.ip(r), "node", req.Node, "err", err)
		writeError(w, Errorf(http.StatusForbidden, CodeForbidden, "%v", err))
	case errors.Is(err, app.ErrNodeName):
		writeError(w, Errorf(http.StatusConflict, CodeConflict, "%v", err))
	case errors.Is(err, app.ErrQuota):
		writeError(w, quotaError(err))
	case err != nil:
		writeError(w, s.configError(err))
	default:
		writeJSON(w, http.StatusOK, resp)
	}
}

// clusterJoin makes this server join a cluster with a code from its master.
func (s *Server) clusterJoin(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Code   string `json:"code"`
		Node   string `json:"node"`
		Title  string `json:"title"`
		Region string `json:"region"`
		Import bool   `json:"import"`
	}
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	if len(s.nodeViews()) > 1 {
		return nil, Errorf(http.StatusConflict, CodeConflict, "this server is the master of a cluster; it cannot join another")
	}
	resp, err := s.app.JoinCluster(r.Context(), app.JoinOptions{Code: req.Code, Node: req.Node, Title: req.Title, Region: req.Region,
		Import: req.Import, Author: authOf(r).user})
	if err != nil {
		if errors.Is(err, app.ErrAlreadyJoined) {
			return nil, Errorf(http.StatusConflict, CodeConflict, "%v", err)
		}
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "%v", err)
	}
	return map[string]any{"node": resp.Node, "imported": resp.Imported, "skipped": resp.Skipped}, nil
}

func (s *Server) clusterAgent(w http.ResponseWriter, r *http.Request) (any, error) {
	st := s.app.Standby()
	st.Key = "" // never shown
	return map[string]any{"member": s.app.Agent().Member, "status": s.app.Agent().Status, "standby": st}, nil
}

// clusterPromote makes this standby the master, with the copy of the
// master's state it holds; the server restarts.
func (s *Server) clusterPromote(w http.ResponseWriter, r *http.Request) (any, error) {
	term, err := s.app.Promote(r.Context(), 0, authOf(r).user)
	if err != nil {
		return nil, Errorf(http.StatusConflict, CodeConflict, "%v", err)
	}
	auditOf(r).note("term", term)
	return map[string]any{"term": term, "restarting": true}, nil
}

func (s *Server) clusterLeave(w http.ResponseWriter, r *http.Request) (any, error) {
	return map[string]any{"ok": true}, s.app.LeaveCluster()
}
