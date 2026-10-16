-- Source: italypaleale/francis/components/sqlite/migrations/05-workflow-labels.sql
-- Revision: 23cd46831a543a73eec05ad034afd46245225876
-- Retrieved: 2026-10-16
-- License: MIT
-- Deviations: None; pinned SQLite provider restored.

-- The workflow engine's labels for an instance, stored as a JSON object in the row they describe
ALTER TABLE %sactor_state ADD COLUMN workflow_labels text;

-- The label set is closed, so each field gets an expression index of its own (rather than one GIN index over the whole object)
CREATE INDEX %sactor_state_wf_status_idx ON %sactor_state (actor_type, json_extract(workflow_labels, '$.status'), actor_id)
    WHERE workflow_labels IS NOT NULL;
CREATE INDEX %sactor_state_wf_version_idx ON %sactor_state (actor_type, json_extract(workflow_labels, '$.version'), actor_id)
    WHERE workflow_labels IS NOT NULL;
CREATE INDEX %sactor_state_wf_parent_idx ON %sactor_state (actor_type, json_extract(workflow_labels, '$.parent'), actor_id)
    WHERE workflow_labels IS NOT NULL;
