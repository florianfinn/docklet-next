-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/063_drop_notification_logs.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Notification delivery attempts are recorded in the events table now;
-- nothing reads or writes notification_logs anymore.
DROP TABLE IF EXISTS notification_logs;

-- +goose Down
CREATE TABLE IF NOT EXISTS notification_logs (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    image_ref TEXT NOT NULL,
    status VARCHAR(50) NOT NULL,
    error TEXT,
    metadata JSONB,
    sent_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_notification_logs_provider ON notification_logs(provider);
CREATE INDEX idx_notification_logs_sent_at ON notification_logs(sent_at);
