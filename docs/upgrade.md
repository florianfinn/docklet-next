<!--
Source: docklet-next/issues/363
Revision: b11b3e70e671dd3c862b966b1d279251842c7319
Retrieved: 2026-11-09
License: Apache-2.0
Deviations: Original upgrade and manual recovery instructions verified against self-update and database producers.
-->

# Upgrade Docklet Next

Create and download a hub archive from **Hub backups** (`/hub-backups`) before
upgrading. Keep the external encryption key and the current Compose/env files
with protected recovery material. Hub archives contain the two SQLite databases;
they do not contain workload volumes, project files or external secrets.
Check [compatibility](compatibility.md) and the target release notes first.

Migrations run forward at startup. An older binary may refuse a database migrated
by a newer release. Earlier docklet-hub, Arcane, pre-release Docklet Next and
development PostgreSQL databases or registrations cannot be imported; those
installations need a fresh baseline as described in [installation](install.md).

## Update from the application

As an administrator, open **Settings › Live updates**. Select **Update Docklet**
for the hub or **Update agent** for a host. Review the target and confirm. The
hub's confirmation links to Hub backups. Follow the run in the activity view;
the browser reconnects and reloads after a confirmed successful hub update.
Update the hub first: an agent target cannot be newer than the running hub.

The updating process pulls the target by digest and starts a short-lived helper
container from the new image. The helper backs up the project's env file, writes
`HUB_IMAGE` or `DOCKLET_AGENT_IMAGE` with the new pin, and uses the embedded
Compose implementation to bring up **that one service**, without recreating its
dependencies or removing orphans. Later manual Compose commands keep the new pin.
The helper checks the replacement's health for up to 120 seconds.

The helper runs as root. It preserves the original env owner and mode on its
own env replacement and backup, and gives its result file to the application's
UID/GID with mode `0640`. It changes ownership only on files it creates itself.
It does not migrate or open the database; normal application startup does that.

There is **no automatic rollback** for hub or agent updates. A failure before
any container mutation may restore the original env bytes. Once the container
swap can have started, a health failure or unknown outcome requires manual
recovery. Read the run result and inspect the actual running service before
retrying.

New agents still start at the installation package's agent version, even after
an in-app hub update. Use **Update agent** afterwards to bring them up to the hub.

### When the action is unavailable

The page shows a state, a refusal reason and manual commands when the live
check can return them. In-app updates require an official stable image in a
replayable Compose installation. The main reasons are:

| Code | Meaning and next action |
| --- | --- |
| `update.self-not-compose` | Required Compose project, service or file labels are missing. Use the installation's manual method. |
| `update.self-custom-image` | The current repository or tag is not the official stable release matching the binary version. Update the custom image manually. Prerelease and development images are also ineligible. |
| `update.self-unsupported-layout` | The recorded Compose layout or mounts cannot be replayed. Use the recorded manual inputs. |
| `update.requires-hub-update` | The proposed agent version exceeds the hub; update the hub first. |
| `host.busy` | Another host operation is active; let it finish. |
| `feature-disabled` | Release checks are disabled by `UPDATE_CHECK_DISABLED`. |
| `update.check-unavailable` | Release, image or installation information could not be checked. Restore access and check again, or use the manual path. |
| `update.system-version-unknown` | The running build has no usable product version. Use a versioned image manually. |

The supported layout has a Compose working directory and ordered config files
recorded in container labels, and one interpolation env file inside that directory
(the recorded file or its `.env`). The files must be regular and free of symlink
components; `include` and `extends` are refused. Required interpolation inputs
must resolve from that env file. The effective image must match the running
container and change when the role's image variable changes. Selected service
env/label files must also stay inside the directory. The helper needs a
bind-mounted Unix Docker socket and writable mounted application data; TCP/TLS
daemon endpoints and socket proxies do not meet this profile. Full file/layout
validation runs in the helper before the env pin is changed, so a later
validation failure can still occur after the button was enabled.

## Upgrade manually

