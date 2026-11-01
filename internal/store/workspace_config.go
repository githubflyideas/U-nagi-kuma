package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// WorkspaceConfigState is safe metadata, not a configuration or service credential.
// Revision zero means no applied configuration. Generation fences service readers.
type WorkspaceConfigState struct {
	TenantID    TenantID    `json:"tenant_id"`
	WorkspaceID WorkspaceID `json:"workspace_id"`
	Revision    int64       `json:"revision"`
	Generation  int64       `json:"generation"`
}

var ErrWorkspaceConfigConflict = errors.New("workspace configuration revision conflict")
var ErrWorkspaceConfigInvalid = errors.New("invalid workspace configuration")

// WorkspaceConfigValidator runs inside an immediate transaction. It must not call
// Store methods, perform I/O, retain resolve, or expose resolved values in errors.
type WorkspaceConfigValidator func(body []byte, resolve func(string) (string, error)) error

const workspaceConfigMigration = `
CREATE TABLE workspace_applied_configs (
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>0),
 generation INTEGER NOT NULL CHECK(typeof(generation)='integer' AND generation>0),
 key_id TEXT NOT NULL, dek TEXT NOT NULL, value TEXT NOT NULL,
 PRIMARY KEY(tenant_id,workspace_id),
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id)
);
CREATE TRIGGER workspace_config_identity BEFORE UPDATE OF tenant_id,workspace_id ON workspace_applied_configs
 WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.workspace_id IS NOT OLD.workspace_id
 BEGIN SELECT RAISE(ABORT,'configuration ownership is immutable'); END;
CREATE TRIGGER workspace_config_revision BEFORE UPDATE OF revision,generation ON workspace_applied_configs
 WHEN NEW.revision <= OLD.revision OR NEW.generation <= OLD.generation
 BEGIN SELECT RAISE(ABORT,'configuration revision and generation must advance'); END;
`

// This entire payload is encrypted; binding fields prevent ciphertext substitution
// across scopes or revisions. Never serialize it into user configuration or logs.
type workspaceConfigPayload struct {
	State      WorkspaceConfigState
	Body       []byte
	Credential string
}

func workspaceConfigState(tx *sql.Tx, tenant TenantID, workspace WorkspaceID) (WorkspaceConfigState, error) {
	state := WorkspaceConfigState{TenantID: tenant, WorkspaceID: workspace}
	err := tx.QueryRow(`SELECT revision,generation FROM workspace_applied_configs WHERE tenant_id=? AND workspace_id=?`, tenant, workspace).Scan(&state.Revision, &state.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return state, err
}

func (s *Store) WorkspaceConfigState(ctx WorkspaceContext) (out WorkspaceConfigState, err error) {
	err = s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		var e error
		out, e = workspaceConfigState(tx, ctx.TenantID, ctx.WorkspaceID)
		return e
	})
	if err != nil {
		out = WorkspaceConfigState{}
	}
	return
}

