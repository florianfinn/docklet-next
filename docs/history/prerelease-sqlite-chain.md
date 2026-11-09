<!--
Source: docklet-next/issues/361
Revision: b11b3e70e671dd3c862b966b1d279251842c7319
Retrieved: 2026-11-09
License: Apache-2.0
Deviations: Original pre-release SQLite chain preservation record.
-->

# Pre-release SQLite migration chain

Before the first release, Goose migrations 001–020 used database identity
`docklet-next:sqlite:1`. They accumulated the core SQLite schema, backup archive
metadata, runtime receipts/output/registration, image protection/lifecycle,
restore retention, update policies, vulnerability attempts/notifications, volume
backup lifecycle, notifications/Auto Heal, container/host operations, simple-user
assignments, runtime history, service widgets, update windows, named targets and
condition state, and removal of webhooks.

The release replaces that chain with one generated baseline and identity
`docklet-next:sqlite:2`. The old SQL files and their hashes are retained unchanged
under `backend/internal/schema/testdata/prerelease-chain/`, for tests only. The
frozen proof compares the full resulting schema and SQL seeds with the release
baseline, normalising the identity literal and the intentionally retired
`environment_update_jobs` table and its indexes. The chain is never a runtime
migration source or an import route. Existing pre-release databases and archives
are refused; operators start with a fresh data directory.

The current [baseline contract](../design/database-baseline.md) defines the
release proofs and immutable forward migrations.
