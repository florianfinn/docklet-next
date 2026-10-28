# Hub database backup and restore

## Decision and boundary

Decision for #32: back up the SQLite Hub database and its adjacent Francis actor
store as one coordinated pair. SQLite is the only supported engine, as specified
in [SQLite hub database](sqlite-database.md). Old development PostgreSQL archives
and earlier Docklet/Arcane databases are not imported. No PostgreSQL executable,
client package, database server or SQLite CLI is needed by this feature.

The archive includes persisted users, settings, sessions, job definitions, actor
state and activity history. It excludes environment files, external signing and
encryption secrets, Docker volumes, project files and agent state. Restoring
credentials encrypted in the database requires the original external encryption
key. The operator must protect archives: they are unencrypted and checksums prove
internal consistency, not source authenticity. Restore only trusted archives.

The archive is unencrypted. Target credentials, target keys and a configured local
recovery key stored inside it require the original encryption key after restore.
Export repository recovery keys separately; those exports neither encrypt the
Hub archive nor replace the key needed to decrypt its stored fields.

Local archives and downloads are the recovery path. Copying to mount/SMB
volume-backup targets is a follow-up owned by the volume-backup transfer feature.
That interface is not delivered; target IDs are rejected and target controls stay
hidden. No second destination model or credential store is introduced. The
existing volume-backup repository and its recovery key remain separate.

## Coordinated snapshot

Manual and scheduled creation use environment `0` and the `system-backup` activity
gate. Begin one maintenance barrier before either image: reject new database
requests, pause coordinator intake, cancel and drain other local workers, and
close browser streams/edge tunnels. A scheduled creator is excluded from its own
worker drain; all neighbouring workers and background coordinator repair drain.
Creation does not revoke sessions. Maintenance status identifies `backup` versus
`restore`, so the UI returns to the list after creation and asks for login only
after restore. Scheduling remains Off/Daily/Weekly with a time in the Hub's local
time zone. Unsupported existing cron expressions stay visible and unchanged until
explicitly replaced.

Reserve the sole Hub writer connection, then the sole Francis writer connection
before taking either image. Start their shared 30-second deadline before waiting
for the Hub reservation; it also covers Francis reservation, image sizing and both
`VACUUM INTO` operations. Existing transactions have then finished and no
actor, lease renewal, timer or request can write either file until release.
Execute `VACUUM INTO ?` on each reserved connection to separate new private files.
This includes committed WAL data and does not copy a live database file through
ordinary file descriptors. Keep the lifetime Hub file lock throughout. Release
both reservations and resume admission after both images are durable, before
hashing, validation and tar streaming. Bound the reserved-connection snapshot
phase, including writer reservation, to 30 seconds so it cannot outlive Francis
host health/lease deadlines. The overall operation retains its 60-minute limit.
Release on failure too. A failure to drain never publishes an archive.

Resolve symlinks in the database and archive paths and reject equal directories
or nesting in either direction. Storage status and startup reconciliation apply
the same boundary before creating or scanning archives.

Validate both images using read-only immutable SQLite connections. Require one
`integrity_check` result `ok`, no `foreign_key_check` rows, the Docklet database
identity and consecutive supported Goose migration history, and the expected
Francis namespace and migration level. Both images must pass before publication.

## Archive version 1

The server-owned filename is `<backup-id>.tar`. IDs contain only ASCII letters,
digits, `_` and `-`, with at most 128 characters. Upload filenames never select a
path. The uncompressed tar has exactly three regular members in this order:

1. `manifest.json`, at most 16 KiB, with format `docklet-sqlite`, version `1`,
   `snapshotId`, `hubSchema`, `francisSchema` and an `images` array.
2. `hub.sqlite`, described by the first image entry.
3. `francis.sqlite`, described by the second image entry.

Both images carry one archive-only `docklet_backup_image` row with the same
random snapshot UUID and their fixed role; the manifest must match. A valid image
from another archive therefore fails even if its manifest hash is recomputed.
Remove these rows/tables during restore finalization.

Each image entry has the fixed `name`, byte `size` and lowercase hexadecimal
SHA-256 `sha256`. Francis schema level is currently `7`. The Hub version comes
from the image's applied Goose history; the manifest must match it. Reject newer
Hub/Francis schema levels, foreign identities, corrupt images, mismatched sizes
or hashes, additional members, links and supplied paths. Only zero padding may
follow the tar end marker. Only this uncompressed SQLite pair is supported.
The wire DTO replaces `sourceServerMajor` with `archiveVersion`; download and
upload remain `application/octet-stream`. This is a pre-release contract change.

