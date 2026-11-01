package server

import (
	"encoding/json"
	"net/http"

	"github.com/githubflyideas/unagikuma/internal/cluster"
)

// agentConnect upgrades a validated node under its own reconnect budget.
func (s *Server) agentConnect(w http.ResponseWriter, r *http.Request) {
	release, ok := s.admitAgent(w, r, false)
	if !ok {
		return
	}
	defer release()
	s.app.Cluster().ServeHTTP(w, r)
}

// agentState serves the master's state under a separate standby budget.
func (s *Server) agentState(w http.ResponseWriter, r *http.Request) {
	release, ok := s.admitAgent(w, r, true)
	if !ok {
		return
	}
	defer release()
	s.app.Cluster().ServeState(w, r)
}

// agentRetire takes the signed notice of the master that replaced this
// one: it steps down. Signature verification remains login-rate limited.
func (s *Server) agentRetire(w http.ResponseWriter, r *http.Request) {
	if ok, wait := s.login.Allow(s.ip(r), s.now()); !ok {
		writeError(w, rateLimited(wait))
		return
	}
	if r.TLS == nil {
		writeError(w, Errorf(http.StatusForbidden, CodeForbidden, "TLS required"))
		return
	}
	var n cluster.RetireNotice
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&n); err != nil {
		writeError(w, Errorf(http.StatusBadRequest, CodeInvalid, "%v", err))
		return
	}
	if err := s.app.AcceptRetire(n); err != nil {
		writeError(w, Errorf(http.StatusConflict, CodeConflict, "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
