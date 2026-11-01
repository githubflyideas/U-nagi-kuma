package store

// Migration 17: company names remain unique; history's stable numeric IDs are untouched.
const workspaceMigration = `
CREATE TABLE workspaces (
 id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=32 AND id NOT GLOB '*[^0-9a-f]*'),
 tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 name TEXT NOT NULL CHECK(length(trim(name))>0),
 is_default INTEGER NOT NULL DEFAULT 0 CHECK(is_default IN (0,1)),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,name)
);
CREATE UNIQUE INDEX workspace_default ON workspaces(tenant_id) WHERE is_default=1;
INSERT INTO workspaces(id,tenant_id,name,is_default) SELECT lower(hex(randomblob(16))),id,'Default',1 FROM tenants;
CREATE TRIGGER workspace_tenant_insert AFTER INSERT ON tenants BEGIN
 INSERT INTO workspaces(id,tenant_id,name,is_default) VALUES(lower(hex(randomblob(16))),NEW.id,'Default',1);
END;
CREATE TRIGGER workspace_identity_immutable BEFORE UPDATE OF id,tenant_id,is_default ON workspaces
 WHEN NEW.id IS NOT OLD.id OR NEW.tenant_id IS NOT OLD.tenant_id OR NEW.is_default IS NOT OLD.is_default
 BEGIN SELECT RAISE(ABORT,'workspace identity is immutable'); END;
CREATE TRIGGER workspace_default_preserved BEFORE DELETE ON workspaces WHEN OLD.is_default=1
 BEGIN SELECT RAISE(ABORT,'default workspace cannot be deleted'); END;
CREATE TABLE workspace_members (
 tenant_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL,
 user_id INTEGER NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
 disabled INTEGER NOT NULL DEFAULT 0 CHECK(disabled IN (0,1)),
 PRIMARY KEY(workspace_id,user_id),
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,user_id) REFERENCES tenant_members(tenant_id,user_id) ON DELETE CASCADE
);
INSERT INTO workspace_members(tenant_id,workspace_id,user_id,role)
 SELECT m.tenant_id,w.id,m.user_id,m.role FROM tenant_members m JOIN workspaces w ON w.tenant_id=m.tenant_id AND w.is_default=1;
CREATE TRIGGER workspace_member_insert AFTER INSERT ON tenant_members BEGIN
 INSERT INTO workspace_members(tenant_id,workspace_id,user_id,role)
 SELECT NEW.tenant_id,id,NEW.user_id,NEW.role FROM workspaces WHERE tenant_id=NEW.tenant_id AND is_default=1;
END;
CREATE TRIGGER workspace_member_role AFTER UPDATE OF role ON tenant_members BEGIN
 UPDATE workspace_members SET role=NEW.role WHERE tenant_id=NEW.tenant_id AND user_id=NEW.user_id
 AND workspace_id IN (SELECT id FROM workspaces WHERE tenant_id=NEW.tenant_id AND is_default=1);
END;
CREATE TRIGGER workspace_member_identity BEFORE UPDATE OF tenant_id,workspace_id,user_id ON workspace_members
 WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.workspace_id IS NOT OLD.workspace_id OR NEW.user_id IS NOT OLD.user_id
 BEGIN SELECT RAISE(ABORT,'membership identity is immutable'); END;
ALTER TABLE monitors ADD COLUMN workspace_id TEXT REFERENCES workspaces(id);
UPDATE monitors SET workspace_id=(SELECT id FROM workspaces WHERE tenant_id=monitors.tenant_id AND is_default=1);
CREATE INDEX monitors_workspace ON monitors(tenant_id,workspace_id,id);
-- Only trusted legacy SQL omits workspace ownership; repository contexts never do.
CREATE TRIGGER monitor_workspace_default AFTER INSERT ON monitors WHEN NEW.workspace_id IS NULL BEGIN
 UPDATE monitors SET workspace_id=(SELECT id FROM workspaces WHERE tenant_id=NEW.tenant_id AND is_default=1) WHERE id=NEW.id;
END;
CREATE TRIGGER monitor_workspace_valid BEFORE INSERT ON monitors WHEN NEW.workspace_id IS NOT NULL AND NOT EXISTS
 (SELECT 1 FROM workspaces WHERE id=NEW.workspace_id AND tenant_id=NEW.tenant_id)
 BEGIN SELECT RAISE(ABORT,'invalid workspace owner'); END;
CREATE TRIGGER monitor_workspace_immutable BEFORE UPDATE OF workspace_id ON monitors
 WHEN (OLD.workspace_id IS NOT NULL AND NEW.workspace_id IS NOT OLD.workspace_id) OR NOT EXISTS
 (SELECT 1 FROM workspaces WHERE id=NEW.workspace_id AND tenant_id=NEW.tenant_id)
 BEGIN SELECT RAISE(ABORT,'monitor workspace is immutable'); END;
ALTER TABLE monitor_state RENAME TO monitor_state_v16;
CREATE TABLE monitor_state (
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL, name TEXT NOT NULL,
 state TEXT NOT NULL, status TEXT NOT NULL, reason TEXT, detail TEXT,
 latency_ms REAL, last_check INTEGER, last_change INTEGER, incident_id INTEGER, def_hash TEXT,
 PRIMARY KEY(tenant_id,workspace_id,name),
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id)
);
INSERT INTO monitor_state SELECT tenant_id,(SELECT id FROM workspaces WHERE tenant_id=monitor_state_v16.tenant_id AND is_default=1),
 name,state,status,reason,detail,latency_ms,last_check,last_change,incident_id,def_hash FROM monitor_state_v16;
DROP TABLE monitor_state_v16;
CREATE TRIGGER state_workspace_valid BEFORE INSERT ON monitor_state WHEN NEW.workspace_id IS NOT NULL AND NOT EXISTS
 (SELECT 1 FROM workspaces WHERE id=NEW.workspace_id AND tenant_id=NEW.tenant_id)
 BEGIN SELECT RAISE(ABORT,'invalid state workspace'); END;
CREATE TRIGGER state_workspace_immutable BEFORE UPDATE OF workspace_id,tenant_id ON monitor_state
 WHEN NEW.tenant_id IS NOT OLD.tenant_id OR (OLD.workspace_id IS NOT NULL AND NEW.workspace_id IS NOT OLD.workspace_id) OR NOT EXISTS
 (SELECT 1 FROM workspaces WHERE id=NEW.workspace_id AND tenant_id=NEW.tenant_id)
 BEGIN SELECT RAISE(ABORT,'state workspace is immutable'); END;
`
