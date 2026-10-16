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
refuses detected network storage or filesystem detection errors. Hub backups
are described below; copies to network archive targets are a later feature.

The hub owns `docklet.db`, `docklet.francis.db`, each file's SQLite-managed `-wal`
and `-shm` companions, and the stable `docklet.db.lock`. The data directory is
private (`0700`); database and lock files are `0600`. The runtime identity needs
creation and rename rights. Allow space for both databases and their WAL files;
backup archives live on the separate `hub-backups` volume. WAL autocheckpointing is enabled. Never
remove a live WAL/SHM file, replace the database beneath a running hub or copy
only a live main database file as a backup.

Start with `docker compose up -d hub`; inspect health with `docker compose ps`.
Stop with `docker compose stop hub` before moving or resetting storage. A clean
restart reopens the same files and runs forward migrations. After a crash SQLite
recovers its WAL and startup repairs abandoned runs; an accepted remote action
may still have finished. A second hub refuses startup with `hub.already_running`.
No previous PostgreSQL, Arcane or Docklet database/registration is imported. Begin
with an empty volume; unsupported or newer schemas are refused.

## Sign-in and recovery

`AUTH_MODE=local` is the default and uses the local administrator password and
sessions. `AUTH_MODE=none` grants full administrator access to anyone who can reach
the hub; the operator deliberately accepts that access. Switching away from none
revokes sessions but preserves users, API keys, environment tokens and webhook
tokens created meanwhile. Review users and revoke or rotate unwanted credentials.
The hub does not allowlist the request Host header; none-mode reads remain
unauthenticated, including requests using another Host. Browser write and WebSocket
Origin checks do not add Host validation for reads.

`AUTH_MODE=proxy` accepts
the identity in `AUTH_PROXY_HEADER` (default `Remote-User`) only from the direct TCP
peer addresses or CIDRs in `AUTH_TRUSTED_PROXIES`. The list is comma-separated and
has no default; missing/malformed configuration and all-address `/0` networks
prevent startup. Identity values must contain printable UTF-8 characters only.

Every accepted proxy name maps to the same administrator; it is used only for
activity attribution. Configure the proxy to authenticate every request and strip
and replace the identity header. Trust the address seen by the hub after any
container networking or address translation. For example, a documentation-only
policy can list `192.0.2.10,2001:db8::/32`. Forwarded addresses, including
`X-Forwarded-For`, never establish sign-in identity. `TRUSTED_PROXIES` separately
controls forwarded client/TLS metadata and does not grant identity trust. Local
passwords, sessions and personal API keys cannot bypass proxy sign-in.

Put these values in the private Compose environment file and recreate the hub
with `docker compose up -d --force-recreate hub` using the same `--env-file` as for
installation. Merely restarting a container does not update its environment.
Changing modes revokes previous sessions; a same-mode restart preserves them.
`GET /api/auth/mode` exposes the active mode without credentials. The UI cannot
change it. No web sign-in screens are introduced by this backend step.

For recovery, set `AUTH_RESET_ADMIN_PASSWORD` in that private environment file to
a new password satisfying the configured password policy, then recreate the hub.
Startup updates the global administrator password and revokes its sessions once
per start. The log records that a reset occurred without printing the password.
Remove the variable after successful recovery and recreate the hub again; leaving
it set repeats the reset on the next start. Alternatively select `AUTH_MODE=none`,
restart, and restore the intended mode when operator work is complete. There is
no password-reset CLI.

`AUTH_MODE`, `AUTH_PROXY_HEADER`, `AUTH_TRUSTED_PROXIES` and
`AUTH_RESET_ADMIN_PASSWORD` all support `_FILE` and `__FILE` variants. Mount the
referenced files into the hub container and set their container paths. `__FILE`
has precedence over `_FILE`, then the direct value; the existing loader warns
and falls back to the direct value if a file cannot be read. Empty direct mode
values use `local`. An invalid recovery value prevents startup.

For every hub mode, fetch `GET /api/auth/mode`, retain the returned CSRF cookie
and `X-CSRF-Token` header, then send that token header plus the exact `APP_URL`
Origin on every POST, PUT, PATCH or DELETE. Missing/wrong Origin or CSRF tokens
return 403, including requests carrying local Bearer/API-key or proxy identity
headers. Machine transports authenticated by environment/agent tokens and
path-token webhook ingress retain their separate authentication contracts.

For a local candidate check, run `node scripts/smoke-hub-image.mjs IMAGE` after
building. It checks an empty temporary volume, `/api/health`, the served web
bundle, second-hub refusal, persistent restart and every extracted licence hash.
It removes only its own labelled containers and volume. This automated smoke does
not replace operator approval and practical acceptance of a release package.

## Database archives and recovery

The separate `hub-backups` volume holds unencrypted `<backup-id>.tar` archives of
both SQLite files. Backups pause database requests and live views briefly; a
restore revokes all sessions. Keep a protected downloaded copy and the external
encryption key away from the Hub host. Volume data, project files and external
secrets are not in these archives. Mount/SMB target copying is a follow-up.

For an empty replacement data volume, stop the hub, place a trusted archive under
its server-owned filename in the backup volume, and run:

```sh
docker compose stop hub
docker compose run --rm hub restore-hub --backup-id example-archive
docker compose up -d hub
```

The offline command takes the lifetime lock and refuses a running hub. It
validates both images, retains safety copies beside the databases, replaces the
pair, migrates forward and revokes restored sessions before readiness. A durable
restore marker causes startup to repeat an interrupted installation from protected
inputs. Keep the marker, recovery directory and safety files if startup refuses
recovery; do not delete them to bypass validation. Safety copies and staged images
consume local space and are retained for operator removal after verification.

The UI round-trip acceptance waits for M4 sign-in; a browser check with a synthetic
session is implementation evidence only. See the indexed backup design and
practical acceptance procedure.