func (x *Secrets) PreviewWorkspaceConfig(ctx WorkspaceContext, body []byte, expected int64, validate WorkspaceConfigValidator) (WorkspaceConfigState, error) {
	return x.stageWorkspaceConfig(ctx, body, expected, validate, false)
}
func (x *Secrets) ApplyWorkspaceConfig(ctx WorkspaceContext, body []byte, expected int64, validate WorkspaceConfigValidator) (WorkspaceConfigState, error) {
	return x.stageWorkspaceConfig(ctx, body, expected, validate, true)
}
func (x *Secrets) stageWorkspaceConfig(ctx WorkspaceContext, body []byte, expected int64, validate WorkspaceConfigValidator, publish bool) (out WorkspaceConfigState, err error) {
	if x == nil || x.s == nil {
		return out, ErrTenantAccess
	}
	// Own the bytes before validation and publication; never persist resolved text.
	body = append([]byte(nil), body...)
	err = x.s.workspaceTx(ctx, true, func(tx *sql.Tx) error {
		var e error
		out, e = workspaceConfigState(tx, ctx.TenantID, ctx.WorkspaceID)
		if e != nil {
			return e
		}
		if expected < 0 || expected != out.Revision {
			return ErrWorkspaceConfigConflict
		}
		var active int
		e = tx.QueryRow(`SELECT active FROM workspace_secret_readers WHERE tenant_id=? AND workspace_id=?`, ctx.TenantID, ctx.WorkspaceID).Scan(&active)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e == nil && active != 1 {
			return ErrTenantAccess
		}
		if validate == nil || len(body) == 0 || len(body) > 1<<20 || len(x.kek) != 32 {
			return ErrWorkspaceConfigInvalid
		}
		resolve := func(input string) (string, error) {
			return resolveWorkspaceReferences(input, func(name string) (string, error) {
				return readWorkspaceSecret(tx, ctx.TenantID, ctx.WorkspaceID, x.kek, name)
			})
		}
		// Check even references in ignored comments/options; never permit env fallback.
		if _, e = resolve(string(body)); e != nil {
			return e
		}
		if e = validate(append([]byte(nil), body...), resolve); e != nil {
			return e
		}
		if !publish {
			return nil
		}
		var token [32]byte
		if _, e = rand.Read(token[:]); e != nil {
			return e
		}
		credential := hex.EncodeToString(token[:])
		_, e = tx.Exec(`INSERT INTO workspace_secret_readers(tenant_id,workspace_id,generation,active,token_hash) VALUES(?,?,1,1,?) ON CONFLICT(tenant_id,workspace_id) DO UPDATE SET generation=generation+1,token_hash=excluded.token_hash WHERE active=1`, ctx.TenantID, ctx.WorkspaceID, readerHash(credential))
		if e != nil {
			return e
		}
		if e = tx.QueryRow(`SELECT generation FROM workspace_secret_readers WHERE tenant_id=? AND workspace_id=? AND active=1`, ctx.TenantID, ctx.WorkspaceID).Scan(&out.Generation); e != nil {
			return e
		}
		out.Revision++
		payload, e := json.Marshal(workspaceConfigPayload{out, body, credential})
		if e != nil {
			return e
		}
		dek := make([]byte, 32)
		if _, e = rand.Read(dek); e != nil {
			return e
		}
		value, e := seal(dek, payload)
		if e != nil {
			return e
		}
		wrapped, e := seal(x.kek, dek)
		if e != nil {
			return e
		}
		_, e = tx.Exec(`INSERT INTO workspace_applied_configs(tenant_id,workspace_id,revision,generation,key_id,dek,value) VALUES(?,?,?,?,?,?,?) ON CONFLICT(tenant_id,workspace_id) DO UPDATE SET revision=excluded.revision,generation=excluded.generation,key_id=excluded.key_id,dek=excluded.dek,value=excluded.value`, ctx.TenantID, ctx.WorkspaceID, out.Revision, out.Generation, keyID(x.kek), wrapped, value)
		return e
	})
	if err != nil {
		out = WorkspaceConfigState{}
	}
	return
}