Write to private staging storage, flush images and tar, re-read and validate the
finished archive, then atomically rename and fsync the archive directory. Save
completed metadata afterwards. Startup removes incomplete/orphaned archives;
it never advertises a completed archive without its committed metadata and file.
The archive directory defaults to `/var/lib/docklet/backups`, set by
`DOCKLET_BACKUP_DIR`, and is mounted separately from the live data directory.
Directories use `0700`; archive files use `0600`. Unavailable archive storage
disables backup operations while the Hub can serve its database. A pending restore
marker remains a separate fail-closed readiness boundary on the data volume.

Archives/uploads are limited to 2 GiB, with a 60-minute deadline. Streaming reads
and writes check cancellation and size; database snapshots use the SQLite context.
Working storage must also accommodate both full images and validation copies.
Disk-full or I/O failures remove incomplete archives and return coded errors.
Diagnostics contain IDs and stable codes, never raw SQLite errors, credentials,
archive SQL, connection URLs, uploaded filenames or runtime paths.

## Restore and interruption recovery

Restore requires global-admin restore permission and an explicit confirmation of
the archive ID. Recheck execution permission and validate both images before
maintenance. A supported older archive may migrate forward; a newer schema is
rejected. No downgrade runs. The HTTP path requires an initialized hub; the
offline `restore-hub --backup-id <id>` command can restore an empty data volume
without source metadata or an existing administrator session.

Enter the same maintenance barrier, drain activities and scheduler work, stop the
Francis host and its diagnostics, and close the Hub pool. Keep the stable lifetime
lock inode. Every ordinary connection must close before file replacement, safety
copying or removal of old WAL/SHM files. Explicitly checkpoint both stopped files
with `wal_checkpoint(TRUNCATE)` before safety copying; a busy result or any error
keeps live restore blocked and unready. Refuse a non-empty WAL before taking each
safety copy. The live pair is always on local storage.

Extract and revalidate both images into a private recovery directory next to the
main database. Flush these immutable inputs and safety copies of the previous
pair, then fsync that directory and its parent. Write and fsync the versioned
`<main-file>.restore/restore-state.json` marker on the data volume before replacing either file.
It records the resolved destination, archive/activity IDs, phase and both image
hashes. Runtime paths in that private marker are never API or public evidence.
Two renames are not atomic together, so the marker and persistent inputs are the
recovery authority.

For each image, remove old WAL/SHM only while closed, copy its recovery input to a
same-filesystem installation file, flush it, rename over the live file and fsync
the parent. Mark `verifying` after both replacements. Reopen under the held lock,
migrate forward, check integrity/foreign keys, clear copied Francis host and
lease ownership, fail interrupted activities and revoke every restored session.
Refresh authentication/API-key/permission/settings caches and dynamic jobs before leaving
maintenance. The requesting session is revoked too. Retain safety copies for
operator recovery; they are local safeguards, not an off-host backup.

At startup, before ordinary migrations, actors, HTTP or scheduler startup, a valid
marker acquires the same lifetime lock, validates its destination and persistent
input hashes, and repeats both installations. It always rolls forward from the
original pair without needing the archive volume, including after interruption between renames, during migration or
session revocation. Clear and fsync the marker only after verification and session
revocation succeed. A malformed marker, missing/changed recovery input, lock
conflict or verification failure leaves startup unready. The operator must retain
these files and restore a trusted archive; never delete a marker to bypass checks.
Cancellation is allowed before replacement; after that point it cannot promise
that data replacement was reversed. An online failure keeps maintenance active
and exits unready for startup recovery.

## Retention, access and acceptance

Each enabled policy retains its newest completed scheduled archives, default
seven, minimum one. Manual and uploaded archives are excluded. Prune oldest first
after successful creation. A pruning failure keeps the new archive and reports a
partial job. Deletion removes the selected file and metadata; startup reconciles
an interrupted deletion. Target retention is outside this feature.

Read, manage and restore permissions keep their separate meanings; upload uses
restore permission. Maintenance status is database-free, and during maintenance
it exposes only operation, phase and activity ID without authentication. Health
returns 503 while unready. Download uses a server-selected ID, attachment headers
and no-store caching. The production UI displays scope, unencrypted archive
handling, schedule/retention, upload, download, deletion and restore confirmation.

Default tests use temporary SQLite files without external services. They cover
WAL-inclusive creation, paired restore into an empty hub, session revocation,
interrupted replacement recovery, invalid/foreign/newer images and manifests,
upload limits, retention, schedule and admission. Production browser verification
uses the actual built UI. The practical round-trip in
[practical acceptance](practical-acceptance.md) remains a separate approval and
waits for M4 sign-in (#58); acceptance #59 stays open. This feature does not add
login, import older databases, transfer target copies or perform a deployment.
