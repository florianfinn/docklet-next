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
cat backend/internal/schema/retained_integrity.sql \
  >> backend/resources/migrations/postgres/001_baseline.sql
```

The dump normalizer retains table, sequence, constraint and index definitions,
without PostgreSQL session settings or dump comments. It rejects unknown SQL
statements. The overlay contains retained-column defaults, nullability,
foreign keys, checks and indexes derived from the 90 old PostgreSQL migrations
applied to an empty database, excluding columns and objects removed by the
functional cut. The independent test reference in
`backend/internal/schema/testdata/retained-integrity.json` records 500 expected
properties from that old chain. The generated SQL has no seed rows.
`scripts/migration-hashes.json` records its SHA-256, and
`scripts/check-migrations.mjs` pins the baseline SHA-256 independently of the
manifest. Changing the baseline before first release requires an explicit
reviewed update of both pins and the old-chain reference when applicable;
shipped baselines require a new forward migration instead. The guard rejects
missing manifests, missing baselines and version numbers not after `001`.

The tagged PostgreSQL integration test creates two empty databases: it applies
Goose to one and GORM AutoMigrate plus the retained integrity overlay to the
other. It compares the catalog in both directions, excludes only Goose's
bookkeeping objects, checks all 500 independently recorded old-chain integrity
properties, and checks that all model tables start empty. A separate test
starts hub wiring against another empty database and requests `/api/health`.

The 90 imported Arcane migration files at source revision
`095cb778daba1034e2a39961199e712a75d4ba29` were removed before release.
This provenance deviation replaces their cumulative Arcane schema with the
retained model schema after the functional cut. The discarded history included
Swarm, GitOps, templates, central variables, S3, standalone image builds,
OIDC, passkeys, federated credentials and APNS.
