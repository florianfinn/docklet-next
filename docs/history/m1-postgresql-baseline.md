# PostgreSQL baseline and forward migrations

Archived on 2026-10-16 from the pre-SQLite design at
`1ead6d2c7e17c79b46c42eb8567c3a1d70730fd5`. This records the M1 input and
PostgreSQL implementation, not the current database decision. See the
[SQLite database contract](../design/sqlite-database.md).

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

To rederive the integrity reference and overlay before release, apply the
upstream PostgreSQL migrations `001` through `090` at revision
`095cb778daba1034e2a39961199e712a75d4ba29` to an empty PostgreSQL 18
database. Generate a second, model-only database with `go run
./cmd/schema-baseline`. Query both databases with the `integrityCatalog` SQL
in `backend/internal/schema/schema_integration_test.go`: it emits sorted
`notnull`, `default`, `constraint` and `index` properties using
`pg_get_expr`, `pg_get_constraintdef` and `pg_get_indexdef`. Query the model
database's retained table and column names with:

```sql
SELECT c.relname, a.attname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid
WHERE n.nspname = 'public' AND c.relkind = 'r'
  AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY c.relname, a.attname;
```

Keep old-chain properties only for those tables. Keep `notnull` and `default`
properties only for retained columns. Keep constraints only when every local
and referenced column and table survives; `pg_constraint.conkey`, `confrelid`
and `confkey` give the exact dependencies. Keep indexes only when their table
and indexed columns survive. The removed `s3_destination_id` column excludes
exactly four indexes: `idx_volume_backups_s3_destination_id`,
`idx_volume_backup_policies_s3_destination_id`,
`idx_system_backup_runs_s3_destination_id` and
`idx_system_backup_policies_s3_destination_id`. No other old-chain property
on a retained table or column is excluded. Sort the retained properties by
type, table, column or definition and write them as JSON arrays in
`retained-integrity.json`, matching the parsing in the tagged test. The
current reference has 223 nullability, 111 default, 54 constraint and 112
index entries.

Compare that reference with the model-only `integrityCatalog`. For missing
nullability and defaults, add `ALTER TABLE ... ALTER COLUMN ... SET NOT NULL`
or `SET DEFAULT` statements to `retained_integrity.sql`. For missing
constraints, use `pg_constraint.conname` and `pg_get_constraintdef` from the
old database to add named `ALTER TABLE ... ADD CONSTRAINT` statements. For
missing indexes, use the old `pg_get_indexdef` as `CREATE INDEX` statements.
Sort each statement group by table and column or object name. Apply the
overlay to the model database, then require all reference entries to occur
in its catalog and regenerate the baseline byte for byte. The tagged schema
test verifies that result against the shipped baseline.

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
