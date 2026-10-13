-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/006_rename_tables.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE IF EXISTS users_table RENAME TO users;
ALTER TABLE IF EXISTS image_update_table RENAME TO image_updates;
ALTER TABLE IF EXISTS images_table RENAME TO images;
ALTER TABLE IF EXISTS containers_table RENAME TO containers;
ALTER TABLE IF EXISTS networks_table RENAME TO networks;
ALTER TABLE IF EXISTS volumes_table RENAME TO volumes;
ALTER TABLE IF EXISTS stacks_table RENAME TO stacks;

-- +goose Down
ALTER TABLE IF EXISTS users RENAME TO users_table;
ALTER TABLE IF EXISTS image_updates RENAME TO image_update_table;
ALTER TABLE IF EXISTS images RENAME TO images_table;
ALTER TABLE IF EXISTS containers RENAME TO containers_table;
ALTER TABLE IF EXISTS networks RENAME TO networks_table;
ALTER TABLE IF EXISTS volumes RENAME TO volumes_table;
ALTER TABLE IF EXISTS stacks RENAME TO stacks_table;
