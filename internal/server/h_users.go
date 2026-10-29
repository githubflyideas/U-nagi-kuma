package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/githubflyideas/unagikuma/internal/store"
)

type userReq struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Role     string   `json:"role"`
	Areas    []string `json:"areas"`
}

// cleanAreas keeps sections only for operators: other roles see or change
// everything they are allowed to.
func (q *userReq) cleanAreas() []string {
	if q.Role != store.RoleOperator {
		return nil
	}
	var out []string
	for _, a := range q.Areas {
		if a = strings.TrimSpace(a); a != "" && a != "*" {
			out = append(out, a)
		}
	}
	return out
}

func validUsername(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func userError(err error) error {
	switch {
	case errors.Is(err, store.ErrUserExists):
		return Errorf(http.StatusConflict, CodeConflict, "%v", err)
	case errors.Is(err, store.ErrLastAdmin):
		return Errorf(http.StatusConflict, CodeConflict, "%v", err)
	case errors.Is(err, store.ErrNotFound):
		return notFound("user")
	}
	return Errorf(http.StatusBadRequest, CodeInvalid, "%v", err)
}

func (s *Server) usersList(w http.ResponseWriter, r *http.Request) (any, error) {
	list, err := s.app.Store().Users()
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": list}, nil
}

func (s *Server) userCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	var q userReq
	if err := decodeJSON(r, s.maxBody(), &q); err != nil {
		return nil, err
	}
	if !validUsername(q.Username) {
		return nil, Errorf(http.StatusBadRequest, CodeInvalid, "invalid username")
	}
	if err := s.app.AllowAdd("users"); err != nil {
		return nil, quotaError(err)
	}
	id, err := s.app.Store().CreateUserRole(q.Username, q.Password, q.Role, q.cleanAreas())
	if err != nil {
		return nil, userError(err)
	}
	s.log.Info("user created", "user", q.Username, "role", q.Role, "by", authOf(r).user)
	auditOf(r).note("role", q.Role).note("areas", q.cleanAreas()).target = q.Username
	return map[string]any{"id": id}, nil
}

func (s *Server) userUpdate(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, notFound("user")
	}
	var q userReq
	if err := decodeJSON(r, s.maxBody(), &q); err != nil {
		return nil, err
	}
	st := s.app.Store()
	before, _ := s.userByID(id)
	a := auditOf(r)
	a.target = before.Username
	if q.Role != "" {
		a.note("before", map[string]any{"role": before.Role, "areas": before.Areas}).note("after", map[string]any{"role": q.Role, "areas": q.cleanAreas()})
		if err := st.SetRole(id, q.Role, q.cleanAreas()); err != nil {
			return nil, userError(err)
		}
	}
	if q.Password != "" {
		if err := st.SetPassword(id, q.Password); err != nil {
			return nil, userError(err)
		}
	}
	if q.Password != "" {
		a.note("password", "reset")
	}
	s.log.Info("user changed", "id", id, "role", q.Role, "password", q.Password != "", "by", authOf(r).user)
	return map[string]any{"ok": true}, nil
}

func (s *Server) userDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, notFound("user")
	}
	if id == authOf(r).userID {
		return nil, Errorf(http.StatusConflict, CodeConflict, "you cannot delete your own account")
	}
	before, _ := s.userByID(id)
	if err := s.app.Store().DeleteUser(id); err != nil {
		return nil, userError(err)
	}
	auditOf(r).note("role", before.Role).note("areas", before.Areas).target = before.Username
	return map[string]any{"ok": true}, nil
}

// userByID finds an account for the audit log ("" when unknown).
func (s *Server) userByID(id int64) (store.User, bool) {
	list, _ := s.app.Store().Users()
	for _, u := range list {
		if u.ID == id {
			return u, true
		}
	}
	return store.User{Username: strconv.FormatInt(id, 10)}, false
}
