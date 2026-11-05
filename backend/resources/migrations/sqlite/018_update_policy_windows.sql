-- Source: docklet-next/issues/341
-- Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
-- Retrieved: 2026-11-04
-- License: Apache-2.0
-- Deviations: Original M18 policy, schedule, recovery-slot and contract foundation.

-- +goose Up
CREATE TABLE "update_policies" ("id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"registration_id" TEXT NOT NULL,"target_kind" TEXT NOT NULL DEFAULT 'stack',"target_id" TEXT NOT NULL,"target_identity" TEXT NOT NULL,"mode" TEXT NOT NULL,"window_kind" TEXT,"window_day" INTEGER,"window_time" TEXT,"revision" INTEGER NOT NULL,"actor_id" TEXT NOT NULL,"actor_key_id" TEXT NOT NULL,"checked_at" datetime,"code" TEXT NOT NULL,"apply_checked_at" datetime,"apply_code" TEXT NOT NULL,"bookkeeping_at" datetime,"bookkeeping_code" TEXT NOT NULL,"candidates" TEXT NOT NULL,"missing_since" datetime,PRIMARY KEY ("id"),CHECK (target_kind IN ('stack','container')),CHECK (mode IN ('check','scheduled')),CHECK (revision > 0),CHECK ((mode = 'check' AND window_kind IS NULL AND window_day IS NULL AND window_time IS NULL) OR (mode = 'scheduled' AND window_kind IS NOT NULL AND window_kind IN ('daily','weekly') AND window_time IS NOT NULL AND window_time GLOB '[0-2][0-9]:[0-5][0-9]' AND substr(window_time,1,2) <= '23' AND ((window_kind = 'daily' AND window_day IS NULL) OR (window_kind = 'weekly' AND window_day IS NOT NULL AND window_day IN (0,1,2,3,4,5,6))))));
CREATE INDEX `idx_update_policies_environment_id` ON `update_policies`(`environment_id`);
CREATE UNIQUE INDEX `idx_update_policy_target` ON `update_policies`(`environment_id`,`target_kind`,`target_id`);

-- Derive windows only from the supported six-field numeric daily/weekly forms.
WITH RECURSIVE parts(id, n, word, rest) AS (
  SELECT id, 0, '', trim(auto_update_interval) || ' ' FROM image_update_schedules
  UNION ALL
  SELECT id, n + 1, substr(rest,1,instr(rest,' ') - 1), ltrim(substr(rest,instr(rest,' ') + 1))
  FROM parts WHERE rest <> '' AND n < 7
), windows AS (
  SELECT id, max(n) AS count,
    max(CASE WHEN n=1 THEN word END) AS second,
    max(CASE WHEN n=2 THEN word END) AS minute,
    max(CASE WHEN n=3 THEN word END) AS hour,
    max(CASE WHEN n=4 THEN word END) AS dom,
    max(CASE WHEN n=5 THEN word END) AS month,
    max(CASE WHEN n=6 THEN word END) AS dow
  FROM parts GROUP BY id
), mapped AS (
  SELECT p.*, s.auto_update_interval,
    (p.mode='apply' AND lower(coalesce((SELECT value FROM settings WHERE key='pollingEnabled'),'true')) NOT IN ('false','f','0')
      AND lower(coalesce((SELECT value FROM settings WHERE key='autoUpdate'),'false')) IN ('true','t','1')
      AND s.auto_update_enabled=1 AND s.actor_id<>''
      AND (p.environment_id='0' OR EXISTS (SELECT 1 FROM environments e WHERE e.id=p.environment_id AND e.enabled=1))) AS effective,
    (w.count=6 AND w.second='0' AND w.minute<>'' AND w.minute NOT GLOB '*[^0-9]*' AND CAST(w.minute AS INTEGER) BETWEEN 0 AND 59
      AND w.hour<>'' AND w.hour NOT GLOB '*[^0-9]*' AND CAST(w.hour AS INTEGER) BETWEEN 0 AND 23
      AND w.dom='*' AND w.month='*' AND (w.dow='*' OR w.dow GLOB '[0-6]')) AS supported,
    w.minute, w.hour, w.dow
  FROM stack_update_policies p LEFT JOIN image_update_schedules s ON s.environment_id=p.environment_id AND s.registration_id=p.registration_id
  LEFT JOIN windows w ON w.id=s.id
)
INSERT INTO update_policies (id,environment_id,registration_id,target_kind,target_id,target_identity,mode,window_kind,window_day,window_time,revision,actor_id,actor_key_id,checked_at,code,apply_checked_at,apply_code,bookkeeping_at,bookkeeping_code,candidates)
SELECT id,environment_id,registration_id,'stack',target_id,target_identity,
  CASE WHEN effective THEN 'scheduled' ELSE 'check' END,
  CASE WHEN effective THEN CASE WHEN supported AND dow='*' THEN 'daily' ELSE 'weekly' END END,
  CASE WHEN effective AND NOT (supported AND dow='*') THEN CASE WHEN supported THEN CAST(dow AS INTEGER) ELSE 0 END END,
  CASE WHEN effective THEN CASE WHEN supported THEN printf('%02d:%02d',CAST(hour AS INTEGER),CAST(minute AS INTEGER)) ELSE '04:00' END END,
  revision,actor_id,actor_key_id,checked_at,code,apply_checked_at,apply_code,bookkeeping_at,bookkeeping_code,candidates FROM mapped;

INSERT INTO settings (key,value)
SELECT 'updateNotify', CASE WHEN EXISTS (
  SELECT 1 FROM stack_update_policies p JOIN image_update_schedules s ON s.environment_id=p.environment_id AND s.registration_id=p.registration_id
  WHERE p.mode IN ('notify','apply') AND s.polling_enabled=1 AND s.actor_id<>''
    AND lower(coalesce((SELECT value FROM settings WHERE key='pollingEnabled'),'true')) NOT IN ('false','f','0')
    AND (p.environment_id='0' OR EXISTS (SELECT 1 FROM environments e WHERE e.id=p.environment_id AND e.enabled=1))
) THEN 'true' ELSE 'false' END
ON CONFLICT(key) DO UPDATE SET value=excluded.value;

INSERT INTO settings (key,value)
SELECT 'vulnerabilityScanSchedule', CASE WHEN lower(coalesce((SELECT value FROM settings WHERE key='vulnerabilityScanEnabled'),'false')) IN ('true','t','1')
  AND json_type(schedule)='object' AND json_extract(schedule,'$.mode') IN ('daily','weekly')
  AND json_extract(schedule,'$.time') GLOB '[0-2][0-9]:[0-5][0-9]' AND substr(json_extract(schedule,'$.time'),1,2)<='23'
  AND NOT EXISTS (SELECT 1 FROM json_each(schedule) WHERE key NOT IN ('mode','day','time'))
  AND ((json_extract(schedule,'$.mode')='daily' AND coalesce(json_extract(schedule,'$.day'),'')='')
    OR (json_extract(schedule,'$.mode')='weekly' AND json_extract(schedule,'$.day') IN ('sun','mon','tue','wed','thu','fri','sat')))
  THEN schedule ELSE '{"mode":"manual"}' END
FROM (SELECT CASE WHEN json_valid(coalesce((SELECT value FROM settings WHERE key='vulnerabilityScanSchedule'),''))
  THEN (SELECT value FROM settings WHERE key='vulnerabilityScanSchedule') ELSE '{}' END AS schedule) WHERE true
ON CONFLICT(key) DO UPDATE SET value=excluded.value;

DELETE FROM settings WHERE key IN ('autoUpdate','autoUpdateInterval','pollingEnabled','pollingInterval','vulnerabilityScanEnabled');
DROP TABLE stack_update_policies;
DROP TABLE image_update_schedules;

CREATE TABLE "container_previous_images" ("container_name" TEXT NOT NULL,"image_id" TEXT NOT NULL,"image_ref" TEXT NOT NULL,"run_id" TEXT NOT NULL,"created_at" datetime NOT NULL,"owner_container_id" TEXT NOT NULL,PRIMARY KEY ("container_name"));
ALTER TABLE image_lifecycle_runs ADD COLUMN "point_code" TEXT;
ALTER TABLE container_recreate_journal ADD COLUMN "expected_image_id" TEXT;
ALTER TABLE image_policy_children ADD COLUMN "settled_at" datetime;
UPDATE image_policy_children SET settled_at=CURRENT_TIMESTAMP WHERE status IN ('refused','settled');

CREATE TABLE "image_protections_m18" ("image_id" TEXT NOT NULL,"reason" TEXT NOT NULL,"owner_id" TEXT NOT NULL,PRIMARY KEY ("image_id","reason","owner_id"),CHECK (reason IN ('system', 'restore-point', 'active-update', 'container-previous')),CHECK (length(image_id) > 0),CHECK (length(owner_id) > 0));
INSERT INTO image_protections_m18 SELECT image_id,reason,owner_id FROM image_protections;
DROP TABLE image_protections;
ALTER TABLE image_protections_m18 RENAME TO image_protections;

-- Preserve receipt identities, terminal evidence and key bindings while extending the operation CHECK.
CREATE TEMP TABLE runtime_key_bindings_m18 AS SELECT * FROM runtime_key_bindings;
DROP TABLE runtime_key_bindings;
CREATE TABLE "runtime_receipts_m18" ("run_id" TEXT NOT NULL,"kind" TEXT NOT NULL,"store_generation" TEXT NOT NULL,"manager_id" TEXT NOT NULL,"registration_id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"executor_generation" TEXT NOT NULL,"request_digest" TEXT NOT NULL,"caller_id" TEXT,"caller_name" TEXT,"operation_id" TEXT,"target_kind" TEXT,"target_id" TEXT,"target_name" TEXT,"parent_project_id" TEXT,"gate_scope" TEXT,"gate_key" TEXT,"admitted_at" datetime,"dispatch_state" TEXT,"dispatched_at" datetime,"executor_state" TEXT,"unknown_reason" TEXT,"executor_result_code" TEXT,"executor_error" TEXT,"executor_started_at" datetime,"executor_settled_at" datetime,"cancel_requested" numeric NOT NULL,"cancel_requested_at" datetime,"last_reconciled_at" datetime,"hub_disposition" TEXT,"hub_disposition_by" TEXT,"hub_disposition_reason" TEXT,"release_acknowledged" numeric NOT NULL,"hub_disposition_at" datetime,"settled_at" datetime,"final_write_at" datetime NOT NULL, "output_next_seq" INTEGER NOT NULL DEFAULT 1,"result" TEXT,PRIMARY KEY ("run_id"),CHECK (length(run_id) > 0 AND length(store_generation) > 0 AND length(manager_id) > 0 AND length(registration_id) > 0 AND length(environment_id) > 0 AND length(executor_generation) > 0 AND length(request_digest) > 0),CHECK (kind IN ('receipt','not_received')),CHECK (cancel_requested IN (0,1) AND (cancel_requested = 1) = (cancel_requested_at IS NOT NULL)),CHECK ((kind = 'receipt' AND caller_id IS NOT NULL AND length(caller_id) > 0 AND operation_id IS NOT NULL AND target_kind IS NOT NULL AND target_id IS NOT NULL AND length(target_id) > 0 AND gate_scope IS NOT NULL AND length(gate_scope) > 0 AND gate_key IS NOT NULL AND length(gate_key) > 0 AND admitted_at IS NOT NULL AND dispatch_state IS NOT NULL AND executor_state IS NOT NULL) OR (kind = 'not_received' AND caller_id IS NULL AND caller_name IS NULL AND operation_id IS NULL AND target_kind IS NULL AND target_id IS NULL AND target_name IS NULL AND parent_project_id IS NULL AND gate_scope IS NULL AND gate_key IS NULL AND admitted_at IS NULL AND dispatch_state IS NULL AND dispatched_at IS NULL AND executor_state IS NULL AND unknown_reason IS NULL AND executor_result_code IS NULL AND executor_error IS NULL AND executor_started_at IS NULL AND executor_settled_at IS NULL AND cancel_requested = 0 AND last_reconciled_at IS NULL AND hub_disposition IS NULL AND settled_at IS NOT NULL)),CHECK (operation_id IS NULL OR (target_kind = 'stack' AND operation_id IN ('deploy-project','down-project','restart-project','redeploy-project','update-project','restore-project','stop-project')) OR (target_kind = 'container' AND operation_id IN ('start-container','stop-container','restart-container','recreate-container','redeploy-container','update-container','restore-container-image'))),CHECK (parent_project_id IS NULL OR (target_kind = 'container' AND length(parent_project_id) > 0)),CHECK (dispatch_state IS NULL OR dispatch_state IN ('pending','dispatching','accepted')),CHECK ((dispatch_state IS NOT NULL AND dispatch_state IN ('dispatching','accepted')) = (dispatched_at IS NOT NULL)),CHECK (executor_state IS NULL OR executor_state IN ('pending','accepted','executing','unknown','success','failed','cancelled')),CHECK ((executor_state IS NOT NULL AND executor_state = 'unknown') = (unknown_reason IS NOT NULL) AND (unknown_reason IS NULL OR length(unknown_reason) > 0)),CHECK ((executor_state IS NOT NULL AND executor_state IN ('success','failed','cancelled')) = (executor_settled_at IS NOT NULL)),CHECK ((executor_result_code IS NULL AND executor_error IS NULL) OR executor_settled_at IS NOT NULL),CHECK (hub_disposition IS NULL OR hub_disposition IN ('cancelled','interrupted','released','refused','not_received')),CHECK ((hub_disposition IS NULL AND hub_disposition_by IS NULL AND hub_disposition_reason IS NULL AND hub_disposition_at IS NULL) OR (hub_disposition IS NOT NULL AND hub_disposition_by IS NOT NULL AND length(hub_disposition_by) > 0 AND hub_disposition_reason IS NOT NULL AND length(hub_disposition_reason) > 0 AND hub_disposition_at IS NOT NULL)),CHECK (release_acknowledged IN (0,1) AND (release_acknowledged = 1) = (hub_disposition IS NOT NULL AND hub_disposition = 'released')),CHECK ((settled_at IS NOT NULL) = (kind = 'not_received' OR executor_settled_at IS NOT NULL OR hub_disposition IS NOT NULL)));
INSERT INTO runtime_receipts_m18 SELECT * FROM runtime_receipts;
DROP TABLE runtime_receipts;
ALTER TABLE runtime_receipts_m18 RENAME TO runtime_receipts;
CREATE UNIQUE INDEX idx_runtime_receipts_environment_run ON runtime_receipts (environment_id,run_id);
CREATE UNIQUE INDEX idx_runtime_receipts_active_scope ON runtime_receipts (environment_id,gate_scope) WHERE kind = 'receipt' AND settled_at IS NULL;
CREATE INDEX idx_runtime_receipts_active_parent ON runtime_receipts (environment_id,parent_project_id) WHERE kind = 'receipt' AND settled_at IS NULL AND parent_project_id IS NOT NULL;
CREATE INDEX idx_runtime_receipts_retention ON runtime_receipts (final_write_at,run_id) WHERE settled_at IS NOT NULL;
CREATE TABLE "runtime_key_bindings" ("environment_id" TEXT NOT NULL,"idempotency_key" TEXT NOT NULL,"run_id" TEXT NOT NULL,"caller_id" TEXT NOT NULL,"caller_name" TEXT,"request_digest" TEXT NOT NULL,"first_seen_at" datetime NOT NULL,PRIMARY KEY ("environment_id","idempotency_key"),CHECK (length(environment_id) > 0 AND length(idempotency_key) > 0 AND length(run_id) > 0 AND length(caller_id) > 0 AND length(request_digest) > 0),FOREIGN KEY (environment_id,run_id) REFERENCES runtime_receipts(environment_id,run_id) ON DELETE RESTRICT);
INSERT INTO runtime_key_bindings SELECT * FROM runtime_key_bindings_m18;
DROP TABLE runtime_key_bindings_m18;
CREATE INDEX idx_runtime_key_bindings_run ON runtime_key_bindings (run_id);
