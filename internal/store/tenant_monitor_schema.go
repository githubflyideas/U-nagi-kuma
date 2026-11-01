package store

// Migration 16 preserves all numeric IDs, including retired identities. History
// tables remain keyed by those globally unique IDs; ownership is immutable.
const tenantMonitorMigration = `
ALTER TABLE monitors ADD COLUMN tenant_id TEXT REFERENCES tenants(id);
UPDATE monitors SET tenant_id=(SELECT id FROM tenants WHERE is_default=1);
DROP INDEX monitors_active;
CREATE UNIQUE INDEX monitors_active ON monitors(tenant_id,name) WHERE retired_at IS NULL;
CREATE INDEX monitors_tenant ON monitors(tenant_id,id);
CREATE TRIGGER monitor_owner_insert BEFORE INSERT ON monitors
WHEN NEW.tenant_id IS NULL OR NEW.tenant_id=''
BEGIN SELECT RAISE(ABORT,'monitor requires tenant'); END;
CREATE TRIGGER monitor_identity_immutable BEFORE UPDATE OF id,tenant_id ON monitors
WHEN NEW.id<>OLD.id OR NEW.tenant_id IS NOT OLD.tenant_id
BEGIN SELECT RAISE(ABORT,'monitor identity is immutable'); END;

ALTER TABLE monitor_state RENAME TO monitor_state_legacy;
CREATE TABLE monitor_state (
 tenant_id TEXT NOT NULL REFERENCES tenants(id), name TEXT NOT NULL,
 state TEXT NOT NULL, status TEXT NOT NULL, reason TEXT, detail TEXT,
 latency_ms REAL, last_check INTEGER, last_change INTEGER, incident_id INTEGER, def_hash TEXT,
 PRIMARY KEY(tenant_id,name));
INSERT INTO monitor_state SELECT (SELECT id FROM tenants WHERE is_default=1),
 name,state,status,reason,detail,latency_ms,last_check,last_change,incident_id,def_hash FROM monitor_state_legacy;
DROP TABLE monitor_state_legacy;
`
