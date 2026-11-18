# Docklet Next

Docker and Compose management for a small number of hosts, with a Go hub and
agent core and a React interface. Built for one operator and a few users.

**Status: 1.0.0, the first product release.** Release images target linux/amd64.
The hub uses SQLite on local persistent storage; no database service or import of
earlier docklet-hub, Arcane or pre-release data is supported.

## Features

- **Stacks and containers:** create, edit, import and deploy Compose stacks; start,
  stop and restart stacks and containers and redeploy stacks as durable action
  runs; edit and recreate standalone containers; live logs and an interactive
  container shell.
- **Hosts:** the local hub host plus Direct agents (the hub connects) and Edge
  agents (the agent tunnels out), with onboarding packages, host maintenance
  (edit, check, reconnect, restart, remove) and in-app hub and agent updates.
- **Images and updates:** image inventory, update checks, vulnerability scans
  through the pinned tools image, update policies and schedules with recovery of
  interrupted runs.
- **Storage and networks:** volumes, bind mounts, networks, encrypted volume
  backups and restore, and coordinated hub database backups.
- **Operations:** dashboard with host history and service widgets, activity
  center, notifications with named targets, Auto Heal, global search and bulk
  actions.
- **Access:** local, proxy or no sign-in mode, a global administrator, simple
  users with a short allowlist of rights, and API keys.
- **Optional modules:** proxy route management for existing Traefik and Caddy
  installations, switched on hub-wide in Settings.

## Quick start

The [release package](docs/install.md) is the supported installation: its
Compose file pins both images by version and digest and forwards every sign-in
setting. For a first look, this example keeps everything of Docklet in one
directory and manages the stacks next to it:

```text
/srv/docker/            projects directory: one subdirectory per stack
/srv/docker/docklet/    Docklet itself: compose.yaml, .env, data/, backups/
```

Any local directory works; replace `/srv/docker` everywhere below if you use another
one.

Create `/srv/docker/docklet/compose.yaml`:

```yaml
name: docklet
services:
  hub:
    image: ghcr.io/florianfinn/docklet-next-hub:1.0.0
    restart: unless-stopped
    stop_grace_period: 30s
    labels:
      io.docklet.system: hub
    ports:
      - "127.0.0.1:3552:3552"
    environment:
      APP_URL: ${APP_URL:?Set APP_URL in .env}
      ENCRYPTION_KEY: ${ENCRYPTION_KEY:?Set ENCRYPTION_KEY in .env}
      PUID: ${PUID:?Set PUID in .env}
      PGID: ${PGID:?Set PGID in .env}
      TZ: ${TZ:-UTC}
      AUTH_MODE: local
      PROJECTS_DIRECTORY: /srv/docker
      # Fixed container paths; leave them as they are.
      DATABASE_URL: file:/app/data/docklet.db
      DOCKLET_BACKUP_DIR: /var/lib/docklet/backups
      DOCKER_CONFIG: /app/data/.docker
    volumes:
      - ./data:/app/data
      - ./backups:/var/lib/docklet/backups
      - /srv/docker:/srv/docker
      - /var/run/docker.sock:/var/run/docker.sock
```

Create `/srv/docker/docklet/.env` beside it:

```dotenv
APP_URL=https://docklet.example.com
PUID=1000
PGID=1000
TZ=Europe/Berlin
```

Then add a new encryption key and start the hub:

```sh
cd /srv/docker/docklet
echo "ENCRYPTION_KEY=$(openssl rand -hex 32)" >> .env
chmod 600 .env
docker compose up -d
docker compose logs hub
```

| Variable | Required | Meaning |
| --- | --- | --- |
| `APP_URL` | yes | Public origin of the hub, the root of a dedicated domain or subdomain. Links, sign-in and agent connections use it. |
| `ENCRYPTION_KEY` | yes | Encrypts stored credentials (registry logins, backup targets and their keys) and secures sign-in sessions. Keep a copy with your backups: without the original key a restored hub cannot read those credentials, and changing it later makes them unreadable. |
| `PUID`, `PGID` | yes | User and group the hub runs as. Use the owner of the projects directory (`stat -c '%u %g' /srv/docker`), so Docklet can create and edit stacks there. The hub hands `data/` and `backups/` to this user at startup. |
| `TZ` | no | IANA time zone for schedules and dates, default `UTC`. |
| `AUTH_MODE` | no | Sign-in mode: `local` (passwords, default), `proxy` (a trusted reverse proxy authenticates) or `none`. See the [installation guide](docs/install.md#select-a-sign-in-mode). |
| `PROJECTS_DIRECTORY` | no | Where stacks live inside the container. Mount it at the same path as on the host, as above, so relative paths in your stacks resolve correctly. |
| `DATABASE_URL`, `DOCKLET_BACKUP_DIR`, `DOCKER_CONFIG` | fixed | Database file, hub backup archives and registry login data inside the mounted `data/` and `backups/` directories. |

All other settings, such as proxy sign-in, trusted proxies or image overrides, are
described in [`.env.example`](.env.example). Docklet keeps its own settings in the
database under `data/`; there is no separate configuration directory. Keep `data/`
on a local disk, not on a network share.

Serve the hub through a TLS reverse proxy that forwards WebSockets to
`127.0.0.1:3552` (see the [reverse-proxy guide](docs/deployment/reverse-proxy.md)).
Sign in as `arcane` with the initial password from `docker compose logs hub`,
change it when prompted and complete setup. The `io.docklet.system: hub` label
marks this directory as Docklet's own, so it never appears as a stack, and Docklet
hides its own containers. Then use **Hosts** › **Add host**; the hub generates
the agent installation package. Before production use, switch to the release
package for digest pins, proxy sign-in and the documented upgrade path.

## Install

Use the [installation guide](docs/install.md) with a package from
[GitHub Releases](https://github.com/florianfinn/docklet-next/releases), then add
agents through the hub. Read [upgrading and recovery](docs/upgrade.md) and
[compatibility](docs/compatibility.md) before changing versions. The
[reverse-proxy guide](docs/deployment/reverse-proxy.md) covers the required
dedicated domain or subdomain root. The [source-build guide](deploy/hub/README.md)
provides a local-image fallback; [release packages](docs/release.md) describes
how packages are built and verified.

## Licence

Based on Arcane. The core was adopted once and is maintained independently. Own
code is licensed under [Apache-2.0](LICENSE); Arcane-derived parts retain
[BSD-3-Clause](LICENSES/BSD-3-Clause-Arcane.txt). See [NOTICE](NOTICE) and
[licensing](docs/design/licensing.md) for the split and distribution duties.

## Contributing and security

Start with [CONTRIBUTING.md](CONTRIBUTING.md), the binding [AGENTS.md](AGENTS.md)
and the [design index](docs/design/README.md); acceptance records are in the
[history index](docs/history/README.md). Current work belongs in
[GitHub issues](https://github.com/florianfinn/docklet-next/issues) and
[milestones](https://github.com/florianfinn/docklet-next/milestones). Report
vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

For agent collaboration, follow [Agent coordination](AGENTS.md#agent-coordination)
and [Branches, review and completion](AGENTS.md#branches-review-and-completion).
The separately maintained
[subagent-orchestration skill](https://github.com/florianfinn/claude-skills/tree/master/plugins/subagent-orchestration)
is an optional general reference; project instructions take precedence.
