# Docklet Next

Docker and Compose management for a small number of hosts, with a Go hub and
agent core and a React interface. Built for one operator and a few users.

**Status: 1.7.0.** A fresh or upgraded hub opens first-run setup; complete it right after the first start. Release images target linux/amd64.
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

Any local directory works: change the lines marked `# change` and the values in
`.env`.

Create `/srv/docker/docklet/compose.yaml`:

```yaml
name: docklet
services:
  hub:
    # "Update Docklet" in the UI sets HUB_IMAGE in .env (in-place update).
    image: ${HUB_IMAGE:-ghcr.io/florianfinn/docklet-next-hub:1.7.0}
    restart: unless-stopped
    stop_grace_period: 30s
    labels:
      io.docklet.system: hub # self-protection: Docklet hides its own stack
    ports:
      - "127.0.0.1:3552:3552" # local reverse proxy; "3552:3552" for direct access
    environment:
      APP_URL: ${APP_URL:?Set APP_URL in .env}
      ENCRYPTION_KEY: ${ENCRYPTION_KEY:?Set ENCRYPTION_KEY in .env}
      PUID: ${PUID:?Set PUID in .env}
      PGID: ${PGID:?Set PGID in .env}
      TZ: ${TZ:-UTC}
      PROJECTS_DIRECTORY: /srv/docker # change
    volumes:
      - ./data:/app/data
      - ./backups:/var/lib/docklet/backups
      - /srv/docker:/srv/docker # change
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
| `HUB_IMAGE` | no | Set by the in-app update; leave it out of `.env` at first. |
| `TZ` | no | IANA time zone for schedules and dates, default `UTC`. |
| `PROJECTS_DIRECTORY` | no | Where stacks live inside the container. Mount it at the same path as on the host, as above, so relative paths in your stacks resolve correctly. |
| `DATABASE_URL`, `DOCKLET_BACKUP_DIR`, `DOCKER_CONFIG` | no | Database file, hub backup archives and registry login data. The image already points them into the mounted `/app/data` and `/var/lib/docklet/backups`; the volume mounts are enough. Set them only to move these files. Exception: with `PUID=0` and `PGID=0`, add `DOCKER_CONFIG: /app/data/.docker` under `environment`. |

All other settings, such as proxy sign-in, trusted proxies or image overrides, are
described in [`.env.example`](.env.example). Docklet keeps its own settings in the
database under `data/`; there is no separate configuration directory. Keep `data/`
on a local disk, not on a network share.

Serve the hub through a TLS reverse proxy that forwards WebSockets to
`127.0.0.1:3552` (see the [reverse-proxy guide](docs/deployment/reverse-proxy.md)).
Without a proxy on this host, publish `3552:3552` and set `APP_URL` to the address
you open in the browser.
Open the hub and complete setup right away, before others can reach it:
choose how to sign in (local accounts, a trusted reverse proxy, or no sign-in) and
create your administrator account. Docklet hides its own directory and
containers, as described at the label above. Then use **Hosts** › **Add host**;
the hub generates the agent installation package. Before production use, switch to the release
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
