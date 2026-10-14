-- +goose Up
ALTER TABLE activities DROP COLUMN gate_owner;
ALTER TABLE activities DROP COLUMN gate_lease_at;
