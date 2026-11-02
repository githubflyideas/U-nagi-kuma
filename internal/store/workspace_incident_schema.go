package store

const workspaceIncidentMigration = `
CREATE TABLE workspace_incidents (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL, monitor_id INTEGER NOT NULL,
 state TEXT NOT NULL, updated_at INTEGER NOT NULL,
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id),
 FOREIGN KEY(monitor_id) REFERENCES monitors(id)
);
CREATE INDEX workspace_incidents_owner ON workspace_incidents(tenant_id,workspace_id,monitor_id,id);
CREATE TABLE workspace_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL, monitor_id INTEGER NOT NULL,
 incident_id INTEGER NOT NULL REFERENCES workspace_incidents(id),
 kind TEXT NOT NULL, at INTEGER NOT NULL,
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id),
 FOREIGN KEY(monitor_id) REFERENCES monitors(id)
);
CREATE INDEX workspace_events_owner ON workspace_events(tenant_id,workspace_id,id);
CREATE TABLE workspace_notification_intents (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 tenant_id TEXT NOT NULL, workspace_id TEXT NOT NULL, monitor_id INTEGER NOT NULL,
 incident_id INTEGER NOT NULL REFERENCES workspace_incidents(id),
 generation INTEGER NOT NULL, channel TEXT NOT NULL, message TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending', lease_until INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 FOREIGN KEY(tenant_id,workspace_id) REFERENCES workspaces(tenant_id,id),
 FOREIGN KEY(monitor_id) REFERENCES monitors(id)
);
CREATE INDEX workspace_notification_pending ON workspace_notification_intents(tenant_id,workspace_id,generation,state,id);
CREATE TRIGGER workspace_incident_owner_insert BEFORE INSERT ON workspace_incidents
WHEN NOT EXISTS(SELECT 1 FROM monitors WHERE id=NEW.monitor_id AND tenant_id=NEW.tenant_id AND workspace_id=NEW.workspace_id)
BEGIN SELECT RAISE(ABORT,'workspace incident ownership'); END;
CREATE TRIGGER workspace_incident_owner_immutable BEFORE UPDATE OF tenant_id,workspace_id,monitor_id ON workspace_incidents
BEGIN SELECT RAISE(ABORT,'immutable workspace incident ownership'); END;
CREATE TRIGGER workspace_event_owner_insert BEFORE INSERT ON workspace_events
WHEN NOT EXISTS(SELECT 1 FROM workspace_incidents WHERE id=NEW.incident_id AND tenant_id=NEW.tenant_id AND workspace_id=NEW.workspace_id AND monitor_id=NEW.monitor_id)
BEGIN SELECT RAISE(ABORT,'workspace event ownership'); END;
CREATE TRIGGER workspace_event_immutable BEFORE UPDATE ON workspace_events
BEGIN SELECT RAISE(ABORT,'immutable workspace event'); END;
CREATE TRIGGER workspace_notification_owner_insert BEFORE INSERT ON workspace_notification_intents
WHEN NOT EXISTS(SELECT 1 FROM workspace_incidents WHERE id=NEW.incident_id AND tenant_id=NEW.tenant_id AND workspace_id=NEW.workspace_id AND monitor_id=NEW.monitor_id)
BEGIN SELECT RAISE(ABORT,'workspace notification ownership'); END;
CREATE TRIGGER workspace_notification_owner_immutable BEFORE UPDATE OF tenant_id,workspace_id,monitor_id,incident_id,generation,channel,message ON workspace_notification_intents
BEGIN SELECT RAISE(ABORT,'immutable workspace notification ownership'); END;
`
