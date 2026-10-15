-- Source: docklet-next/docs/design/api-contract.md
-- Revision: 1d6b25c3162636f018e0a6491e7f5428110874e3
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #37; adds coded run traces and gate lookups.
-- +goose Up
ALTER TABLE activities ADD COLUMN message_code text;
ALTER TABLE activities ADD COLUMN affected_services text;
ALTER TABLE activities ADD COLUMN gate_key text;
ALTER TABLE activities ADD COLUMN gate_scope text;
ALTER TABLE activity_messages ADD COLUMN code text;
ALTER TABLE activity_messages ADD COLUMN step text;
ALTER TABLE events ADD COLUMN message_code text;
UPDATE events SET message_code = type;
CREATE TABLE activity_sync_receipts (
    run_id text PRIMARY KEY,
    updated_at timestamp with time zone NOT NULL
);
CREATE INDEX idx_activities_gate_scope ON activities (gate_scope);
CREATE INDEX idx_activities_gate_key ON activities (gate_key);
