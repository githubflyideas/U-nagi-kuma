package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/githubflyideas/unagikuma/internal/app"
	"github.com/githubflyideas/unagikuma/internal/oncall"
)

// oncallView is the on-call page: every rotation with who is on call and
// the next shifts, and the response times of the stats window.
func (s *Server) oncallView(w http.ResponseWriter, r *http.Request) (any, error) {
	ts := s.app.Config().Config.Tunables
	rots, err := s.app.OnCall(ts.Int("oncall.shifts-shown"))
	if err != nil {
		return nil, err
	}
	stats, err := s.app.OnCallStats(ts.Duration("oncall.stats-window"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"rotations": rots, "stats": stats}, nil
}

func (s *Server) oncallOverrideCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Rotation string    `json:"rotation"`
		Member   string    `json:"member"`
		From     time.Time `json:"from"`
		Until    time.Time `json:"until"`
		Note     string    `json:"note"`
	}
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	if req.From.IsZero() {
		req.From = s.now()
	}
	o, err := s.app.AddOverride(oncall.Override{Rotation: req.Rotation, Member: req.Member, From: req.From, Until: req.Until,
		Note: req.Note, By: authOf(r).user})
	if errors.Is(err, app.ErrOnCall) {
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "%v", err)
	}
	if err != nil {
		return nil, err
	}
	auditOf(r).target = o.Rotation + ": " + o.Member
	return o, nil
}

func (s *Server) oncallOverrideDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, notFound("stand-in")
	}
	ok, err := s.app.DeleteOverride(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound("stand-in")
	}
	return map[string]any{"deleted": id}, nil
}
