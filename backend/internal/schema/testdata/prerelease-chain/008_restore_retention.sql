-- Source: docklet-next/docs/design/images-updates.md
-- Revision: 727498b22e03475aa5b4e5325727ff182e8cebdd
-- Retrieved: 2026-10-25
-- License: Apache-2.0
-- Deviations: M6 restore-target protection survives receipt/history cleanup.

-- +goose Up
-- Preserve payload bytes and the existing index while matching canonical model order.
CREATE TEMP TABLE restore_points_retention AS SELECT * FROM restore_points;
DROP TABLE restore_points;
CREATE TABLE "restore_points" ("id" TEXT NOT NULL,"target_id" TEXT NOT NULL,"target_identity" TEXT NOT NULL,"registration_id" TEXT NOT NULL,"source_run_id" TEXT NOT NULL,"created_at" datetime NOT NULL,"last_restored_at" datetime,"failed_attempt" numeric NOT NULL DEFAULT false,"source_state" TEXT NOT NULL,"revision" TEXT NOT NULL,"payload" TEXT NOT NULL,"available" numeric NOT NULL,PRIMARY KEY ("id"),CHECK (available IN (0,1)),CHECK (source_state IN ('settled','partial-source')),CHECK (failed_attempt IN (0,1)));
INSERT INTO restore_points (id,target_id,target_identity,registration_id,source_run_id,created_at,last_restored_at,failed_attempt,source_state,revision,payload,available)
SELECT id,target_id,target_identity,registration_id,source_run_id,created_at,NULL,false,source_state,revision,payload,available FROM restore_points_retention;
DROP TABLE restore_points_retention;
CREATE INDEX `idx_restore_points_target_created` ON `restore_points`(`target_id`,`created_at`);
UPDATE restore_points AS p SET last_restored_at = (
    SELECT MAX(r.admitted_at) FROM runtime_image_intents AS i
    JOIN runtime_receipts AS r ON r.run_id = i.run_id
    WHERE json_extract(i.payload, '$.request.pointId') = p.id
      AND r.target_id = p.target_id AND r.registration_id = p.registration_id
      AND r.kind = 'receipt' AND r.operation_id = 'restore-project'
);
UPDATE restore_points AS p SET failed_attempt = 1
WHERE p.source_state = 'partial-source' AND EXISTS (
    SELECT 1 FROM runtime_receipts AS r
    WHERE r.run_id = p.source_run_id AND r.settled_at IS NOT NULL
      AND r.executor_state <> 'success'
);
