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

The current PostgreSQL implementation's generation commands and retained
integrity derivation are preserved in the
[M1 baseline record](../history/m1-postgresql-baseline.md). They are reference
material for verifying equivalent constraints, not SQLite setup instructions.
