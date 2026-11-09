<!--
Source: docklet-next/docs/design/sqlite-database.md
Revision: b3607be5144101152c8eb7738d389e8c73986fba
Retrieved: 2026-10-16
License: Apache-2.0
Deviations: Original SQLite candidate packaging and offline recovery instructions; source-build fallback with release installation and recovery moved to operator guides (#363).
-->

# Build the hub and agent from source

Build local images when you need a fallback without a product image registry.
The builds still need the pinned toolchain base images and dependencies. For
release installation, use [the installation guide](../../docs/install.md);
[upgrading and recovery](../../docs/upgrade.md) and
[compatibility](../../docs/compatibility.md) apply to persistent storage and hosts.
For reproducible OCI release builds, see [release packages](../../docs/release.md).

Run commands from the repository root. Use the same checkout and version for hub
and agent; `0.0.0` below is the local source-build version. The assessed image
target is Linux/amd64. The Dockerfile defaults to the `hub` target, embedding the
production React bundle and Go/web licence evidence. The separate `agent` target
contains only the agent executable and Go licence evidence and starts with
`AGENT_MODE=true`.

## Build and start the hub

Create a private, untracked `.env.source` at the repository root. Set
`ENCRYPTION_KEY` to a newly generated secret and `APP_URL` to the dedicated public
origin. Optionally set `AUTH_MODE`, `TZ`, `PUID`, `PGID` and `HUB_PORT` as described
in the installation guide. Leave `HUB_IMAGE` unset to use the local
`docklet-next-hub:0.0.0` tag. Use the same env file for every command:

```sh
chmod 600 .env.source
docker compose -f deploy/hub/compose.yaml --env-file .env.source build --build-arg DOCKLET_VERSION=0.0.0 hub
docker compose -f deploy/hub/compose.yaml --env-file .env.source up -d --no-build hub
docker compose -f deploy/hub/compose.yaml --env-file .env.source ps
```

The source template retains its `build` section; the rendered release package
removes that section and pins the hub and agent images by digest. Source builds
without release stamps use a local agent image fallback tagged with the hub
SemVer (`0.0.0` for development). They do not establish official in-app update
eligibility. Use manual Compose updates for local images.

The hub port is published on loopback. Configure the operator's proxy using
[the reverse-proxy guide](../../docs/deployment/reverse-proxy.md). Keep the
`hub-data` and `hub-backups` volumes on persistent storage; the SQLite data volume
must be local. Do not import earlier databases or registrations. First login,
agent installation, sign-in policies and backup recovery are covered in the
installation and upgrade guides.

## Build and transfer the agent

Build the agent at the same local version:

```sh
docker build --platform linux/amd64 --target agent --build-arg DOCKLET_VERSION=0.0.0 --tag docklet-next-agent:0.0.0 --file deploy/hub/Dockerfile .
docker save --output docklet-next-agent.tar docklet-next-agent:0.0.0
```

Transfer the archive to each agent host, then load it there:

```sh
docker load --input docklet-next-agent.tar
```

Alternatively use an operator-controlled registry and set `DOCKLET_AGENT_IMAGE`
in the hub's private env file, then recreate the hub. Operator overrides accept
any valid image reference; a version and digest pin gives reproducible installs.
The generated agent Compose file also accepts an override on its host.
Environment creation, updates and deletion require a
global administrator. The former endpoint
`GET /api/environments/{id}/deployment` no longer exists; use the generated
one-time installation package and token through the hub instead.
Ensure the selected agent image
exists on each target host or is accessible through its configured registry.

## Build arguments and validation

`NODE_IMAGE` and `GO_IMAGE` override the pinned build stages. `RUNTIME_IMAGE`
names the pinned stage preparing directories and the assessed CA bundle; the
final hub and agent images are `scratch`. They contain static binaries, embedded
time zones, the CA bundle and licence texts, with no shell or GPU utilities.
Reassess changed dependencies and the CA hash before distributing changed images.
`DOCKLET_VERSION` controls the source-build product version; the release build
script also stamps revision, build time and the exact default agent image.

CI builds and smoke-tests both local targets without publication. In an approved
isolated test environment, the existing checks are:

```sh
node scripts/smoke-hub-image.mjs docklet-next-hub:0.0.0
node scripts/smoke-agent-image.mjs docklet-next-agent:0.0.0
```

These checks create temporary containers and volumes. They need Docker, Go and
OpenSSL. They verify health, persistence where applicable, licence hashes,
private directory modes, numeric non-root startup and CA trust without a runtime
shell. They do not replace practical release acceptance or authorise a deploy.
