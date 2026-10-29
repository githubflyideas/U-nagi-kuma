package server

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/config"
	"github.com/githubflyideas/unagikuma/internal/directory"
	"github.com/githubflyideas/unagikuma/internal/store"
)

// The contact directory: who may receive alerts. Channels are checked
// against it; see the directory package.

type personJSON struct {
	store.Person
	Channels []string `json:"channels"` // channels that send to this person
}

func (s *Server) people(w http.ResponseWriter, r *http.Request) (any, error) {
	d := s.app.Directory()
	cfg := s.app.Config().Config
	used := map[int64][]string{}
	for _, ch := range cfg.Channels {
		key, kind, ok := directory.AddressKey(ch.Kind)
		if !ok {
			continue
		}
		seen := map[int64]bool{}
		for _, item := range directory.SplitList(ch.Opts[key]) {
			p := d.ByHandle(item)
			if !strings.HasPrefix(item, "@") {
				p = d.Owner(kind, item)
			}
			if p != nil && !seen[p.ID] {
				seen[p.ID] = true
				used[p.ID] = append(used[p.ID], ch.Name)
			}
		}
	}
	out := []personJSON{}
	for _, p := range d.People() {
		chans := used[p.ID]
		if chans == nil {
			chans = []string{}
		}
		sort.Strings(chans)
		out = append(out, personJSON{p, chans})
	}
	bounces, err := s.app.Store().Bounces()
	if err != nil {
		return nil, err
	}
	return map[string]any{"people": out, "bounces": bounces, "unknown": s.app.UnknownAddresses(),
		"threshold": cfg.Tunables.Int("contacts.bounce-threshold")}, nil
}

type personReq struct {
	Handle   string   `json:"handle"`
	Name     string   `json:"name"`
	Note     string   `json:"note"`
	Username string   `json:"username"`
	Emails   []string `json:"emails"`
	Phones   []string `json:"phones"`
}

func (s *Server) personSave(w http.ResponseWriter, r *http.Request) (any, error) {
	var req personReq
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	var id int64
	if v := r.PathValue("id"); v != "" {
		var err error
		if id, err = strconv.ParseInt(v, 10, 64); err != nil {
			return nil, notFound("person")
		}
	}
	if req.Username != "" {
		if _, err := s.app.Store().UserID(req.Username); err != nil {
			return nil, Errorf(http.StatusBadRequest, CodeInvalid, "no login named %s", req.Username)
		}
	}
	id, err := s.app.Store().SavePerson(store.Person{ID: id, Handle: req.Handle, Name: req.Name, Note: req.Note, Username: req.Username,
		Emails: req.Emails, Phones: req.Phones})
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, notFound("person")
	case err != nil:
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "%v", err)
	}
	auditOf(r).target = req.Handle
	s.app.DirectoryChanged()
	return map[string]any{"id": id}, nil
}

func personID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, notFound("person")
	}
	return id, nil
}

func (s *Server) personDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := personID(r)
	if err != nil {
		return nil, err
	}
	if err := s.app.Store().DeletePerson(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, notFound("person")
		}
		return nil, err
	}
	s.app.DirectoryChanged()
	return nil, nil
}

// personLeft marks someone as having left: nothing is sent to them any
// more, and their login (if linked) is disabled.
func (s *Server) personLeft(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := personID(r)
	if err != nil {
		return nil, err
	}
	var req struct {
		Left bool `json:"left"`
	}
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	switch err := s.app.Store().SetLeft(id, req.Left); {
	case errors.Is(err, store.ErrNotFound):
		return nil, notFound("person")
	case errors.Is(err, store.ErrLastAdmin):
		return nil, Errorf(http.StatusConflict, CodeConflict, "%v", err)
	case err != nil:
		return nil, err
	}
	auditOf(r).note("left", strconv.FormatBool(req.Left))
	s.app.DirectoryChanged()
	return nil, nil
}

var handleClean = regexp.MustCompile(`[^a-z0-9._-]+`)

// peopleImport adds a person for every channel address the directory
// does not know yet, named after the address: the quick way to bring an
// existing setup under the directory, then rename and merge by hand.
func (s *Server) peopleImport(w http.ResponseWriter, r *http.Request) (any, error) {
	st := s.app.Store()
	taken := map[string]bool{}
	for _, p := range s.app.Directory().People() {
		taken[p.Handle] = true
	}
	unique := func(h string) string {
		h = strings.Trim(handleClean.ReplaceAllString(strings.ToLower(h), "-"), "-._")
		if h == "" || !store.ValidHandle(h) {
			h = "contact"
		}
		base, n := h, 2
		for taken[h] {
			h = base + "-" + strconv.Itoa(n)
			n++
		}
		taken[h] = true
		return h
	}
	created := 0
	var errs []string
	unknown := s.app.UnknownAddresses()
	for _, raw := range unknown[store.ContactEmail] {
		addr := store.NormalizeEmail(raw)
		name := strings.TrimSpace(strings.Trim(strings.SplitN(raw, "<", 2)[0], `" `))
		local, _, _ := strings.Cut(addr, "@")
		if name == "" || strings.Contains(name, "@") {
			name = local
		}
		if _, err := st.SavePerson(store.Person{Handle: unique(local), Name: name, Emails: []string{addr}}); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		created++
	}
	for _, raw := range unknown[store.ContactPhone] {
		ph := store.NormalizePhone(raw)
		tail := ph
		if len(tail) > 4 {
			tail = tail[len(tail)-4:]
		}
		if _, err := st.SavePerson(store.Person{Handle: unique("phone-" + tail), Name: ph, Phones: []string{ph}}); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		created++
	}
	auditOf(r).note("created", strconv.Itoa(created))
	s.app.DirectoryChanged()
	if errs == nil {
		errs = []string{}
	}
	return map[string]any{"created": created, "errors": errs}, nil
}

// peopleLookup checks a recipient list before a channel is saved: which
// addresses the directory does not know, and what else is wrong.
func (s *Server) peopleLookup(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct {
		Kind string `json:"kind"` // channel kind
		List string `json:"list"`
	}
	if err := decodeJSON(r, s.maxBody(), &req); err != nil {
		return nil, err
	}
	_, kind, ok := directory.AddressKey(req.Kind)
	if !ok {
		return map[string]any{"unknown": []string{}, "issues": []directory.Issue{}}, nil
	}
	d := s.app.Directory()
	_, issues := d.Resolve(kind, req.List)
	unknown := d.Unknown(kind, req.List)
	if unknown == nil {
		unknown = []string{}
	}
	if issues == nil {
		issues = []directory.Issue{}
	}
	return map[string]any{"kind": kind, "unknown": unknown, "issues": issues}, nil
}

func (s *Server) bounceClear(w http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	kind, addr := q.Get("kind"), q.Get("address")
	if (kind != store.ContactEmail && kind != store.ContactPhone) || addr == "" {
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "kind (email or phone) and address are required")
	}
	if err := s.app.Store().ClearBounces(kind, addr); err != nil {
		return nil, err
	}
	s.app.DirectoryChanged()
	return nil, nil
}

// chainIssues lists the channels of a chain, group members included,
// that have directory issues.
func chainIssues(cfg *config.Config, chain []string, issues map[string][]directory.Issue) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(names []string)
	walk = func(names []string) {
		for _, n := range names {
			if seen[n] {
				continue
			}
			seen[n] = true
			if len(issues[n]) > 0 {
				out = append(out, n)
			}
			if c := cfg.FindChannel(n); c != nil {
				walk(c.Members)
			}
		}
	}
	walk(chain)
	return out
}
