<!--
Source: docklet-next/issues/363
Revision: b11b3e70e671dd3c862b966b1d279251842c7319
Retrieved: 2026-11-09
License: Apache-2.0
Deviations: Original release installation instructions verified against the package and runtime producers.
-->

# Install Docklet Next

Install the hub from a package on [GitHub Releases](https://github.com/florianfinn/docklet-next/releases),
then add agents through the hub. Check [compatibility](compatibility.md) first.
Release images target **linux/amd64**:
`ghcr.io/florianfinn/docklet-next-hub` and
`ghcr.io/florianfinn/docklet-next-agent`.
Until a release is published, and for local builds without a product registry,
use the [source-build guide](../deploy/hub/README.md).

## Prepare the host and public address

Use Docker Engine with the Docker Compose plugin (`docker compose`) on the hub
and agent hosts. Keep the hub's persistent database storage local. SQLite is the
only database; SMB, NFS and FUSE storage are unsupported, including named volumes
backed by those filesystems. Begin with empty data volumes: earlier docklet-hub,
Arcane, pre-release Docklet Next and development PostgreSQL data or registrations
are not imported.

Serve the hub at the root of a dedicated domain or subdomain, for example
`https://docklet.example.com/`. Sub-path hosting is unsupported for assets,
routing, API, WebSockets, authentication and links. Set `APP_URL` to that public
origin. Configure TLS, WebSocket forwarding, unbuffered events and forwarded
headers using [the reverse-proxy guide](deployment/reverse-proxy.md).

The package publishes the hub on `127.0.0.1:3552` by default. A proxy on the same
host can use that address. A containerised proxy needs an explicitly configured
path to the hub; its own loopback address does not reach another container.
The mounted Docker socket gives the hub management access to the local daemon.

## Download and verify the package

Choose an available release and download **all** files listed in its
`SHA256SUMS` into a new, dedicated installation directory. The package includes:

- `docklet-next-X.Y.Z-compose.yaml`, with pinned hub and agent defaults;
- `docklet-next-X.Y.Z.env.example`, containing the inputs read by that Compose file;
- `release-manifest.json`, with source and image identities;
- `LICENSE`, `NOTICE`, `BSD-3-Clause-Arcane.txt` and `SHA256SUMS`.

`X.Y.Z` below is a placeholder; replace it with the chosen release version,
including its prerelease suffix when applicable. Run these commands inside the
installation directory. Verify the downloaded files before editing them:

```sh
sha256sum --check SHA256SUMS
cp docklet-next-X.Y.Z-compose.yaml compose.yaml
cp docklet-next-X.Y.Z.env.example .env
chmod 600 .env
```

Keep the downloaded originals and manifest for later verification. Store `.env`
privately and keep its encryption key with protected backups, separately from the
hub's data volume. Generate a new key, for example with `openssl rand -hex 32`,
and paste the output into `.env`; do not use an example secret.

## Configure the hub

Edit the copied `.env`. Required deployment values are:

- `ENCRYPTION_KEY`: the private encryption key. Compose refuses an empty value.
- `APP_URL`: the public origin, such as `https://docklet.example.com`.
- `AUTH_MODE`: choose the sign-in policy below; the default is `local`.

Optional package inputs are:

- `HUB_PORT`: loopback host port, default `3552`.
- `TZ`: IANA time zone for schedules and display, default `UTC`.
- `PUID` and `PGID`: numeric application identity, both default `65532` in the
  hub package. Startup prepares the private data directories and drops privileges;
  it detects the Docker socket's group and preserves supplementary groups.
- `HUB_IMAGE` and `DOCKLET_AGENT_IMAGE`: image overrides. Leave them unset or blank
  to use the package defaults. Docklet's release defaults use
  `name:X.Y.Z@sha256:…`; use the actual digest from the package, not this notation.
- `AUTH_PROXY_HEADER`, `AUTH_TRUSTED_PROXIES`, `AUTH_RESET_ADMIN_PASSWORD` and their
  `_FILE` / `__FILE` variants, when needed for sign-in or recovery.

The repository [`.env.example`](../.env.example) documents the full runtime
configuration. Compose's `.env` is an interpolation input: a key affects the hub
only when the service passes it into the container. The package forwards the
inputs above; add other runtime values under `services.hub.environment` in your
local `compose.yaml` as needed. In particular, see the reverse-proxy guide for
`TRUSTED_PROXIES`. Mount any secret files and use their container paths.

Image overrides are unrestricted valid references, including mutable tags and
operator registries. This also applies to the optional `TOOLS_IMAGE` setting
(passed explicitly into the service environment when used). Docklet's own tools
default remains pinned. Custom hub or agent images may disable in-app updates;
the hub still checks the agent contract.

### Select a sign-in mode

`AUTH_MODE` is already in the hub Compose environment. Choose exactly one value
and recreate the service after changes:

- `local`: local passwords and sessions. On a fresh installation, sign in as
  `arcane` using the initial password printed in the hub startup log. Change it
  when prompted, then complete setup.
- `proxy`: the proxy authenticates every request and strips and replaces
  `AUTH_PROXY_HEADER` (default `Remote-User`). Set `AUTH_TRUSTED_PROXIES` to exact
  direct TCP peer IP addresses or CIDRs as seen by the hub. Missing or malformed
  values and all-address `/0` networks prevent startup. Forwarded client headers
  never grant sign-in trust. By default accepted proxy identities act as the
  shared administrator. Optional `AUTH_PROXY_ADMIN_USERS` limits that mapping;
  other identities need a matching enabled username, and a name matching an
  administrator account acts as that administrator. Pass this optional setting
  explicitly in the Compose environment. `AUTH_PROXY_LOGOUT_URL` optionally sets
  an absolute HTTP(S) proxy sign-out URL and also needs explicit forwarding.
- `none`: everyone who can reach the hub has full administrator access. Mode
  changes revoke sessions but retain users, API keys and agent tokens. Review
  users and rotate unwanted credentials after using this mode.

`TRUSTED_PROXIES` controls forwarded metadata separately from
`AUTH_TRUSTED_PROXIES`. Optional proxy administrator and logout settings support
`_FILE` and `__FILE`, as do the four package sign-in settings. For file-backed
values, `__FILE` takes precedence over `_FILE`, then the direct value; unreadable
files warn and fall back to the direct value.

## Start and complete setup

Use the same project directory, Compose file and env file for later commands:

```sh
docker compose -f compose.yaml --env-file .env pull hub
docker compose -f compose.yaml --env-file .env up -d hub
docker compose -f compose.yaml --env-file .env ps
docker compose -f compose.yaml --env-file .env logs hub
```

Wait for the service to become healthy (`/api/health`), then open the configured
public address. Complete the first-login password change in local mode and the
hub setup flow. Proxy mode uses the proxy identity; none mode opens directly.
The `hub-data` volume holds the database pair under `/app/data`; `hub-backups`
holds hub archives under `/var/lib/docklet/backups`. Keep both across container
replacement. Never delete live SQLite WAL/SHM files or copy just a live main
database file as a backup. Use **Hub database** › Hub database backups (`/hub-backups`) to create
and download a coordinated archive.

## Add Direct and Edge agents

As a global administrator, add a host through the hub's onboarding flow. Select
the connection mode, hub address, project directory, installation directory and
file owner, then download the generated private installation archive.
Environment creation, updates and deletion require a global administrator.

- **Direct:** the hub connects to the agent's published HTTP port (default `3552`).
  Restrict that port to the hub with the agent host's firewall. HTTP requires a
  trusted network.
- **Edge:** the agent makes the outbound tunnel connection to the hub; its
  generated Compose file publishes no agent port. The selected hub address must
  be reachable from that host.

Agents must reach `/api/environments/pair` and `/api/tunnel/*` without browser
sign-in. If the public proxy requires browser authentication there, use an
appropriate internal hub address for the agent connection.

The archive contains `compose.yaml`, `.env`, `install.sh` and `README.txt`.
Create the project directory with the intended owner first. Extract the archive
privately, retain `.env` beside the script and run on the agent host:

```sh
sh install.sh
```

The installer requires a POSIX shell, Docker Compose v2, `stat` with `-c`, `awk`,
`mktemp` and standard file utilities; BusyBox is supported. It uses the
project directory's owner as `PUID`/`PGID` unless an explicit owner was selected.
The agent detects the socket group at every start before dropping privileges;
an explicitly selected manual group is emitted as `group_add`. The installer
does not change project ownership or permissions. Startup may change ownership
of the agent's own `/app/data` directory. Ensure the runtime identity can write
the project directory.

The installer refuses to overwrite existing Compose projects unless every
service has `io.docklet.system=agent`. It starts only the generated agent project:

```sh
docker compose -f compose.yaml --env-file .env up -d
```

Run that command only from the generated installation directory if managing it
manually. Keep `.env` private (the installer writes it with mode `0600`) and
delete the downloaded archive after installation. Copyable commands contain
the token and can leave it in shell history; prefer the archive.

Wait for the hub's next successful health check to confirm compatibility and
report directory access (default interval: two minutes). New agents start at the
package's agent version; use **Update agent** afterwards if the hub has already
been upgraded. See [upgrading and recovery](upgrade.md).
