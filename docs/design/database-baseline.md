# Database baseline and forward migrations

The [SQLite database contract](sqlite-database.md#fresh-migration-baseline-and-schema-proof)
defines the fresh, renumbered SQLite baseline, timestamp/JSON representation,
provenance/hash transition and schema proof. It replaces the PostgreSQL-only
baseline decision under [#60](https://github.com/florianfinn/docklet-next/issues/60).
The runtime transition is owned by implementation step 2 in that contract.

No earlier Arcane, Docklet or development PostgreSQL data is imported. Before
the first SQLite release, generate a fresh baseline from retained models and
reviewed integrity properties. Shipped migrations remain immutable; later
migrations run forward at startup, with foreign/newer schema rejection and no
downgrade. Production never uses AutoMigrate.

The historical PostgreSQL generation commands and retained
integrity derivation are preserved in the
[M1 baseline record](../history/m1-postgresql-baseline.md). They are reference
material for verifying equivalent constraints, not SQLite setup instructions.

Generate the development schema only in a new local temporary file:

```sh
DATABASE_URL=file:baseline.db go run ./backend/cmd/schema-baseline > baseline.sql
```

Der Befehl übernimmt die Hub-Lebenszeitsperre, lehnt vorhandene Zieldateien ab
und erzeugt deterministisches SQL aus den beibehaltenen Modellen und den
geprüften SQLite-Integritätsregeln. Its output must
match `backend/resources/migrations/sqlite/001_baseline.sql` byte for byte. This
is a development verification command, not an operator migration or import tool.
The independent retained-integrity fixture and behavioural constraint tests
verify the overlay as well as the generated migration.
