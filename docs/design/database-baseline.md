# PostgreSQL baseline and forward migrations

Docklet Next starts with one PostgreSQL baseline. It does not import Arcane or
earlier Docklet databases. Goose applies later migrations forward at startup;
the application rejects legacy migration state and a database version newer
than the binary. Downgrades are unavailable. Restore a backup when rolling back
an application release that changed the schema.

The baseline is generated from the persistent GORM registry in
`backend/internal/schema/models.go`. Each entry is a retained model. To
regenerate it before the first release, create an empty PostgreSQL 18 database
for this purpose and set `DATABASE_URL` to its connection URL. The command
refuses a database containing public tables:

```sh
cd backend
go run ./cmd/schema-baseline
cd ..
pg_dump --schema-only --no-owner --no-privileges "$DATABASE_URL" \
  | python3 scripts/normalize-baseline.py \
  > backend/resources/migrations/postgres/001_baseline.sql
```

The dump normalizer retains table, sequence, constraint and index definitions,
without PostgreSQL session settings or dump comments. The generated SQL has no
seed rows. `scripts/migration-hashes.json` records its SHA-256. The migration
guard checks that hash and every later shipped migration, and requires later
version numbers to sort after `001`. Regeneration is only a pre-release
operation; changing a shipped baseline requires a new migration instead.

The tagged PostgreSQL integration test creates two empty databases: it applies
Goose to one and GORM AutoMigrate to the other. It compares the catalog in both
directions, excluding only Goose's bookkeeping objects, and checks that all
model tables start empty. A separate test starts hub wiring against another
empty database and requests `/api/health`.

The 90 imported Arcane migration files at source revision
`095cb778daba1034e2a39961199e712a75d4ba29` were removed before release.
This provenance deviation replaces their cumulative Arcane schema with the
retained model schema after the functional cut. The discarded history included
Swarm, GitOps, templates, central variables, S3, standalone image builds,
OIDC, passkeys, federated credentials and APNS.