For the hub, download the target release package and [verify its checksums](install.md#download-and-verify-the-package).
Keep the existing project name, persistent volumes and private settings. Compare
the target Compose file with your local one and apply required changes. Copy
the target hub and agent image references from the verified package or manifest
into `HUB_IMAGE` and `DOCKLET_AGENT_IMAGE` in your existing `.env`. Setting both
keeps newly generated agent files on the target package version.

From the original installation directory, using the same project and file
inputs as installation, run:

```sh
docker compose -f compose.yaml --env-file .env pull hub
docker compose -f compose.yaml --env-file .env up -d --no-deps hub
docker compose -f compose.yaml --env-file .env ps
docker compose -f compose.yaml --env-file .env logs hub
```

If installation used `-p` or additional `-f` files, retain them in the same order.
Do not start a different Compose project or attach empty replacement volumes.
Check `/api/health`, startup migration results, the served UI and sign-in.
Changing `.env` requires recreation with `up`; `restart` alone does not load it.

For agents, set the target `DOCKLET_AGENT_IMAGE` in the agent installation's env
file, then use the exact commands shown by the hub in Settings › Live updates.
Those commands include the recorded project name, config files, env file and
service. For an unchanged generated agent project, the equivalent is:

```sh
docker compose -f compose.yaml --env-file .env pull agent
docker compose -f compose.yaml --env-file .env up -d --no-deps agent
```

Keep the agent's token, directories, data and file-owner settings. Wait for a
successful hub health check; `update-required` clears after compatibility is
confirmed. Older agent contracts retain only health/version and update recovery
access until then.

## Recover a failed in-app update

The helper keeps a per-operation backup beside the interpolation env file:
`<env-file>.docklet-previous.<opId>`, normally `.env.docklet-previous.<opId>`.
Its result remains at `/app/data/self-update/<opId>.json` in the service's data
storage. Keep both until the result and running version have been verified.

Use the recovery commands shown by the hub when it is reachable; they are built
from the recorded installation inputs. If it is down, inspect those inputs on
the host and select the backup for the failed operation. The following example
assumes the standard hub installation; replace `OPERATION_ID` before running:

```sh
cp -- .env.docklet-previous.OPERATION_ID .env
docker compose -f compose.yaml --env-file .env pull hub
docker compose -f compose.yaml --env-file .env up -d --no-deps hub
docker compose -f compose.yaml --env-file .env ps
docker compose -f compose.yaml --env-file .env logs hub
```

For an agent, run the corresponding recorded commands on its host with service
`agent`. Restore the old env only if returning to that image is appropriate:
restoring an image pin does **not** restore data or reverse migrations. Keep the
newer image and investigate if the old binary cannot open the migrated database.
If database recovery is required, use a compatible pre-upgrade archive and image.

### Offline hub database recovery

Stop the hub. Put a trusted hub archive in the backup volume under its original
server-owned `<backup-id>.tar` filename. Configure the intended compatible hub
image. If that binary cannot open the existing database, use an empty replacement
data volume and preserve the original volume; the restore command validates its
destination before replacing it. Replace `example-archive` below with the ID:

```sh
docker compose -f compose.yaml --env-file .env stop hub
docker compose -f compose.yaml --env-file .env run --rm hub restore-hub --backup-id example-archive
docker compose -f compose.yaml --env-file .env up -d hub
```

The offline command takes the lifetime lock, validates and replaces both database
files, migrates forward and revokes restored sessions. It retains safety copies
and a durable restore marker for interrupted recovery. If recovery refuses to
start, keep those files and resolve the reported error; do not delete them to
bypass validation. Provide enough local space for the archive, staging and safety
copies. See the [hub backup contract](design/hub-backup.md) for archive boundaries.

### Sign-in recovery

Set `AUTH_RESET_ADMIN_PASSWORD` in the hub's private env file to a new password
satisfying the configured policy, then recreate the hub with the same Compose
inputs. Startup resets the global administrator's password and revokes its
sessions. Remove the value after successful recovery and recreate again; a value
left set is applied on each startup. The setting also supports secret files.

## Configure the release feed

Update checks read stable published GitHub Releases of
[`florianfinn/docklet-next`](https://github.com/florianfinn/docklet-next/releases).
Drafts, prereleases and invalid tags are ignored; an empty feed or first-page
404 means no release has been published. There are no scheduled self-updates.

`DOCKLET_RELEASE_FEED_URL` optionally replaces the releases-list API URL.
HTTPS is allowed for any host. HTTP is allowed only for loopback, private-network
(RFC 1918 or RFC 4193) and link-local addresses, or hostnames resolving only to
such addresses. The rule is checked for every request and redirect.
`UPDATE_CHECK_DISABLED=true` disables release-feed requests and update actions.
To use either with the package, explicitly pass it under the hub service's
`environment`, then recreate the hub. See [the runtime reference](../.env.example).
