<!--
Source: docklet-next/docs/design/sqlite-database.md
Revision: b3607be5144101152c8eb7738d389e8c73986fba
Retrieved: 2026-10-16
License: Apache-2.0
Deviations: Original SQLite candidate packaging and offline recovery instructions.
-->

# Hub candidate package

Build from the repository root with `docker compose -f deploy/hub/compose.yaml build`.
The template builds a CGO-free hub with its production web bundle and complete
Go/web licence evidence. The assessed package target is Linux/amd64. Images use
version and digest pins. `NODE_IMAGE`, `GO_IMAGE` and `RUNTIME_IMAGE` build arguments
can override those pins; reassess changed dependencies before distributing a package.
`HUB_PORT` overrides the loopback host port. `HUB_IMAGE` overrides the runtime image reference; the local build remains available
without access to a product registry. No product release is published by this template.

Keep operator values in an untracked private environment file. Set `ENCRYPTION_KEY`
to a newly generated secret, `APP_URL` to the dedicated domain root and optionally
`PUID`/`PGID` to the desired numeric identity. Use `--env-file` consistently for all
Compose commands. The listener is published on loopback; attach the operator's
reverse proxy to it. Sub-path hosting is unsupported. The Docker socket grants
management access to the local Docker host.

The `hub-data` named volume is mounted at `/app/data`. Keep it on local persistent
storage. Linux bind mounts are supported when their directory is writable by the
runtime identity; use the named volume on Docker Desktop and other hosts. Named
volumes backed by SMB/NFS/FUSE are also unsupported for database files. Startup
refuses detected network storage or filesystem detection errors. Backups may
later use network archive targets; the hub backup engine is a separate feature.

The hub owns `docklet.db`, `docklet.francis.db`, each file's SQLite-managed `-wal`
and `-shm` companions, and the stable `docklet.db.lock`. The data directory is
private (`0700`); database and lock files are `0600`. The runtime identity needs
creation and rename rights. Allow space for both databases, their WAL files and
future independent backup archives. WAL autocheckpointing is enabled. Never
remove a live WAL/SHM file, replace the database beneath a running hub or copy
only a live main database file as a backup.

Start with `docker compose up -d hub`; inspect health with `docker compose ps`.
Stop with `docker compose stop hub` before moving or resetting storage. A clean
restart reopens the same files and runs forward migrations. After a crash SQLite
recovers its WAL and startup repairs abandoned runs; an accepted remote action
may still have finished. A second hub refuses startup with `hub.already_running`.
No previous PostgreSQL, Arcane or Docklet database/registration is imported. Begin
with an empty volume; unsupported or newer schemas are refused.

Offline password recovery uses the same volume and runtime identity:

```sh
docker compose stop hub
docker compose run --rm -e ALLOW_CLI_PASSWORD_RESET=true hub admin reset-password --username example-admin
docker compose up -d hub
```

Run these commands from this directory, or pass `-f deploy/hub/compose.yaml` from
the repository root. Supply the real administrator name. Enter the new password
only at the terminal prompt. `docker exec` against a running hub is refused with
`hub.already_running`; the enable flag does not bypass the lifetime lock. Recovery
revokes other sessions. Creating a missing data directory also uses `PUID`/`PGID`.

For a local candidate check, run `node scripts/smoke-hub-image.mjs IMAGE` after
building. It checks an empty temporary volume, `/api/health`, the served web
bundle, second-hub refusal, persistent restart and every extracted licence hash.
It removes only its own labelled containers and volume. This automated smoke does
not replace operator approval and practical acceptance of a release package.
