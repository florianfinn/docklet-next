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
