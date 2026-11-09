# Database baseline and forward migrations

The release baseline is `backend/resources/migrations/sqlite/001_baseline.sql`,
owned by [#361](https://github.com/florianfinn/docklet-next/issues/361). It replaces
the twenty-file pre-release chain. Fresh hub and agent databases have identity
`docklet-next:sqlite:2` and applied Goose history `0,1`. The baseline seeds only
`updateNotify=false` and `vulnerabilityScanSchedule={"mode":"manual"}`; services
add their other defaults during runtime startup.

Pre-release identity `docklet-next:sqlite:1` and legacy migration histories are
refused with the startup diagnostic `database.prerelease`, including the file
path and guidance to start with a new data directory. No pre-release, Arcane,
earlier Docklet or PostgreSQL database or registration is imported. Restore
rejects pre-release archives with `backup.incompatible_migration`, including
archives whose manifest claims a supported version but whose snapshot has the
pre-release identity. This startup diagnostic never reaches an HTTP response;
it needs no API or UI error catalogue entry.

Under the lifetime lock, existing nonempty databases are classified before any
writable SQLite open. Refusal leaves file bytes, permissions, WAL/SHM sidecars
and the independent Francis file unchanged. A failed first migration can leave
only Goose's version table with applied version 0; this exact initial state may
restart. Unowned, inconsistent and newer schemas remain refused. Production
uses forward-only Goose migrations and never AutoMigrate or downgrade.

`schema.BaselineModels()` freezes the release model list used by the generator;
`schema.Models()` includes that list and later additions. The retired update-all
job table is absent. Future migrations start at 002, and their models belong in
`Models()` without changing the baseline model list or the two frozen proofs:

- Generator output from the frozen models, reviewed integrity overlay and SQL
  seed list equals 001 byte for byte.
- The test-only [pre-release chain](../history/prerelease-sqlite-chain.md) produces
  the same catalog and SQL seed data, normalising only the identity literal and
  the intentionally removed `environment_update_jobs` table and its indexes.
- All embedded migrations match AutoMigrate of the current `Models()` plus the
  reviewed integrity overlay, in both directions.

Generate only in a new local temporary file:

```sh
DATABASE_URL=file:baseline.db go run ./backend/cmd/schema-baseline > baseline.sql
```

The generator takes the lifetime lock and refuses existing target files. It is
a development verification command, not an operator import tool. Shipped
migrations are immutable. The guard permits one exact transition from the pinned
20-file pre-release manifest to the pinned release baseline, optionally with
correctly manifested forward migrations; all other shipped changes fail.

`MigrationOptions.Source` is a test-only filesystem seam. Target discovery,
identity/history validation, migration and restore share the source held by the
process database. Production callers use the embedded migrations; no runtime
flag enables another source. Tests use a synthetic 002 to exercise real forward
migration after restore independently of the current release migration count.

The [SQLite database contract](sqlite-database.md#fresh-migration-baseline-and-schema-proof)
records timestamp/JSON representation, storage and integrity requirements. The
[M1 baseline record](../history/m1-postgresql-baseline.md) preserves the earlier
PostgreSQL generation commands as historical reference only.
