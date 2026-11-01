package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// RuntimeSecretReader is ONLY a secret reader. It holds no interactive UserID,
// accepts no caller-selected scope, and implements no mutations or grant API.
// Its credential is not a session/API token and must never enter user auth.
type RuntimeSecretReader struct {
	s          *Store
	tenant     TenantID
	workspace  WorkspaceID
	generation int64
	hash       string
	kek        []byte
}

// IssueWorkspaceSecretReader replaces the previous reader generation atomically.
// The returned opaque credential is disclosed once. Persist it only in trusted
// service storage; the database retains only its hash. No runtime is activated.
func (s *Store) IssueWorkspaceSecretReader(ctx WorkspaceContext) (credential string, err error) {
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		return
	}
	credential = hex.EncodeToString(token[:])
	hash := readerHash(credential)
	err = s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		result, e := tx.Exec(`INSERT INTO workspace_secret_readers(tenant_id,workspace_id,generation,active,token_hash) VALUES(?,?,1,1,?) ON CONFLICT(tenant_id,workspace_id) DO UPDATE SET generation=generation+1,token_hash=excluded.token_hash WHERE active=1`, ctx.TenantID, ctx.WorkspaceID, hash)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrTenantAccess
		}
		return nil
	})
	if err != nil {
		credential = ""
	}
	return
}
func readerHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Store) RevokeWorkspaceSecretReader(ctx WorkspaceContext) error {
	return s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		_, e := tx.Exec(`UPDATE workspace_secret_readers SET generation=generation+1,token_hash=NULL WHERE tenant_id=? AND workspace_id=?`, ctx.TenantID, ctx.WorkspaceID)
		return e
	})
}

// SetWorkspaceRuntimeActive controls this workspace's service-secret boundary,
// NOT the legacy scheduler. Any call invalidates previous credentials; enabling
// never resurrects them. Interactive secret admins can still repair credentials.
func (s *Store) SetWorkspaceRuntimeActive(ctx WorkspaceContext, active bool) error {
	return s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		_, e := tx.Exec(`INSERT INTO workspace_secret_readers(tenant_id,workspace_id,generation,active,token_hash) VALUES(?,?,1,?,NULL) ON CONFLICT(tenant_id,workspace_id) DO UPDATE SET generation=generation+1,active=excluded.active,token_hash=NULL`, ctx.TenantID, ctx.WorkspaceID, active)
		return e
	})
}
func (s *Store) RuntimeSecrets(credential string, kek []byte) (*RuntimeSecretReader, error) {
	// Reject malformed credentials without trying alternate encodings/identities.
	raw, e := hex.DecodeString(credential)
	if e != nil || len(raw) != 32 || hex.EncodeToString(raw) != credential {
		return nil, ErrTenantAccess
	}
	r := &RuntimeSecretReader{s: s, hash: readerHash(credential), kek: append([]byte(nil), kek...)}
	e = s.core.QueryRow(`SELECT r.tenant_id,r.workspace_id,r.generation FROM workspace_secret_readers r JOIN workspaces w ON w.tenant_id=r.tenant_id AND w.id=r.workspace_id WHERE r.token_hash=? AND r.active=1`, r.hash).Scan(&r.tenant, &r.workspace, &r.generation)
	if e != nil {
		return nil, tenantAccessError(e)
	}
	return r, nil
}
func (r *RuntimeSecretReader) Get(name string) (string, error) {
	return r.read(func(tx *sql.Tx) (string, error) { return readWorkspaceSecret(tx, r.tenant, r.workspace, r.kek, name) })
}

// Resolve expands only ${secret:NAME}. Environment, malformed and unknown
// placeholders fail closed. A single authorized snapshot covers every reference;
// an error never returns a partially resolved string or any plaintext.
func (r *RuntimeSecretReader) Resolve(input string) (string, error) {
	return r.read(func(tx *sql.Tx) (string, error) {
		return resolveWorkspaceReferences(input, func(name string) (string, error) {
			return readWorkspaceSecret(tx, r.tenant, r.workspace, r.kek, name)
		})
	})
}

var ErrSecretReference = errors.New("invalid or unsupported scoped secret reference")

func (r *RuntimeSecretReader) read(fn func(*sql.Tx) (string, error)) (out string, err error) {
	if r == nil || r.s == nil {
		return "", ErrTenantAccess
	}
	r.s.wmu.Lock()
	defer r.s.wmu.Unlock()
	tx, e := r.s.core.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var valid int
	e = tx.QueryRow(`SELECT 1 FROM workspace_secret_readers r JOIN workspaces w ON w.tenant_id=r.tenant_id AND w.id=r.workspace_id WHERE r.tenant_id=? AND r.workspace_id=? AND r.generation=? AND r.token_hash=? AND r.active=1`, r.tenant, r.workspace, r.generation, r.hash).Scan(&valid)
	if e != nil {
		return "", tenantAccessError(e)
	}
	out, e = fn(tx)
	if e != nil {
		return "", e
	}
	if e = tx.Commit(); e != nil {
		return "", e
	}
	return out, nil
}
