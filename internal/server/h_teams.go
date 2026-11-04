package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
)

// Teams are TEAM records in monitors.bind: who owns which monitors and
// with which role. Keeping them in the file means they reach replicas
// with the rest of the configuration and every change is in its history.

type teamJSON struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Managers    []string `json:"managers"`
	Operators   []string `json:"operators"`
	Viewers     []string `json:"viewers"`
	Policy      string   `json:"policy,omitempty"`
	Monitors    int      `json:"monitors"`
	ETag        string   `json:"etag,omitempty"`
	// Unknown are member names with no user account (yet).
	Unknown []string `json:"unknown,omitempty"`
	// Unreachable are managers the system cannot tell when the shared
	// infrastructure fails: no contact linked to their login, or no way
	// to reach them chosen there.
	Unreachable []string `json:"unreachable,omitempty"`
}

func (s *Server) teams(w http.ResponseWriter, r *http.Request) (any, error) {
	cfg := s.app.Config().Config
	users := map[string]bool{}
	if list, err := s.app.Store().Users(); err == nil {
		for _, u := range list {
			users[u.Username] = true
		}
	}
	etags := s.recordETags()
	pc := personalChannels(cfg)
	reach := map[string]bool{}
	for _, p := range s.app.Directory().People() {
		if p.Username != "" && reachOf(p, pc) == "" {
			reach[p.Username] = true
		}
	}
	out := []teamJSON{}
	ts := teamOf(r)
	for _, t := range cfg.Teams {
		if ts != nil && ts.roles[t.Name] == "" {
			continue // a team member sees their own teams only
		}
		tj := teamJSON{Name: t.Name, Title: t.Title, Description: t.Description, Policy: t.Policy,
			Managers: nonNil(t.Managers), Operators: nonNil(t.Operators), Viewers: nonNil(t.Viewers), ETag: etags[t.Name]}
		for _, m := range cfg.Monitors {
			if m.Team == t.Name {
				tj.Monitors++
			}
		}
		for _, u := range t.Managers {
			if !reach[u] {
				tj.Unreachable = append(tj.Unreachable, u)
			}
		}
		for _, list := range [][]string{t.Managers, t.Operators, t.Viewers} {
			for _, u := range list {
				if !users[u] {
					tj.Unknown = append(tj.Unknown, u)
				}
			}
		}
		out = append(out, tj)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return map[string]any{"items": out}, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (s *Server) teamSave(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Name        string   `json:"name"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Managers    []string `json:"managers"`
		Operators   []string `json:"operators"`
		Viewers     []string `json:"viewers"`
		Policy      string   `json:"policy"`
		Message     string   `json:"message"`
	}
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	if !config.ValidName(req.Name) {
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "a team needs a valid name")
	}
	var opts []config.Opt
	add := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			opts = append(opts, config.Opt{Key: k, Value: v})
		}
	}
	list := func(names []string) string {
		var out []string
		for _, n := range names {
			if n = strings.TrimSpace(n); n != "" {
				out = append(out, n)
			}
		}
		return strings.Join(out, ",")
	}
	add("title", req.Title)
	add("managers", list(req.Managers))
	add("operators", list(req.Operators))
	add("viewers", list(req.Viewers))
	add("policy", req.Policy)
	add("description", req.Description)
	spec := config.RecordSpec{Name: req.Name, Type: "TEAM", Opts: opts}
	st, err := s.app.EditConfig(authOf(r).user, req.Message, ifMatch(r, spec.Name, func(f *config.File) error { return f.UpsertRecord(spec) }))
	if err != nil {
		return nil, s.configError(err)
	}
	return s.savedRecord(st, spec.Name), nil
}

func (s *Server) teamDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	name := r.PathValue("name")
	cfg := s.app.Config().Config
	if cfg.FindTeam(name) == nil {
		return nil, notFound("team")
	}
	n := 0
	for _, m := range cfg.Monitors {
		if m.Team == name {
			n++
		}
	}
	if n > 0 {
		return nil, Errorf(http.StatusConflict, CodeConflict, "team %s still owns %d monitors: move them to another team or to the shared infrastructure first", name, n)
	}
	st, err := s.app.EditConfig(authOf(r).user, "delete team "+name, ifMatch(r, name, func(f *config.File) error { return f.DeleteRecord(name) }))
	if err != nil {
		return nil, s.configError(err)
	}
	return s.saved(st), nil
}
