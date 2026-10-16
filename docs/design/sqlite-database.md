# SQLite hub database

## Decision and scope

SQLite is the only supported hub database. The maintainer accepted
[decision #60](https://github.com/florianfinn/docklet-next/issues/60) on
2026-10-16. The operating scale is one operator, a few private users and one hub
process, with the retained Arcane behaviour. Keeping a separate PostgreSQL
service and two SQL dialects adds operational and test work without an accepted
need at this scale. Agents do not open the hub database.

Existing development databases are discarded. There is no PostgreSQL-to-SQLite,
Arcane or earlier Docklet database/registration import. Nothing has been released;
this decision permits one fresh, renumbered SQLite migration baseline. Once a
SQLite package ships, its migrations are immutable and later changes run forward
at startup. PostgreSQL URLs fail validation with empty-database setup guidance.

[Milestone 6](https://github.com/florianfinn/docklet-next/milestone/6) owns runtime,
Francis, schema, concurrency, tests, deploy configuration and CI. Hub backup
[#32](https://github.com/florianfinn/docklet-next/issues/32) resumes on SQLite
after this milestone merges to main; its engine, archive schema, API and UI are
outside this milestone. Sign-in disable through a startup environment variable
belongs to M4 [#58](https://github.com/florianfinn/docklet-next/issues/58).

This is the implementation contract for the SQLite runtime, schema, persistence
checks and package. There are no remaining product questions.

## Driver, provenance and licences

Use `github.com/libtnb/sqlite` v1.2.2 as the GORM dialector and
`modernc.org/sqlite` v1.60.1 as the pure-Go driver, matching the pinned Arcane
dependency selection. Both hub and agent builds retain `CGO_ENABLED=0`; no SQLite
shared library, C compiler or SQLite CLI is needed in the runtime image. The
[pinned dialector](https://github.com/libtnb/sqlite/blob/9f6db903b5608c0590eda65aada576306f322f80/sqlite.go)
supports an injected connection pool and suppresses GORM row-lock clauses.
Consequently, deleting a `FOR UPDATE` clause alone gives no replacement guarantee.

Arcane reference sources are v2.15.1 at
`095cb778daba1034e2a39961199e712a75d4ba29`:
`backend/internal/database/database.go`, `backend/pkg/utils/dbutil/dbutil.go`,
`backend/pkg/utils/francis/provider.go`,
`runtime.go` and `backend/pkg/libarcane/startup/runtime_identity.go`.
Recover only the required file handling and wiring. No retained SQL calls the
`normalize` scalar; leave `backend/pkg/utils/sqlite/sqlite.go` out unless a caller
appears. The [M1 removal inventory](../history/m1-sqlite-removal.md) records all
90 upstream SQLite migration paths; do not restore those migrations, dual-dialect
branches or old data import.

The wrapper is MIT; the modernc driver has BSD-3-Clause source and bundles other
notices, including SQLite's public-domain dedication. The full selected graph,
rather than one module label, determines redistribution duties. Francis is MIT
with retained Apache-2.0 notices for adopted Dapr material; adopted Arcane code
retains BSD-3-Clause. Preserve per-file source, full revision, retrieval date,
licence and specific deviations in headers/sidecars under the
[provenance rules](provenance.md).

Refresh `scripts/inventory-core.mjs`, `docs/provenance/core-import.json`, the Go
licence inventory and packaged notices for the actual hub and agent graphs.
The old SQLite-removal graph boundary cannot certify a SQLite product. In
particular, reassess the exact compiled modernc libc header files and their
LGPL/GCC-exception notices described in [Arcane adoption](arcane-adoption.md).
Record exact hashes and obligations; no blanket licence approval follows from
this driver decision. Unknown or incompatible compiled material blocks release.
Remove PostgreSQL providers and tooling, tidy all affected modules, and prove
the shipped graphs contain no PostgreSQL driver/provider. A transitive driver
that remains reachable must be removed at its importing edge; merely deleting
its name from a manifest is insufficient.

## Files and configuration

Keep `DATABASE_URL` as the only database location setting. The native default is
`file:data/docklet.db`, relative to the process working directory. The Docker
template explicitly sets `DATABASE_URL=file:/app/data/docklet.db` and mounts the
local named volume `hub-data` at `/app/data`. Bind mounts are supported on Linux
hosts only; use the named volume on Docker Desktop and other hosts.
Use the standard `file:` URI escaping for spaces and special characters.

| File for the Docker default | Purpose |
| --- | --- |
| `/app/data/docklet.db` | Hub tables, migration state and database identity |
| `/app/data/docklet.francis.db` | Francis-owned actor state, jobs and migrations |
| Each database path plus `-wal` and `-shm` | SQLite-managed WAL and shared-memory files |
| `/app/data/docklet.db.lock` | Stable inode for the exclusive lifetime hub lock |

Derive Francis's filename by replacing the main file's final extension with
`.francis` plus that extension, or appending `.francis.db` if it has none. This
matches the pinned [Arcane sidecar rule](https://github.com/getarcaneapp/arcane/blob/095cb778daba1034e2a39961199e712a75d4ba29/backend/pkg/utils/francis/provider.go).
The lock path is the resolved main file path plus `.lock`. Resolve relative
paths and existing parent symlinks once, create the parent if needed, and reject
symlink/hard-link database aliases that could acquire a different lock for the
same database. Do not rename or replace database files while the hub is running.

Accept file-backed, writable local databases only. Reject remote URI authorities,
memory databases, shared-cache memory modes, read-only/immutable modes,
PostgreSQL schemes and the `vfs` option. Parse query options explicitly; reject
options that override the required connection policy. The application supplies pragmas and transaction
mode internally, so the normal operator URL needs no query string. Do not add
`DATABASE_TYPE`, `DB_DRIVER`, a separate Francis URL or pool/pragmas env settings.
Existing `_FILE` secret conventions and `PUID`/`PGID` identity handling remain.
Step 2 updates the config default and `.env.example` together.

Both databases, their WAL/SHM files and the lock must reside on local persistent
storage available to the same host. SMB/NFS and other network shares are
unsupported for database files, including a named volume backed by such a share.
This follows the [WAL same-host requirement](https://www.sqlite.org/wal.html).
On Linux, check the resolved database directory before opening either file with
`go-sql-utils/sqlite.IsNetworkedFileSystem` v0.3.9, already a Francis dependency.
Fail startup on detection errors or a positive result (NFS, SMB/CIFS or FUSE),
with guidance to use local persistent storage. This catches common mistakes;
it cannot certify every storage backend. Backup archives may use network targets.
Keep directory access private to the runtime identity; create database/lock files
with mode `0600` and their directory with `0700`, subject to a stricter umask.
The runtime UID needs creation and rename rights for SQLite's companion files.

## Connections, transactions and contention

Open one hub pool with `MaxOpenConns=1`, `MaxIdleConns=1` and no periodic
connection expiry. Use the same limits for the Francis writer pool. Francis
diagnostics may use one separately owned read-only connection to its sidecar;
it must never become a second writer. There is no extra main read pool initially.
WAL preserves the option of bounded readers later, but measurements must justify
that change. Never call the parent pool from an open transaction: with one
connection that would deadlock waiting for itself. No checked-out connection,
cursor or transaction may span network I/O, an external callback or an iterator
handed to another layer.

Change `ExportVulnerabilities` (`vulnerability/children/query/service.go:536`):
its `Rows()` cursor currently survives into the CSV `StreamResponse`
(`handler.go:286-298`). Fetch bounded keyset pages with short reads, materialise
and close each page before yielding or writing, and retain the requested sort
with a unique tie-breaker. Do not keep a transaction across pages. With
`MaxOpenConns=1`, a blocked CSV writer must leave the connection available for a
concurrent activity completion within its existing 10-second deadline. Audit
held cursors and deeper helper calls as well as direct transaction closures.

| Setting | Required value and application |
| --- | --- |
| `journal_mode` | `WAL`, set before migrations and verified on both files |
| `foreign_keys` | `ON`, configured on every physical connection before transactions |
| `busy_timeout` | `2500` milliseconds on every physical connection |
| `synchronous` | `FULL` on every write connection; durable commits take priority over unmeasured throughput |
| `wal_autocheckpoint` | `1000` pages on each writer; retain SQLite's ordinary checkpoint mechanism |
| Transaction start | `_txlock=immediate` for write pools so GORM/SQL transactions begin with `BEGIN IMMEDIATE` |
| Hub time binding/scanning | `_time_integer_format=unix_micro` and `_inttotime=1` on every hub/migration connection; declared `DATETIME`/`TIMESTAMP` columns store integer microseconds and scan as UTC `time.Time`. Francis retains its separate millisecond contract. |

Use a shared connection factory/modernc DSN setup for hub, migrations, Francis
and diagnostic access. Francis writer and diagnostics pools explicitly omit both
hub time options; their timestamps remain integer milliseconds. A single `Exec(PRAGMA foreign_keys=ON)` on a pool is
insufficient. Test connection replacement and verify the actual values; fail
startup if required settings cannot be established. The driver documents
[connection options](https://pkg.go.dev/modernc.org/sqlite@v1.60.1#Driver.Open).
Explicitly set the policy rather than depending on dialector defaults.

SQLite has one writer per file. Acquire the write transaction before reading
state used to decide a write; a deferred read transaction followed by an upgrade
is not the selected pattern. Keep transactions short and local to database work.
Do password hashing, Docker calls, actor RPC, filesystem transfers and live
publication outside them. Publish events and invalidate caches only after commit.
The [transaction rules](https://www.sqlite.org/lang_transaction.html) explain
why `BEGIN IMMEDIATE` can fail busy before any read-modify-write decision.

Keep `busy_timeout`; classify an exhausted modernc `SQLITE_BUSY`, including
extended variants by primary code, in the existing `IsRetryableWriteError`.
Retain the activity completion/append retry loops and their original 10-second
owner-write deadline; add no generic retry layer or separate backoff policy.
The sole connection and lifetime lock serialize cooperating writers.
`SQLITE_LOCKED` indicates cursor/connection misuse to fix. Constraint, disk-full,
I/O and corrupt-file errors are not busy errors. Never replay an external action
or an uncertain commit.

Ordinary writes queue in the pool; queue time belongs in latency measurements.
Long readers can prevent WAL reclamation. Sample WAL bytes and passive checkpoint results;
close leaked readers rather than truncating a live WAL file. A clean shutdown
may checkpoint after readers and writers stop. An uncheckpointed WAL after a
crash is recovered by SQLite on next open, never manually deleted.

## Single hub and run admission

Replace `backend/internal/database/hub_lock.go`'s PostgreSQL session with a
nonblocking `flock(LOCK_EX | LOCK_NB)` on the stable `.lock` file on Linux,
with equivalent descriptor-owned locks on other supported targets. The lock is
acquired before opening/migrating either database, startup repair, Francis,
jobs or HTTP readiness. Keep its descriptor open for the whole hub lifetime.
A second process fails immediately with `hub.already_running`, starts no HTTP
or jobs and changes no migration state. The OS releases the lock on crash.
Never unlink the lock file on release, because replacing its inode could admit
two holders. On shutdown, stop work and close every database handle before
unlocking. Failed startup closes all resources and releases the lock.

Implement the lock for supported native build targets, failing closed where
unsupported; production Docker is Linux. Do not use classic process-owned POSIX
record locks: closing another descriptor can release them. The hub must never
open live database, WAL or SHM files through ordinary file descriptors; use
`Lstat`/`Stat` for alias/ownership checks and SQLite APIs for contents. Follow
[SQLite's descriptor guidance](https://www.sqlite.org/howtocorrupt.html#posix_advisory_locks_canceled_by_a_separate_thread_doing_close_).
Rejecting `vfs` also prevents a collision with the unix-dotfile VFS's `.lock` name.

Administrative writers apply the same runtime identity before resolving paths or
creating files and acquire the same lifetime lock. In particular,
`backend/cli/admin/reset_password.go` (line 68) currently initializes/migrates directly;
`admin reset-password` must refuse a running hub with `hub.already_running`.
When stopped, it acquires the lock, migrates forward, resets the password and
revokes sessions, closes the database and releases the lock. Keep the existing
explicit `ALLOW_CLI_PASSWORD_RESET=true` gate and terminal password prompt.
Read-only inspection must not migrate. Goose uses the SQLite dialect under this
lock; remove its PostgreSQL session locker. The lock is cooperative on a local filesystem, not a defence
against an operator editing or replacing files beneath a running hub.

Replace the run-scope advisory lock at `backend/internal/activity/gate.go:77`
with one immediate transaction covering active lookup, freshness lookup and
insert. Keep the local running/cancelling mutex through commit and worker
registration, with a consistent mutex-before-database order. Add a unique partial
index on `(environment_id, gate_scope)` where `gate_scope <> '' AND status IN
('queued', 'running')`. Gated admission still creates `running`; the broader
predicate guards malformed queued contenders too. Ungated rows have no scope.
An index conflict triggers rollback and a fresh admission lookup, never a
second worker. Scope/key and status constraints remain explicit in the baseline.

Only a committed new row yields `run.started` and an executable context.
Same-key active work joins; different keys return `run.busy`; cancelling work
returns `run.cancelling`, including checks. These precede freshness. A successful
run is fresh only when `ended_at > now_us - 30000000`; exactly 30 seconds is expired.
Use the same bound clock sample for the transaction's decisions and timestamps.
`Recheck` bypasses freshness only. Keep startup repair and the one-terminal-write
and receipt semantics in [run-gate.md](run-gate.md); database serialization does
not make remote Docker effects atomic or replay-safe.

## Replacement of every row-lock site

The inventory below is verified against
`1ead6d2c7e17c79b46c42eb8567c3a1d70730fd5` on 2026-10-16. Line numbers refer to
that tree. There are 13 GORM `clause.Locking{Strength: "UPDATE"}` sites. All
replacement reads occur inside the immediate transaction, including callers of
the user/role helpers. Remove the locking clauses and rename lock-only helpers
to describe their remaining validation/read purpose.

| Site under `backend/internal/` | Invariant and SQLite replacement |
| --- | --- |
| `user/service.go:231`, `checkTargetPrivilegeInternal` | Read target and actor plus current assignments inside the enclosing update/delete transaction; re-evaluate privilege and existence before mutation. Concurrent assignment changes must serialize with this check. |
| `user/service.go:346`, `SetPasswordAndRevokeSessionsExcept` | Hash before the transaction; read current user, update password and revoke all other sessions atomically. A deleted user remains not-found; a rollback changes neither password nor sessions. |
| `role/service.go:184`, `lockAssignedUserRowsInternal` | Read affected user IDs and validate rows in the role update/delete transaction; collect IDs for cache invalidation after commit. Assignment writers share the immediate transaction policy. |
| `role/service.go:205`, `UpdateRole` | Read and reject built-in roles, enforce the unique role name in the database, update permissions and collect affected users in one transaction. Map uniqueness failures to the existing conflict. |
| `role/service.go:250`, `DeleteRole` | Read and reject built-in roles, capture affected users, delete role/assignments with the same FK actions, then invalidate after commit. |
| `role/service.go:326`, `replaceUserAssignmentsForSourceInternal` | Verify user/role/environment existence, replace only that assignment source and check at least one effective global admin remains, all before commit. Preserve unique assignment indexes, including NULL scope semantics. |
| `kv/service.go:110`, `IncrementInt64` | Read/parse/add/write atomically; preserve int64 parsing behaviour and unique key on the missing-key insert. Concurrent increments must sum exactly, including the first insert. |
| `activity/completion.go:76`, `CompleteActivity` | Check ownership and cancellation, conditionally update `WHERE status IN ('queued','running')`, check affected rows and append one final message in the same transaction. Zero affected rows produces no second message. |
| `activity/service.go:589`, `UpdateActivity` | Read ownership/state, update only active rows and require one affected row before publishing the snapshot. Terminal work cannot be reopened. |
| `activity/service.go:695`, `appendBatchWithRetryInternal` | Read ownership/state and atomically insert the batch/output plus snapshot progress. Keep the active-status conditional update and affected-row check; rollback inserts if it loses. Publish only the committed batch. |
| `activity/service.go:770`, `PatchActivityMetadata` | Read and merge JSON under the immediate transaction; condition the write on active status and check affected rows, preserving disjoint concurrent patches. |
| `project/service.go:2869`, `UpdateProjectTag` | Verify project and current Compose/UI tag ownership in the transaction, then apply the UI change. Keep unique tag keys; report Compose ownership conflicts without partial edits. |
| `project/service.go:2890`, `reconcileComposeProjectTags` | Verify project, replace Compose tags and resolve UI collisions atomically. Concurrent UI edits observe either the complete old or complete new assignment set. |

For all conditional single-row updates/deletes whose outcome matters, distinguish
zero affected rows from success. Do not use GORM `Save` as a fallback insert after
a lost conditional update. Keep durable uniqueness for KV keys, role names, tag
assignments, API permissions and run-sync receipts. Test existing `ON CONFLICT`
targets against the SQLite indexes, especially `COALESCE(environment_id, '')`.

## Values, timestamps and JSON

Declare hub timestamp columns as `DATETIME` (or `TIMESTAMP`), with numeric
affinity and signed 64-bit Unix-microsecond storage. The hub driver's mandatory
`_time_integer_format=unix_micro&_inttotime=1` centrally binds `time.Time` as
integer microseconds and scans declared timestamp columns back as UTC
`time.Time`. This covers model writes, map updates, predicates and raw SQL.
Microseconds preserve PostgreSQL timestamp precision and the activity batch
writer's microsecond ordering. Preserve the per-batch offsets and stable
`created_at, id` ordering in message queries; do not truncate them to milliseconds.
Public DTOs remain `time.Time` and RFC3339 JSON. A nullable timestamp uses SQL NULL and a nil pointer; the zero Go
time is distinct from Unix epoch zero. Keep interval/duration counters as their
declared units, not as timestamp values.

Keep GORM time fields and auto-time handling without a timestamp serializer or
`type:integer` tags. Only computed projections without declared types, such as
`MAX(created_at)`, need an explicit microsecond scanner accepting `int64` and
NULL where nullable, converting with `time.UnixMicro(value).UTC()` and rejecting
text/floats. Francis diagnostics use the separate millisecond scanner below.
Verify binding/scanning after physical connection replacement, not just at open.
Require a future-row `Where("expires_at < ?", time.Now())` to match zero rows,
plus session/receipt retention and stale-run boundary tests. Prove `CreatedAt`
and `UpdatedAt` auto-population on `Create`, `Save` and `Updates(map)`, nullable
and zero-time round-trips, and `typeof(column) = 'integer'`. The retained destructive
predicates include `session/service.go:149`, `activity/retention.go:80` and
`activity/repair.go:63`; a write-only type assertion cannot protect these reads.

Take UTC time from the single hub's injected clock after acquiring the write
transaction. Replace `NOW()`, `INTERVAL`, `EXTRACT`, `GREATEST` and PostgreSQL
casts in `activity/gate.go`, `completion.go`, `service.go` and `repair.go` with
bound microseconds and Go duration arithmetic. Terminal duration is
`max(0, (ended_us - started_us) / 1000)`. Compare integer cutoffs for the
30-second gate, two-minute ungated sweep and retention. A backwards wall-clock adjustment may
delay freshness/expiry; it must not produce a negative duration or a second
completion. No cross-hub clock coordination is needed.

Preserve required schema defaults with reviewed integer-microsecond SQLite
expressions, with the same type/precision tests; ordinary application writes
supply explicit values. Do not mix `CURRENT_TIMESTAMP` text or seconds with
integer microseconds. Francis's separate, pinned SQLite schema stores Unix
milliseconds. Keep its provider-owned format and use a distinct diagnostics
scanner with `time.UnixMilli`; never pass those values to the Hub microsecond
scanner. Test both boundaries explicitly.

Store JSON documents/arrays as `TEXT`; remove Hub `type:jsonb` tags and casts.
Preserve SQL NULL versus JSON `null` and empty object/array semantics in
`database.JSON`, `StringSlice` and feature serializers. Bind serialized JSON as
a Go string so SQLite stores `TEXT`, not the `BLOB` produced by binding raw
`[]byte`; assert `typeof` as well as round-trip contents. For declared JSON fields,
use `CHECK (value IS NULL OR json_valid(value))` with real column names and
preserve their existing nullability/defaults. Add object/array restrictions only
where already required by the model contract. Francis owns its JSON functions
and matching expression indexes. Text search is not a substitute for JSON parsing.

Accept SQLite's default ASCII case-insensitive `LIKE` for user-facing search,
including `event/service.go:291` and `project/service.go:1692`; non-ASCII case
folding is not promised. Keep text equality and ordering at SQLite's default
`BINARY` collation, including uppercase-before-lowercase ASCII ordering. No
locale-sensitive sorting or global `case_sensitive_like` pragma is introduced.
These follow [SQLite's LIKE](https://www.sqlite.org/lang_expr.html#like) and
[collation rules](https://www.sqlite.org/datatype3.html#collating_sequences).
Exact namespaces must stay case-sensitive: change `kv/service.go:70`
(`ListByPrefix`, used for rename journals/cleanup) to
`substr(key, 1, length(?)) = ? COLLATE BINARY`, binding the literal prefix twice.
Test mixed-case prefixes and retain literal `%`, `_` and backslash cases;
also cover search case and text ordering in the default suite. Fixed,
application-written prefixes in volume, backup, image-update, CVE and event queries
may retain `LIKE`; their generated values do not introduce user namespace ambiguity.

## Fresh migration baseline and schema proof

Step 2 owns all migration and model foundations; no parallel branch introduces
another baseline. Replace the seven current PostgreSQL SQL files with
`backend/resources/migrations/sqlite/001_baseline.sql`. Fold their final retained
schema into this single baseline, including run traces, receipt `synced_at`,
single-hub gate fields, raw activity output and the active-scope unique index.
Do not recreate removed owner/lease/cancel columns or #32's unmerged archive tables. Later
SQLite changes start at `002`. Francis migrations remain separate in its sidecar.

Port the retained registry in `backend/internal/schema/models.go` and regenerate
on an empty temporary SQLite file. Review every retained nullability, default,
CHECK, FK action and unique/partial/expression index from the PostgreSQL reference
for equivalent SQLite behaviour. Counting `CREATE TABLE` and `CREATE [UNIQUE]
INDEX` statements in the source baseline gives 33 tables and 84 explicit indexes
(excluding constraint-created indexes); these are not required SQLite counts. Numeric sequence
IDs become `INTEGER PRIMARY KEY` with the model's required allocation behaviour;
UUID/text identifiers remain text. SQLite table-rebuild migrations must preserve
constraints and indexes and finish with a foreign-key check.

Replace `retained_integrity.sql` and its PostgreSQL catalogue proof with a
SQLite-specific overlay/reference. Use two independent fresh files: Goose
baseline versus model generation plus reviewed integrity overlay. Compare both
directions through `sqlite_master`, `PRAGMA table_info`, `foreign_key_list`,
`index_list` and `index_xinfo`; inspect table/index SQL for CHECKs, expressions
and partial predicates. Exclude only named Goose/SQLite bookkeeping objects.
Keep an independently reviewed semantic inventory of the old retained integrity
properties and account for every deliberate dialect adaptation; comparing two
schemas generated by the same incomplete model cannot prove completeness.
SQLite stores `notification_settings.provider` as text and deliberately does
not enforce the former `varchar(50)` limit. This is a documented dialect
adaptation; the application uses its defined provider identifiers.
Test FK deletion actions, uniqueness with NULL scopes, JSON checks, defaults,
numeric ID allocation and empty tables behaviourally. Require
`PRAGMA foreign_key_check` empty and `PRAGMA integrity_check` equal to `ok`.

Update `scripts/check-migrations.mjs`, `migration-hashes.json`, provenance
sidecars/overrides and their guard tests together. Replace the old independent
baseline pin with the reviewed SQLite SHA-256 and retain fail-closed checks for
missing/changed/unmanifested files and duplicate versions. Key the new manifest
by paths relative to `backend/resources/migrations/`, starting with
`sqlite/001_baseline.sql`, instead of the current bare filenames
(`scripts/check-migrations.mjs:13-46`). The comparison-base
guard needs an explicit, narrowly defined one-time transition from the exact
seven-hash PostgreSQL manifest at the inspected base to the new SQLite manifest;
it must not skip checks merely because a directory or manifest changed. Tests
must reject a changed old hash, unrelated deletion and altered SQLite baseline.
Prove the transition against both the step PR's feature base and the closing
PR's main base. After that transition, existing SQLite hashes cannot be rewritten
or removed.

Record a Docklet SQLite database identity/baseline revision in application-owned
metadata. Under the lifetime lock, inspect `sqlite_master` before Goose creates
state: accept an empty file, or the expected identity with a supported migration
history. Reject legacy `schema_migrations`, unowned nonempty files, Arcane
tables/identity, inconsistent/missing applied versions and newer schemas.
Goose runs forward at startup; production does not run AutoMigrate or downgrade.
Reopen/restart tests must prove an already current schema is unchanged and a
failed startup never becomes ready.

## Francis provider

Restore `third_party/francis/components/sqlite/`, its migrations and embedded
queries from `italypaleale/francis` revision
`23cd46831a543a73eec05ad034afd46245225876`, matching the current local fork.
Restore `WithSQLiteProvider` and provider construction in `host/local`, plus
SQLite-only standalone wiring if retained callers need it. Remove PostgreSQL
components and standalone PostgreSQL wiring entirely. Keep the memory provider
for pure tests. Record all restored files and justified changes in provenance;
assess oversized files under the size ratchet before adoption.

The pinned [Francis component](https://github.com/italypaleale/francis/tree/23cd46831a543a73eec05ad034afd46245225876/components/sqlite)
owns its schema, leases, host registration, job finalization and migrations.
Use its `SQLiteProviderOptions.DB` to inject the configured single-connection
sidecar writer, with explicit lifecycle ownership, or its connector only after
proving the identical connection policy. Keep `MaxHosts=1` and the existing
one-second alarm poll. Opening diagnostics uses the sidecar's read-only pool;
it must not create tables or apply migrations. Shut it down with the provider.

Adapt `backend/pkg/utils/francis/{provider,runtime}.go`, diagnostics timestamp
scanning and test host wiring. Do not import Arcane's legacy in-main Francis
store migration. Verify state/TTL, job dispatch/retry/terminal history, lease
renewal, host health/draining, coordinator receipts and restart under real temp
files. The Hub's 13 row-lock replacements do not translate Francis's PostgreSQL
stored procedures; the restored provider supplies its SQLite algorithms.

## Backup compatibility boundary

The two adjacent files and deterministic names support #32 without implementing
its engine here. That feature will take each image with `VACUUM INTO` or the
driver's [online backup API](https://www.sqlite.org/backup.html), never by copying
a live main file while ignoring its WAL. `VACUUM INTO` creates a compact snapshot
and requires a new destination and space for a full image; follow its
[sync/durability rules](https://www.sqlite.org/lang_vacuum.html#vacuuminto).

Quiesce admissions, scheduler/actors and all writes to both files, drain active
transactions, take both images under one maintenance barrier and validate their
integrity, foreign keys, identities, versions and manifest hashes before
publication. Two sequential online backups alone do not provide a coordinated
snapshot. Main and Francis state have no cross-file transaction guarantee.
Archive storage is separate from live database paths. The later product keeps
local backups and downloads, plus copies to mount/SMB volume-backup targets once
that transfer exists; network archive destinations do not permit network DBs.

Restore is offline by same-filesystem file replacement after every connection
is closed and the lifetime lock is acquired. Validate both images first, retain
safety copies, and coordinate the pair with a durable restore marker: two file
renames are not atomic together. Handle old WAL/SHM only while closed, fsync the
files/directory, then reopen, migrate, clear copied Francis ownership and revoke
sessions. #32 owns interruption recovery, the archive schema and practical
backup/restore tests; none is claimed delivered by this milestone.

## Test, deployment and load acceptance

Replace `backend/internal/testpg` with a temp-file SQLite helper, remove the
`postgres_integration` tags and PG-DSN skips, and execute persistence tests by
default with no database service, Docker socket or agent. At the inspected base,
60 of 229 backend test files are PG-tagged and 44 import `testpg`; migrate their
behaviour assertions rather than dropping them to reach green. Also replace the
untagged Francis PG-conditional test. Each test owns its directory and closes
every pool; service concurrency cases share the configured one-connection pool.

Required checks include all row-lock invariants above, gate admission/freshness,
cancelled-worker logging, concurrent completion/progress/repair and receipt
replay. Use real child processes for double start, crash lock release and restart
repair; two services in one process do not test the file lock. Opening and closing
a second lock-file descriptor in the holder must still leave a child refused
with `hub.already_running`. Test `admin reset-password` refusing a live hub and
succeeding under the lock after shutdown. A blocked export writer must not hold
the pool connection or prevent concurrent completion within its deadline.
Replace a pool connection to prove FK/busy/synchronous and hub time setup; test
invalid FK writes and delete actions. Cover fresh migration, reopen, forward upgrade,
newer/foreign/corrupt schema rejection, timestamp sorting/boundaries and multi-line batch order, UPSERT,
JSON, WAL recovery/checkpoints and unwritable/full-storage errors. Give critical
guards targeted counterexamples; the existing run-gate matrix still applies.

Remove the PostgreSQL service, PG env variables and required PG job from
`.github/workflows/checks.yml` only with equivalent SQLite coverage in default
checks. Preserve Docker isolation and fail-closed aggregate checks. Run lint,
tests and production builds with CGO off, including hub, agent, CLI and web;
check both binary dependency graphs and extracted licence bundles. A standalone
SQLite executable is not an image requirement.

Update `README.md`, `CONTRIBUTING.md`, runtime identity/file ownership,
`.dockerignore`, `.env.example` and deploy documentation with the supported
local volume, file layout, reset boundary and stop/start procedure. The inspected
main tree has no `deploy/` directory. Step 2 supplies a minimal pinned Go hub
runtime image and Compose template under `deploy/hub/`, with the local named
data volume and existing health/UI packaging contracts. If reusing #32's template,
remove its PostgreSQL service/client-image assumptions; do not merge its backup
engine or advertise backup as available. Keep overridable pinned image references
and the local build fallback. Production secrets remain local.

The deploy README must document offline password recovery on the same data volume,
using the template's `hub` service and binary entrypoint:

```sh
docker compose stop hub
docker compose run --rm -e ALLOW_CLI_PASSWORD_RESET=true hub admin reset-password --username example-admin
docker compose up -d hub
```

Keep the password in the terminal prompt. `docker exec` against a running hub
must fail with `hub.already_running`; never bypass the lock for recovery.

Measure the final candidate on local storage. The heartbeat path changes a row
only after 30 seconds, so four agents yield approximately `4/30 = 0.13` changed
rows/second from that path, not a bound on total SQL calls. Francis polls every
second and activity output batches at most 32 lines; these are source-derived
observations, not benchmark results. Use the service layer with simulated agent
responses for 15 minutes: four heartbeats every five seconds, three concurrent
interactive users each issuing one retained write/second, four concurrent runs
each emitting ten output lines/second, and live status/diagnostics reads. Include
same-scope admission bursts and verify final row/message counts.

| Measurement | Candidate acceptance target |
| --- | --- |
| Write operation latency, including pool wait/retry, measured per completed service call | p95 at most 250 ms; report sample count and p99 too |
| SQLite contention errors observed during the workload | Zero `SQLITE_BUSY` and `SQLITE_LOCKED` errors; inspect existing retry logs if any occur |
| WAL bytes sampled every second for each file | Peak at most 64 MiB per file; after writers/readers drain, a passive checkpoint reports no blocked frames and a truncate checkpoint leaves no retained WAL frames |
| Correctness | No missing/duplicate committed writes, final messages or workers; no negative durations; all scope/receipt constraints hold |

These are acceptance targets selected for this workload, not existing
measurements or general SQLite capacity claims. Record candidate SHA, hardware,
filesystem, driver/SQLite versions, page size, initial/final DB sizes and workload
parameters with the result. Failure requires finding the transaction/reader
cause and rerunning; do not silently loosen the thresholds or add a larger pool.
Practical acceptance on the exact approved package additionally checks empty
install, persistent restart, second-hub refusal, `/api/health` and the served web
bundle. This docs step does not perform that deployment.

## Implementation step 2

There is one large implementation step onto `feature/sqlite-hub`, followed by
the milestone closing PR to main. Keep it as one step PR with several logical
commits; intermediate incompatible states remain local. Step 2 owns runtime,
Francis, migrations, locks, tests, deploy, CI and documentation together. No
separate schema/runtime/run-gate work is delegated against an unmerged foundation.

| Commit order | Owned changes and check before proceeding |
| --- | --- |
| 1. Driver and Francis foundation | Restore the pinned SQLite provider with provenance/licences; add the pure-Go dialector, per-file connection policy, URL/storage validation and descriptor-owned lifetime lock; remove PG provider wiring. Compile hub/agent/CLI with CGO off and prove provider state survives reopen. |
| 2. Values and fresh schema | Enforce hub driver-native microsecond binding/scanning with declared timestamp types and unchanged GORM time fields; implement JSON TEXT, untyped projection/Francis scanners, identity, semantic schema proof, provenance and path-qualified manifest transition. Prove predicates, GORM auto-times, schema constraints, foreign/newer refusal and reopen, including transition guards against feature and main bases. |
| 3. Atomic service writes and cursor lifetimes | Replace all 13 row locks, gate advisory lock and PG time SQL; map busy errors into existing retries. Audit nested pool access, held cursors and timestamp paths; page `ExportVulnerabilities` before yielding; preserve exact KV prefixes and defined search/sort semantics. Run concurrent user/role/KV/tag/run tests, blocked-export completion and gate counterexamples. |
| 4. Default persistence coverage | Port tagged/helper and Francis tests to temp files; implement process lock/crash, descriptor-close and offline-reset tests, connection replacement, deadline and WAL checks. Run the whole default suite without PG env or service, with Docker unavailable. |
| 5. Packaging and CI | Supply Docker/Compose packaging with a local named volume, Linux-only bind mounts, ownership and offline password-reset instructions; remove PG service/tools/job; refresh actual dependency inventories and packaged notices. Verify production builds, local image startup and served UI/health. |
| 6. Acceptance evidence and final docs | Run the representative load and approved practical empty-install/restart checks; reconcile all docs with final code and record results in #60. Re-run full lint/tests/build, publication, provenance and migration guards on the final head. |

Commit groups are implementation order, not separately accepted steps. If a
foundation requires coupled changes to compile, keep them together and do not
publish broken intermediate commits. Before push, run `pnpm run lint` and
`pnpm run test`; CI also runs `pnpm run build` without a DB service. Require the
database-baseline risk review from a different provider and the independent
milestone closing review under [review-workflow.md](review-workflow.md). Merge
the step to its feature branch before the closing PR; only merge to main plus
green checks, review and recorded acceptance completes #60. #32 remains separate.

## Risks and limits

Output-heavy jobs can queue interactive writes; load latency, busy counts and
WAL measurements expose that risk. A single connection can deadlock on nested
pool access or a held export cursor; transactions carry their own handle and
pages close before yielding. Hub timestamp safety depends on driver options
surviving connection replacement and correct declared types; predicate,
auto-time and computed-projection tests enforce it. SQLite affinity and NULL
uniqueness can weaken constraints unless tested behaviourally. Francis restoration and the actual compiled licence graph need
their own proof. Local-file locks do not support network volumes or multiple
hubs; a later scale change requires another architecture decision. Future
two-file restore can fail between swaps and therefore needs #32's marker and
recovery design. No old-data compatibility or backup engine is promised here.
