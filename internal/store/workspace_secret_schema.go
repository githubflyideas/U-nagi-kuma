package store

// Migration 18 preserves ciphertext and default compatibility; no key is needed to upgrade.
const workspaceSecretMigration = `
ALTER TABLE secrets RENAME TO secrets_v17;
CREATE TABLE secrets (
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL, name TEXT NOT NULL,
 key_id TEXT NOT NULL, dek TEXT NOT NULL, value TEXT NOT NULL,
 scopes TEXT, updated_at INTEGER NOT NULL,
 PRIMARY KEY(tenant_id,workspace_id,name),
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id)
);
INSERT INTO secrets SELECT t.id,w.id,s.name,s.key_id,s.dek,s.value,s.scopes,s.updated_at
 FROM secrets_v17 s CROSS JOIN tenants t JOIN workspaces w ON w.tenant_id=t.id
 WHERE t.is_default=1 AND w.is_default=1;
DROP TABLE secrets_v17;
CREATE TRIGGER secret_owner_immutable BEFORE UPDATE OF tenant_id,workspace_id,name ON secrets
 WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.workspace_id IS NOT OLD.workspace_id OR NEW.name IS NOT OLD.name
 BEGIN SELECT RAISE(ABORT,'secret identity is immutable'); END;
CREATE TABLE workspace_secret_readers (
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK(typeof(generation)='integer' AND generation>0),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0,1)),
 token_hash TEXT,
 PRIMARY KEY(tenant_id,workspace_id), UNIQUE(token_hash),
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id) ON DELETE CASCADE
);
CREATE TRIGGER workspace_reader_identity BEFORE UPDATE OF tenant_id,workspace_id ON workspace_secret_readers
 WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.workspace_id IS NOT OLD.workspace_id
 BEGIN SELECT RAISE(ABORT,'service identity is immutable'); END;
CREATE TRIGGER workspace_reader_generation BEFORE UPDATE ON workspace_secret_readers
 WHEN NEW.generation <= OLD.generation
 BEGIN SELECT RAISE(ABORT,'service generation must advance'); END;
`

// Only the legacy instance adapter uses this binding. Scoped APIs never use it.
const defaultSecretOwner = `tenant_id=(SELECT id FROM tenants WHERE is_default=1) AND workspace_id=(SELECT w.id FROM workspaces w JOIN tenants t ON t.id=w.tenant_id WHERE t.is_default=1 AND w.is_default=1)`
