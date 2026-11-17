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
setting. For a first look, this minimal `compose.yaml` runs the hub with local
sign-in:

```yaml
name: docklet-next
services:
  hub:
    image: ghcr.io/florianfinn/docklet-next-hub:1.0.0
    restart: unless-stopped
    stop_grace_period: 30s
    ports:
      - "127.0.0.1:3552:3552"
    environment:
      TZ: ${TZ:-UTC}
      APP_URL: ${APP_URL:?Set APP_URL to the public origin}
      AUTH_MODE: local
      ENCRYPTION_KEY: ${ENCRYPTION_KEY:?Set ENCRYPTION_KEY in .env}
      DATABASE_URL: file:/app/data/docklet.db
      DOCKLET_BACKUP_DIR: /var/lib/docklet/backups
      PROJECTS_DIRECTORY: /app/data/projects
      DOCKER_CONFIG: /app/data/.docker
    volumes:
      - hub-data:/app/data
      - hub-backups:/var/lib/docklet/backups
      - /var/run/docker.sock:/var/run/docker.sock
volumes:
  hub-data: {}
  hub-backups: {}
```

Put the private values in a `.env` file beside it and start the hub:

```sh
printf 'APP_URL=https://docklet.example.com\nENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)" > .env
chmod 600 .env
docker compose up -d
docker compose logs hub
```

Keep `ENCRYPTION_KEY` with your protected backups; the hub cannot read its
stored secrets without it. Serve the hub at the root of the domain in `APP_URL`
through a TLS reverse proxy that forwards WebSockets to `127.0.0.1:3552` (see the
[reverse-proxy guide](docs/deployment/reverse-proxy.md)). Sign in as `arcane`
with the initial password from the startup log, change it when prompted and
complete setup. Then use **Hosts** › **Add host**; the hub generates the
agent installation package.

The hub database must stay on local storage, not on a network share. Before
production use, switch to the release package for digest pins, proxy sign-in and
the documented upgrade path.

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
