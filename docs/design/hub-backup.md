# Hub database backup and restore

## Decision and boundary

Decision for #32: Docklet Next ships local PostgreSQL hub backups. The hub
creates a `pg_dump` custom-format archive on demand or from a schedule, lists
and downloads completed archives, and accepts an uploaded archive for restore
with `pg_restore`. The hub image includes PostgreSQL 18 client tools. This is
one hub and one database for a single operator and a few private users; Arcane's
adopted system-backup surface is the starting point, not a promise to retain its
Rustic recovery repository or project-file restore model. Docker volume backup
remains a separate feature and is not included in a database archive.

The database archive includes the hub's persisted users, settings, sessions,
job definitions, activity history and migration state. It does not include
environment files, signing secrets stored outside the database, Docker volumes,
project files on disk or agent state. The UI explains this scope and the need to
copy downloaded archives to independent storage. A local backup on the hub's
volume alone does not protect against loss of that host or volume. The custom
archive is not encrypted by the hub and may contain credentials; the operator
must protect exported copies.

## Storage, limits and retention

The hub writes to a dedicated persistent volume mounted at
`/var/lib/docklet/backups`; `DOCKLET_BACKUP_DIR` names that directory and has
the same default. Deployment templates mount the volume there, separate from
PostgreSQL data. The new variable enters `.env.example` in the implementation
step, not this design-only step. A private directory and regular files are
created with restrictive permissions. IDs select files through stored metadata;
request paths and upload filenames never become filesystem paths. Create in a
temporary file in the same directory, flush and atomically rename only after
`pg_dump` succeeds and the archive passes validation. Failed, interrupted and
orphaned temporary files are removed at startup. A completed archive is never
reported before its rename and metadata commit; startup reconciliation adopts
or removes a file when the rename and database write were interrupted.

Each enabled schedule has a retention count, default seven and minimum one.
After a successful scheduled backup, keep its newest `retentionCount` completed
archives and delete older completed archives for that policy, oldest first.
Manual and uploaded archives are not pruned by that policy. Failed runs have no
downloadable file. A retention failure leaves the new backup intact and marks
the job partial; the next run retries pruning. Deleting a backup removes both
its file and metadata with idempotent reconciliation after a crash. The UI
shows local disk usage and the configured retention count.

One archive or upload is limited to 2 GiB. Backup, upload and restore each have
a 60-minute deadline; exceeding either limit fails the run, closes the stream
and removes the incomplete file. The server enforces limits while streaming,
without buffering the archive in memory. Before creation or upload it checks
available space for the limit plus a small working margin; an actual full disk
still fails cleanly. These are explicit product limits, not a guarantee that
every database under 2 GiB will compress below the limit.

## Runs, restore and failure

Manual creation, scheduled creation, upload, deletion and restore use the M3
hub run machinery at environment `0`, under one `system-backup` gate scope.
The operation ID and backup ID form the gate key; a different active operation
gets `run.busy`, and the same active request follows M3 join/cancelling rules.
The UI reads activity states (`running`, `success`, `failed`, `cancelled`),
steps and a final backup record. Scheduled work retains its job coordinator;
its activity is linked to the same backup record. No backup operation silently
resumes after a crash: startup repairs its activity to `failed` and reconciles
temporary and completed files before new admission. Use the existing 256 KiB
per-run output cap, but persist only bounded, redacted diagnostics and coded
steps, never raw command output or connection strings. Successful `pg_dump`
does not imply an independently verified restore; the integration round-trip
provides that verification.

Restore is global-admin only and requires a fresh confirmation in the UI that
names the archive, creation time and data replacement. The server rechecks
permission and archive ownership at execution. Both a listed archive and an
uploaded archive use the same validation: custom-format signature, readable
`pg_restore --list`, PostgreSQL source server major version 18, current target
server major version 18, archive size, and a digest computed while storing it.
The archive must have a migration version supported by this hub build. A
nonempty target must have the same migration version; an empty target is
allowed for the documented full restore. A newer or older nonempty schema is
rejected before maintenance mode. The hub does not
attempt to restore an Arcane or earlier Docklet database. Archive validation
cannot prove that SQL inside a custom archive is benign; only an operator who
trusts the archive source may restore it.

After validation, the hub enters maintenance mode: it stops scheduler intake,
rejects new mutating requests and logins, cancels and drains local work, and
closes ordinary database connections. The dedicated single-hub lock connection
remains open. Active browser and API sessions are interrupted; the UI asks for
a fresh login after completion. Restore runs `pg_restore --clean --if-exists
--no-owner --no-acl --single-transaction --exit-on-error` against the existing
database, with a fixed argument vector and connection credentials passed
outside logged arguments. No `--create` or database drop is used, so the
single-hub lock survives. Restore never targets a database with another hub.
The hub then reopens its pool, verifies migrations and health, invalidates
in-memory auth/session caches, and restarts the scheduler before leaving
maintenance mode. Restored session rows must be revoked before new login;
tokens issued before restore must not authenticate against restored rows.
This includes the session that requested restore. The UI reports completion
only after the hub is ready and requires reauthentication.

A `pg_restore` error rolls back its single transaction; the hub verifies the
original database is still healthy before resuming. If rollback, pool reopening,
session revocation or health verification cannot be established, maintenance
mode persists and the process exits unready rather than serving uncertain data.
The operator must recover from an external copy or a known good archive. A hub
crash during restore cannot claim success: startup checks a durable restore
marker and database health before allowing traffic, and either establishes a
completed restore or stays unready for operator recovery. The marker must live
outside the database being restored. Cancellation is accepted before the
`pg_restore` transaction begins; once replacement starts, the UI cannot claim
that cancellation reversed it. A restore failure remains visible as a coded,
sanitized outcome after recovery.

