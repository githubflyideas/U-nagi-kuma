package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
)

// WorkspaceSecretRepository is an admin-only plaintext credential boundary.
// ctx.UserID MUST come from trusted authentication, never imported/body identity.
// Scopes are legacy descriptive metadata, NOT authorization grants.
type WorkspaceSecretRepository struct {
	s   *Store
	ctx WorkspaceContext
	kek []byte
}

func (s *Store) WorkspaceSecrets(ctx WorkspaceContext, kek []byte) (*WorkspaceSecretRepository, error) {
	r := &WorkspaceSecretRepository{s, ctx, append([]byte(nil), kek...)}
	if e := r.transaction(func(*sql.Tx) error { return nil }); e != nil {
		return nil, e
	}
	return r, nil
}
func (r *WorkspaceSecretRepository) transaction(fn func(*sql.Tx) error) error {
	if r == nil || r.s == nil {
		return ErrTenantAccess
	}
	return r.s.workspaceTx(r.ctx, true, fn)
}
func (r *WorkspaceSecretRepository) Put(name, value string, scopes []string) error {
	return r.transaction(func(tx *sql.Tx) error {
		dek := make([]byte, 32)
		if _, e := rand.Read(dek); e != nil {
			return e
		}
		val, e := seal(dek, []byte(value))
		if e != nil {
			return e
		}
		wrapped, e := seal(r.kek, dek)
		if e != nil {
			return e
		}
		sc, e := json.Marshal(scopes)
		if e != nil {
			return e
		}
		_, e = tx.Exec(`INSERT INTO secrets(tenant_id,workspace_id,name,key_id,dek,value,scopes,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(tenant_id,workspace_id,name) DO UPDATE SET key_id=excluded.key_id,dek=excluded.dek,value=excluded.value,scopes=excluded.scopes,updated_at=excluded.updated_at`, r.ctx.TenantID, r.ctx.WorkspaceID, name, keyID(r.kek), wrapped, val, string(sc), ms(r.s.now()))
		return e
	})
}
func readWorkspaceSecret(tx *sql.Tx, tenant TenantID, workspace WorkspaceID, kek []byte, name string) (string, error) {
	var kid, dek, val string
	e := tx.QueryRow(`SELECT key_id,dek,value FROM secrets WHERE tenant_id=? AND workspace_id=? AND name=?`, tenant, workspace, name).Scan(&kid, &dek, &val)
	if errors.Is(e, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if e != nil {
		return "", e
	}
	if kid != keyID(kek) {
		return "", errors.New("secret key mismatch")
	}
	d, e := unseal(kek, dek)
	if e != nil {
		return "", errors.New("cannot unwrap secret key")
	}
	v, e := unseal(d, val)
	if e != nil {
		return "", errors.New("cannot decrypt secret")
	}
	return string(v), nil
}
func (r *WorkspaceSecretRepository) Get(name string) (out string, err error) {
	err = r.transaction(func(tx *sql.Tx) error {
		var e error
		out, e = readWorkspaceSecret(tx, r.ctx.TenantID, r.ctx.WorkspaceID, r.kek, name)
		return e
	})
	if err != nil {
		out = ""
	}
	return
}
func (r *WorkspaceSecretRepository) Delete(name string) error {
	return r.transaction(func(tx *sql.Tx) error {
		_, e := tx.Exec(`DELETE FROM secrets WHERE tenant_id=? AND workspace_id=? AND name=?`, r.ctx.TenantID, r.ctx.WorkspaceID, name)
		return e
	})
}
func (r *WorkspaceSecretRepository) List() (out []SecretInfo, err error) {
	out = []SecretInfo{}
	err = r.transaction(func(tx *sql.Tx) error {
		rows, e := tx.Query(`SELECT name,scopes,updated_at FROM secrets WHERE tenant_id=? AND workspace_id=? ORDER BY name`, r.ctx.TenantID, r.ctx.WorkspaceID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var si SecretInfo
			var sc sql.NullString
			var up int64
			if e = rows.Scan(&si.Name, &sc, &up); e != nil {
				return e
			}
			if sc.Valid {
				if e = json.Unmarshal([]byte(sc.String), &si.Scopes); e != nil {
					return e
				}
			}
			si.UpdatedAt = fromMS(up)
			out = append(out, si)
		}
		return rows.Err()
	})
	if err != nil {
		out = nil
	}
	return
}