// RestoreWorkspaceConfig is trusted installation-supervisor access, NOT a user
// endpoint. It requires the exact persisted scope/generation. No UserID survives
// issuance. Validation and live capability checks share the decryption transaction.
// Returned bytes are the original (unresolved) config, never the capability.
func (x *Secrets) RestoreWorkspaceConfig(tenant TenantID, workspace WorkspaceID, generation int64, validate WorkspaceConfigValidator) (body []byte, reader *RuntimeSecretReader, err error) {
	if x == nil || x.s == nil || tenant == "" || workspace == "" || generation <= 0 {
		return nil, nil, ErrTenantAccess
	}
	x.s.wmu.Lock()
	defer x.s.wmu.Unlock()
	tx, e := x.s.core.Begin()
	if e != nil {
		return nil, nil, e
	}
	defer tx.Rollback()
	var revision int64
	var kid, wrapped, value, hash string
	e = tx.QueryRow(`SELECT c.revision,c.key_id,c.dek,c.value,r.token_hash FROM workspace_applied_configs c JOIN workspace_secret_readers r ON r.tenant_id=c.tenant_id AND r.workspace_id=c.workspace_id AND r.generation=c.generation JOIN workspaces w ON w.tenant_id=c.tenant_id AND w.id=c.workspace_id WHERE c.tenant_id=? AND c.workspace_id=? AND c.generation=? AND r.active=1 AND r.token_hash IS NOT NULL`, tenant, workspace, generation).Scan(&revision, &kid, &wrapped, &value, &hash)
	if e != nil {
		return nil, nil, tenantAccessError(e)
	}
	if kid != keyID(x.kek) {
		return nil, nil, ErrTenantAccess
	}
	dek, e := unseal(x.kek, wrapped)
	if e != nil {
		return nil, nil, ErrTenantAccess
	}
	raw, e := unseal(dek, value)
	if e != nil {
		return nil, nil, ErrTenantAccess
	}
	var payload workspaceConfigPayload
	if json.Unmarshal(raw, &payload) != nil || payload.State != (WorkspaceConfigState{tenant, workspace, revision, generation}) || readerHash(payload.Credential) != hash {
		return nil, nil, ErrTenantAccess
	}
	if validate == nil {
		return nil, nil, ErrWorkspaceConfigInvalid
	}
	resolve := func(input string) (string, error) {
		return resolveWorkspaceReferences(input, func(name string) (string, error) { return readWorkspaceSecret(tx, tenant, workspace, x.kek, name) })
	}
	if _, e = resolve(string(payload.Body)); e != nil {
		return nil, nil, e
	}
	if e = validate(append([]byte(nil), payload.Body...), resolve); e != nil {
		return nil, nil, e
	}
	reader = &RuntimeSecretReader{s: x.s, tenant: tenant, workspace: workspace, generation: generation, hash: hash, kek: append([]byte(nil), x.kek...)}
	if e = tx.Commit(); e != nil {
		return nil, nil, e
	}
	return payload.Body, reader, nil
}

func rewrapWorkspaceConfigs(tx *sql.Tx, oldKey, newKey []byte) error {
	rows, err := tx.Query(`SELECT tenant_id,workspace_id,dek FROM workspace_applied_configs`)
	if err != nil {
		return err
	}
	type envelope struct {
		tenant    TenantID
		workspace WorkspaceID
		dek       string
	}
	var all []envelope
	for rows.Next() {
		var v envelope
		if err = rows.Scan(&v.tenant, &v.workspace, &v.dek); err != nil {
			rows.Close()
			return err
		}
		all = append(all, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range all {
		key, e := unseal(oldKey, v.dek)
		if e != nil {
			return errors.New("cannot unwrap workspace configuration key")
		}
		wrapped, e := seal(newKey, key)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE workspace_applied_configs SET key_id=?,dek=? WHERE tenant_id=? AND workspace_id=?`, keyID(newKey), wrapped, v.tenant, v.workspace); e != nil {
			return e
		}
	}
	return nil
}

func resolveWorkspaceReferences(input string, get func(string) (string, error)) (string, error) {
	var out strings.Builder
	for {
		start := strings.Index(input, "${")
		if start < 0 {
			out.WriteString(input)
			return out.String(), nil
		}
		out.WriteString(input[:start])
		input = input[start+2:]
		end := strings.IndexByte(input, '}')
		if end < 0 {
			return "", ErrSecretReference
		}
		ref := input[:end]
		input = input[end+1:]
		if !strings.HasPrefix(ref, "secret:") {
			return "", ErrSecretReference
		}
		name := strings.TrimPrefix(ref, "secret:")
		if name == "" || strings.IndexFunc(name, func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-')
		}) >= 0 {
			return "", ErrSecretReference
		}
		value, e := get(name)
		if e != nil {
			return "", e
		}
		out.WriteString(value)
	}
}