The hub does not create an automatic safety archive before restore. The UI
requires an existing independently downloaded archive or explicit acceptance
that current data can be replaced. This avoids claiming protection that a
same-volume pre-restore archive cannot provide.

An empty replacement database has no admin session with which to use the UI.
For disaster recovery, an offline hub-image restore command accepts an archive
from the mounted backup directory, uses the same validation and transactional
restore path, and runs before HTTP and scheduler startup. It does not require
the source hub's metadata rows. The UI path serves an already initialized hub;
the offline path is how practical acceptance restores into a truly empty
database. Both paths revoke restored sessions before the hub becomes ready.

## Security and adopted surface

All hub-backup endpoints require a global admin. Existing
`system-backups:read`, `system-backups:manage` and `system-backups:restore`
permissions retain their read, create/delete/policy and restore distinctions;
the restore permission also guards upload. Capability descriptors disable
actions during maintenance or when client tools/storage are unavailable.
Upload uses a bounded stream to a non-executable private file; it rejects
oversize, malformed, wrong-major and digest-mismatched archives. Responses and
logs contain IDs and stable reason codes, not archive SQL, passwords, keys,
connection URLs, dump stderr or uploaded filenames. Downloads set attachment
headers and no-store caching and stream from a server-selected ID.

| Adopted surface | Decision |
| --- | --- |
| `GET/POST /backups`, `DELETE /backups/{id}`, `GET/PUT /backups/policies`, `GET /backups/history` | Reuse for list, create, delete, schedule and history; make DTOs describe PostgreSQL archives and M3 activity IDs. |
| `POST /backups/{id}/restore` | Reuse for a listed archive; make it an asynchronous gated restore with maintenance status. |
| Download and upload | Add ID-based download and bounded upload routes; update checked-in OpenAPI and generated TypeScript types in the shared-contract owner step. |
| Recovery-key generate/set, `recoveryKey` request fields, Rustic snapshot IDs, file browse and restore-files routes | Remove from the hub database-backup contract and UI; they represent the old encrypted repository and project-file model. Do not remove volume-backup encryption or its recovery-key dependency without a separate volume-backup decision. |
| `system_backup_runs`, `system_backup_policies`, `system-backup:` scheduled job | Reuse the records and job identity where safe; migrate archive metadata and prune obsolete system-only fields with a forward migration. The scheduler retains skipped/partial outcomes. |
| `systemBackupsCategory` placeholder in backend settings | Replace Arcane recovery wording with a Hub database-backup entry; keep the policy under the backup endpoints, not a duplicate generic setting. |
| `web/src/features/volumes/BackupsPage.tsx` and `BackupTargetsSection.tsx` | Keep their volume-backup role. Add a separate Hub database-backup page with list, download, upload, restore, schedule and localized English/German text. |

The current `SupportedDatabaseProvider() == false` and PostgreSQL
`ErrSystemBackupUnavailable` paths are replaced only when their operation is
implemented. No route should advertise a working backup while it still returns
the stub. The existing system-managed volume-backup routes and UI remain
separate; shared code removed from the system surface must preserve them.

## Verification and delivery steps

Unit tests run without PostgreSQL, Docker or a running agent. They cover policy
retention and manual/archive exclusions, stream size and timeout enforcement,
filename/ID isolation, archive-major and migration rejection, gate conflicts,
restart reconciliation, sanitized errors, maintenance admission and session
revocation. A PostgreSQL 18 integration test in CI uses a throwaway database
container: seed representative users, settings, jobs and activity, create an
archive, empty the database, restore the archive, restart hub wiring, and compare
the same data and migration state. It also injects a restore error and verifies
transaction rollback and readiness handling. Practical acceptance repeats a
UI-created backup to empty database to restored hub with the same data; local
tests alone do not close the milestone.

1. **Shared contract owner.** Update `types/backup/`,
   `backend/internal/system/{model.go,children/backup/{module.go,handler.go}}`,
   the next PostgreSQL migration, `api/openapi.json`, generated
   `web/src/api/generated/schema.ts` and contract tests. Define archive,
   upload/download, activity and maintenance DTOs, remove system-only recovery
   fields/routes, and retain volume contracts. Acceptance: generated contract
   matches routes and migration tests pass. Merge this step into
   `feature/hub-backup` before dependent work.
2. **Backup engine and schedule.** Implement bounded `pg_dump` and storage in
   `backend/internal/system/children/backup/`, policy retention and job
   reconciliation, `backend/internal/config/`, hub image/build files and
   `.env.example`; add unit tests and the backup half of the PostgreSQL
   integration test. Acceptance: manual and scheduled archives are listable and
   downloadable, retention and crash cleanup work, PG 18 tools are in the image.
3. **Restore and maintenance.** Add upload validation, `pg_restore`, durable
   restore marker, maintenance admission, session revocation, offline recovery
   entrypoint and scheduler drain in `backend/internal/system/children/backup/`, auth/bootstrap wiring
   and relevant tests. Acceptance: the CI round-trip restores the same data;
   incompatible uploads and failures leave the hub safe. This is a risk step
   requiring an independent interim review under `review-workflow.md`.
4. **Hub UI and acceptance.** Add the Hub backup page, adapter mapping and
   `web/src/features/*/messages/{en,de}.ts` catalogues; update deployment
   documentation and UI/adapter tests. Acceptance: an admin can create, list,
   download, upload and restore through the UI, configure the schedule, see
   actionable states, and complete the documented practical round-trip.

## Maintainer choice

Should the first release keep backups only on the hub's local volume, with an
operator downloading copies elsewhere, or must it also send copies directly to
separate storage? This plan uses local storage and download. Direct remote
delivery would require its own destination and credential design.
