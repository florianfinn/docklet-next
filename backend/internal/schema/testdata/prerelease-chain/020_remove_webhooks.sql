-- Source: docklet-next/issues/358
-- Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
-- Retrieved: 2026-11-07
-- License: Apache-2.0
-- Deviations: Original M20 removal and refusal contracts.

-- +goose Up
DROP TABLE webhooks;
DELETE FROM api_key_permissions WHERE permission LIKE 'webhooks:%';
UPDATE roles SET permissions = (SELECT COALESCE(json_group_array(value), '[]') FROM json_each(roles.permissions) WHERE value NOT LIKE 'webhooks:%');
DELETE FROM events WHERE type LIKE 'webhook.%';
CREATE TABLE "agent_restart_requests_m20" ("run_id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"registration_id" TEXT NOT NULL,"credential_revision" INTEGER NOT NULL,"old_instance" TEXT NOT NULL,"state" TEXT NOT NULL,"created_at" datetime NOT NULL,"updated_at" datetime NOT NULL,"sent_at" datetime,"acknowledged_at" datetime,"code" TEXT,"deadline" datetime NOT NULL,"settled_at" datetime,PRIMARY KEY ("run_id"),CHECK (state IN ('reserved','sent','awaiting','connected','failed','unknown')),CHECK ((state IN ('connected','failed','unknown')) = (settled_at IS NOT NULL)),CHECK ((state IN ('failed','unknown')) = (code IS NOT NULL)),CHECK (state <> 'unknown' OR code = 'agent.restart-unconfirmed'),CHECK (state <> 'failed' OR code IN ('agent.restart-superseded','agent.restart-timeout','agent.restart-unsupported','agent.restart-interrupted','host.unreachable','host.busy')),CHECK (acknowledged_at IS NULL OR sent_at IS NOT NULL));
INSERT INTO agent_restart_requests_m20 (run_id,environment_id,registration_id,credential_revision,old_instance,state,created_at,updated_at,sent_at,acknowledged_at,code,deadline,settled_at) SELECT run_id,environment_id,registration_id,credential_revision,old_instance,state,created_at,updated_at,sent_at,acknowledged_at,code,deadline,settled_at FROM agent_restart_requests;
DROP TABLE agent_restart_requests;
ALTER TABLE agent_restart_requests_m20 RENAME TO agent_restart_requests;
CREATE UNIQUE INDEX idx_agent_restart_requests_active ON agent_restart_requests (environment_id) WHERE settled_at IS NULL;
