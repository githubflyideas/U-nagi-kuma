package server

import (
	"encoding/json"
	"net/http"

	"github.com/githubflyideas/unagikuma/internal/store"
)

// agentMerge takes what a node wrote while it checked on its own: GET
// returns the marks (what is already here), POST adds a batch. The site
// is the authenticated node.
func (s *Server) agentMerge(w http.ResponseWriter, r *http.Request) {
	m := s.app.Cluster()
	if m == nil {
		writeError(w, Errorf(http.StatusConflict, CodeConflict, "this server is not a master"))
		return
	}
	node, err := m.AuthAgent(r)
	if err != nil {
		// Only failures count against the address: a node merging often
		// must not crowd out its own state copies.
		if ok, wait := s.login.Allow(s.ip(r), s.now()); !ok {
			writeError(w, rateLimited(wait))
			return
		}
		writeError(w, Errorf(http.StatusUnauthorized, CodeUnauthorized, "%v", err))
		return
	}
	if r.Method == http.MethodGet {
		marks, err := s.app.MergeMarks(node)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, marks)
		return
	}
	var b store.MergeBatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&b); err != nil {
		writeError(w, Errorf(http.StatusBadRequest, CodeInvalid, "%v", err))
		return
	}
	res, err := s.app.MergeFrom(node, b)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
