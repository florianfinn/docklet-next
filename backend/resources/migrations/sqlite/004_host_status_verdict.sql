-- Source: docklet-next/issues/372
-- Revision: e83119924556ce53fea232ceac7a270bd595304b
-- Retrieved: 2026-11-13
-- License: Apache-2.0
-- Deviations: Original database rule for the host compatibility verdict.

-- +goose Up
ALTER TABLE "environments" ADD COLUMN "status_check_revision" INTEGER NOT NULL DEFAULT 0;
-- +goose StatementBegin
CREATE TRIGGER "environments_verdict_update" BEFORE UPDATE OF "status" ON "environments" WHEN OLD."status" = 'update-required' AND NEW."status" IS NOT OLD."status" AND NOT (NEW."credential_revision" > OLD."credential_revision") AND NOT (NEW."status_check_revision" > OLD."status_check_revision") BEGIN SELECT RAISE(ABORT, 'host.status-verdict'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER "environments_revision_monotonic" BEFORE UPDATE ON "environments" WHEN NEW."status_check_revision" < OLD."status_check_revision" OR NEW."credential_revision" < OLD."credential_revision" BEGIN SELECT RAISE(ABORT, 'host.status-verdict'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER "environments_no_reinsert" BEFORE INSERT ON "environments" WHEN EXISTS (SELECT 1 FROM "environments" WHERE "id" = NEW."id") BEGIN SELECT RAISE(IGNORE); END;
-- +goose StatementEnd
