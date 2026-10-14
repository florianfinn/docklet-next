-- +goose Up
ALTER TABLE activities DROP COLUMN gate_owner;
ALTER TABLE activities DROP COLUMN gate_lease_at;
ALTER TABLE activities DROP COLUMN cancel_requested_at;
