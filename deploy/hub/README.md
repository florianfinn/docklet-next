<!--
Source: docklet-next/docs/design/hub-backup.md
Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
Retrieved: 2026-10-15
License: Apache-2.0
Deviations: Original Hub image deployment notes for issue #32.
-->

# Hub image

Build from the repository root with `docker build -f deploy/hub/Dockerfile -t docklet-next-hub:0.0.0 .`. The image includes PostgreSQL 18 `pg_dump` and `pg_restore`. The Compose example binds the HTTP port to loopback and mounts a dedicated persistent volume at `/var/lib/docklet/backups`. Supply `DATABASE_URL` and application secrets in the root `.env` file; keep that file private. The database and its data volume are separate from the Hub backup volume.

Override `DOCKLET_GO_IMAGE`, `DOCKLET_HUB_RUNTIME_IMAGE`, or `DOCKLET_HUB_IMAGE` for a mirrored registry. Keep the overridden images pinned to a version and digest. A local build is supported without a Hub image registry.

The Hub runs as the `postgres` user. Give that user write access to an existing backup volume. Each backup requires at least 2 GiB plus 64 MiB of free space before it starts; below that threshold the storage endpoint reports `backup.storage_unavailable`.

## Backups and recovery

The `hub-backups` volume is mounted at `/var/lib/docklet/backups` and must persist independently of the PostgreSQL data volume. Keep at least 2 GiB plus 64 MiB free for each creation or upload, as well as space for retained archives. The Hub reports current usage and free space on its Hub database backup page. Scheduled retention applies only to scheduled archives; manual and uploaded archives stay until an operator removes them. Download a trusted copy to storage outside the Hub host. A local volume alone does not protect against host or volume loss.

An archive contains the Hub database, including users, settings, sessions, jobs, activity and migration state. It does not contain `.env`, external signing secrets, Docker volumes, project files or agent state. Archives are not encrypted by the Hub and may contain credentials. Restrict access to the volume and downloaded copies.

For an in-place restore, a global administrator selects a completed archive on the Hub database backup page, confirms its ID and accepts replacement of the current data. The Hub enters maintenance, stops database-backed requests and sign-in, closes live browser views and edge tunnels, and signs out all users. After the restore and health checks complete, users sign in again; edge agents reconnect. Keep an independent copy before replacing data.

For an empty database or a Hub that cannot start after an interrupted restore, use offline recovery. Stop the Hub. Place a trusted PostgreSQL 18 custom-format archive in the mounted backup volume as `<backup-id>.dump`, using its ID from the downloaded filename. Keep the file private and writable by the container's `postgres` user. With the database reachable and the normal private `.env` available, run `docker compose -f deploy/hub/compose.yaml run --rm --no-deps hub restore-hub --backup-id <backup-id>` from the repository root. Then start the Hub and verify `/api/health`, sign-in and the expected data. The command validates archive format, server and migration compatibility, performs the transactional restore and revokes restored sessions. A recovery marker left by an interrupted restore keeps the Hub unready until offline recovery succeeds. Keep the archive and investigate the database if the command fails; do not remove the marker merely to start the Hub.
