package store

// Applied atomically with its schema version by migrateDB. IDs are opaque,
// persisted random values, never names or target addresses.
const tenantFoundationMigration = `
CREATE TABLE tenants (
 id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=32 AND id NOT GLOB '*[^0-9a-f]*'),
 name TEXT NOT NULL CHECK(length(trim(name))>0),
 is_default INTEGER NOT NULL DEFAULT 0 CHECK(is_default IN (0,1))
);
CREATE UNIQUE INDEX tenants_one_default ON tenants(is_default) WHERE is_default=1;
INSERT INTO tenants(id,name,is_default) VALUES(lower(hex(randomblob(16))),'Default',1);
CREATE TABLE tenant_members (
 tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
 disabled INTEGER NOT NULL CHECK(disabled IN (0,1)),
 PRIMARY KEY(tenant_id,user_id)
);
CREATE TABLE network_scopes (
 id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=32 AND id NOT GLOB '*[^0-9a-f]*'),
 tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 label TEXT NOT NULL CHECK(length(trim(label))>0),
 UNIQUE(tenant_id,label)
);
CREATE TRIGGER tenants_identity_immutable BEFORE UPDATE OF id,is_default ON tenants
 WHEN NEW.id != OLD.id OR NEW.is_default != OLD.is_default
 BEGIN SELECT RAISE(ABORT,'tenant identity is immutable'); END;
CREATE TRIGGER tenants_default_preserved BEFORE DELETE ON tenants WHEN OLD.is_default=1
 BEGIN SELECT RAISE(ABORT,'default tenant cannot be deleted'); END;
CREATE TRIGGER network_scope_identity_immutable BEFORE UPDATE OF id,tenant_id ON network_scopes
 WHEN NEW.id != OLD.id OR NEW.tenant_id != OLD.tenant_id
 BEGIN SELECT RAISE(ABORT,'network scope identity is immutable'); END;
INSERT INTO tenant_members(tenant_id,user_id,role,disabled)
 SELECT t.id,u.id,u.role,CASE WHEN u.disabled=0 THEN 0 ELSE 1 END FROM tenants t CROSS JOIN users u WHERE t.is_default=1;
-- All legacy user creation paths deliberately inherit their exact global role.
-- Triggers keep membership changes in the same SQLite statement/transaction.
CREATE TRIGGER tenant_user_insert AFTER INSERT ON users BEGIN
 INSERT INTO tenant_members(tenant_id,user_id,role,disabled)
 SELECT id,NEW.id,NEW.role,CASE WHEN NEW.disabled=0 THEN 0 ELSE 1 END FROM tenants WHERE is_default=1;
END;
CREATE TRIGGER tenant_user_role AFTER UPDATE OF role ON users BEGIN
 UPDATE tenant_members SET role=NEW.role WHERE user_id=NEW.id AND tenant_id IN (SELECT id FROM tenants WHERE is_default=1);
 -- Non-default grants can only be lowered by a global role change, never raised.
 UPDATE tenant_members SET role=CASE WHEN NEW.role='viewer' THEN 'viewer'
 WHEN NEW.role='operator' AND role='admin' THEN 'operator' ELSE role END
 WHERE user_id=NEW.id AND tenant_id IN (SELECT id FROM tenants WHERE is_default=0);
END;
CREATE TRIGGER tenant_user_disabled AFTER UPDATE OF disabled ON users BEGIN
 UPDATE tenant_members SET disabled=CASE WHEN NEW.disabled=0 THEN 0 ELSE 1 END WHERE user_id=NEW.id;
END;
`
