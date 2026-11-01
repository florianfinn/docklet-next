-- Source: docklet-next/issues/322
-- Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
-- Retrieved: 2026-11-01
-- License: Apache-2.0
-- Deviations: Original simple-user resource authorization.

-- +goose Up
ALTER TABLE users ADD COLUMN "disabled_at" datetime;
CREATE TABLE "user_app_assignments" ("id" TEXT NOT NULL,"user_id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"project_id" TEXT NOT NULL,"level" TEXT NOT NULL,"created_at" datetime NOT NULL,PRIMARY KEY ("id"),FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,CHECK (level IN ('restricted', 'full')),UNIQUE (user_id, environment_id, project_id));
CREATE INDEX user_app_assignments_environment ON user_app_assignments(environment_id);
