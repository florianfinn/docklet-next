<!--
Source: docklet-next/issues/439
Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
Retrieved: 2026-11-09
License: Apache-2.0
Deviations: Original narrow Compose self-update design summary.
-->

# Compose-faithful in-app updates

The hub selects stable releases from its release feed and resolves the exact
canonical role tag to a manifest or index digest with a linux/amd64 image. Agents
can update only to a version newer than themselves and no newer than the hub.
Custom repositories, floating tags and non-Compose installs use manual updates.
The stamped binary version and the current container's Config.Image identify the
installation; RepoDigest ordering and OCI display labels do not determine policy.

The updating process persists a correlated operation in the existing KV store
before creating its detached helper. A hub run is reserved before either a helper
or an agent request. A lost response is observed with the same operation ID; it
never causes a replacement operation or another dispatch. Registration and
credential revision qualify every remote observation and its terminal write.

The helper uses the target image's existing ENTRYPOINT with the arguments
`self-update --op <operation-id>`. It has no Compose membership labels, no health
check or restart policy, and explicit daemon-visible project, socket and data
mounts. The socket source is mapped to `/var/run/docker.sock`; TCP/proxy endpoints
are unsupported. It runs with the application's effective user and groups, opens
no database and does not chown any file.

Before changing the env file, the helper loads the ordered Compose files with
only their recorded interpolation file and the explicit project name. It checks
that the selected service's effective image matches Config.Image and that changing
the package variable changes that effective image. Symlinks, outside files,
multiple interpolation files and nested include/extends models are refused.
Only the named service is selected, with dependencies ignored, builds disabled
and no orphan removal. The image key is appended when absent.

The env file is replaced with temp-file, sync and rename, preserving its mode and
owner. An exclusive `<env-file>.docklet-previous.<operation-id>` backup keeps the
original bytes. Helper phases are atomically written to the application's data
mount under `self-update/<operation-id>.json`, with the correlated request in
`self-update/<operation-id>.request.json`. The request and helper are removed only
after durable settlement; the result and backup remain available for diagnosis.

There is no automatic rollback. Only a failure proved to precede the Compose
swap restores the original env bytes. Once replacement may have started, failed
health or a lost helper leaves the new pin and previous backup for manual
recovery. A new application may already have migrated its database: restoring an
old image is not a database restore. Take a supported backup before updating.

A startup observer polls independently of readiness. Success requires a matching
healthy result and the running target version. Generic activity repair excludes
runs owned by unsettled operations. The normal deadline is ten minutes, extended
once while a helper is still running; missing final evidence becomes a failed
run with an unknown-outcome code. Host admission excludes simultaneous restart,
workload and backup mutations. Workload updates targeting Docklet itself decline
before pulling or mutating the Engine with `image.system-protected`.

The recovery routes remain authenticated and administrator-only:
`POST /api/environments/0/system/upgrade` accepts the hub-written
`{opId,targetVersion,targetReference,hubVersion}` envelope;
`GET /api/environments/0/system/upgrade/check` returns version, eligibility and
correlated operation evidence. Hub requests for remote hosts are handled locally
before forwarding, so browsers cannot supply trusted envelope fields. The old
update-all routes and job/resume/log-cleanup implementation are removed. The
manual `upgrade --image` CLI remains available.

For manual recovery of an eligible Compose installation, inspect the preserved
backup and result, restore the chosen env file explicitly, and run the original
project, file and env inputs, for example:

```sh
cp .env.docklet-previous.<operation-id> .env
docker compose -p example -f compose.yaml --env-file .env up -d --no-deps hub
```

Choose a compatible data backup when required. The helper never restores data or
promises that an old binary can read a newer schema. The separate web step owns
update controls, run following, backup guidance and a single confirmed hub reload.
