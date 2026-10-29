package store

import (
	"encoding/json"
	"strings"
	"time"
)

// AuditEntry is one change made by a person or a token: who, what, to
// what, in which configuration section, and from where.
type AuditEntry struct {
	ID     int64          `json:"id"`
	At     time.Time      `json:"at"`
	Actor  string         `json:"actor"`
	Action string         `json:"action"` // e.g. user.update, config.save, secret.put
	Target string         `json:"target,omitempty"`
	Area   string         `json:"area,omitempty"`
	Detail map[string]any `json:"detail,omitempty"`
	Addr   string         `json:"addr,omitempty"`
}

// Audit records a change. The log is append-only through the API.
func (s *Store) Audit(e AuditEntry) error {
	if e.At.IsZero() {
		e.At = s.now()
	}
	detail := ""
	if len(e.Detail) > 0 {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return err
		}
		detail = string(b)
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, err := s.core.Exec(`INSERT INTO audit(at,actor,action,target,area,detail,addr) VALUES(?,?,?,?,?,?,?)`,
		ms(e.At), e.Actor, e.Action, e.Target, e.Area, detail, e.Addr)
	return err
}

// AuditQuery filters the audit log; empty fields match everything.
type AuditQuery struct {
	Actor, Action, Target, Area string
	From, To                    time.Time
	Before                      int64 // page: entries with a smaller id
	Limit                       int
}

// AuditLog returns matching entries, newest first.
func (s *Store) AuditLog(q AuditQuery) ([]AuditEntry, error) {
	var where []string
	var args []any
	if q.Actor != "" {
		where, args = append(where, "actor=?"), append(args, q.Actor)
	}
	if q.Action != "" {
		// "user" matches user.create, user.update...
		where, args = append(where, "(action=? OR action LIKE ?)"), append(args, q.Action, q.Action+".%")
	}
	if q.Target != "" {
		where, args = append(where, "target=?"), append(args, q.Target)
	}
	if q.Area != "" {
		where, args = append(where, "area=?"), append(args, q.Area)
	}
	if !q.From.IsZero() {
		where, args = append(where, "at>=?"), append(args, ms(q.From))
	}
	if !q.To.IsZero() {
		where, args = append(where, "at<?"), append(args, ms(q.To))
	}
	if q.Before > 0 {
		where, args = append(where, "id<?"), append(args, q.Before)
	}
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 200
	}
	sql := `SELECT id,at,actor,action,target,area,detail,addr FROM audit`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY id DESC LIMIT ?"
	args = append(args, q.Limit)
	rows, err := s.core.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var at int64
		var detail string
		if err := rows.Scan(&e.ID, &at, &e.Actor, &e.Action, &e.Target, &e.Area, &detail, &e.Addr); err != nil {
			return nil, err
		}
		e.At = fromMS(at)
		if detail != "" {
			_ = json.Unmarshal([]byte(detail), &e.Detail)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
