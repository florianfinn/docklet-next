# Host onboarding

This record describes how an operator adds a host for M5 (#80, #81, #84): the
direct and Edge registration flows, the agent token lifecycle, the generated
installer and its directory and permission checks, the hidden agent container and
the display of a disconnected host. Arcane's environment module is the baseline;
the frozen React reference supplies the approved onboarding UI and a pure
generator for the installation artefacts. Each item names its source or is
marked **new**. Threats and their verdicts are in the
[Edge threat review](edge-threat-review.md); open product questions are listed
at the end.

File references are relative to `backend/` unless they start with `web/` or
`docs/`.

## Terms

- **Host** is an Arcane environment. Each host has one agent: the same Go binary
  in agent mode (direct) or Edge agent mode (`AGENT_MODE`, `EDGE_AGENT` in
  `internal/config/config.go`; `EDGE_AGENT` implies agent mode).
- **Install directory** holds only the agent's own files: `compose.yaml`, `.env`
  and the agent data directory. It is never a stack; the installer refuses a
  directory that already holds an unrelated stack.
- **Project directory** holds the host's stacks as `<project dir>/<stack>/compose.yaml`.
  Its default comes from the hub setting `agentProjectsDirectory`, set in M4
  setup ([adapter and capabilities](adapter-live-capabilities.md)).

The M4 setup also stores `agentInstallDirectory`, `defaultHostConnection`,
`defaultPuid`, `defaultPgid`, `detectSocketGroup` and the hub addresses
`hubAddress` and `domainUrl` (`internal/settings/model.go`). The onboarding form
starts from these values and lets the operator change them per host
(`freshDraft` in `web/src/features/hosts/onboarding/onboarding-draft.ts`).

## Onboarding steps

The reference UI has four steps: connection, directories, install, connect
(`ONBOARDING_STEPS` in `onboarding-draft.ts`).

1. **Connection.** Name, colour and connection type. Direct asks for the address
   and port the hub dials; Edge asks for nothing more. Source: reference UI;
   Arcane stores name, `api_url` and `is_edge` (`CreateEnvironment` in
   `internal/environment/handler.go`).
2. **Directories.** Project directory, install directory, optional manual
   PUID/PGID and optional manual socket group. Validation rules are under
   [Directories and permissions](#directories-and-permissions).
3. **Install.** The hub creates the host in state `pending` with its token, and
   the UI shows the installation artefacts.
4. **Connect.** The UI waits for the host to become online and reports the
   checks the agent returns.

## Direct registration

1. The hub creates the environment with the bootstrap key flow
   (`createEnvironmentWithApiKeyInternal` with `useApiKey` in
   `internal/environment/handler.go`). The host is `pending`; the raw key is
   returned in the create response and also stored in `environments.access_token` for
   hub-to-agent requests. Source: Arcane.
2. The operator runs the installer on the host. The agent starts passively and
   waits for the hub (direct-mode branch in `internal/bootstrap/bootstrap.go`).
   Source: Arcane.
3. The hub's environment health job calls the agent with the token
   (`runHealthCheckInternal`, `TestConnection` in
   `internal/environment/service.go`). Success marks the host online. Source:
   Arcane.
4. The hub reads the agent's version info and applies the compatibility rule
   before anything else is forwarded. Source: **new**, decision D1 and review
   decision S3 in the threat review.
5. Only a compatible agent receives sync state and registry credentials; Arcane
   pushes them in the health job right after the connection test, before any
   version check. Source: Arcane sync, ordering **new** (#81).
6. The host appears in the host list and the onboarding step completes. The
   agent's local check results are shown when available (decision 3).

The direct agent publishes one port on its host. The generated compose maps it
as `<port>:3552`; Arcane's own snippets do the same on every interface
(`Direct` in `internal/environment/children/snippets/service.go`). Binding and
plain-HTTP warnings follow decision D5 in the threat review.

## Edge registration

1. The hub creates the environment as above with `is_edge` set and API key
   registration (`useApiKey=true`). Legacy token-only Edge creation is rejected
   with HTTP 400 before a host is persisted. Edge hosts use an
   `edge://` API URL marker (`isEdgeEnvironmentURLInternal` in
   `internal/middleware/environment.go`). Source: Arcane.
2. The installer starts the agent with `EDGE_AGENT=true`, `MANAGER_API_URL` and
   the token. No port is published. Source: Arcane variables; generator **new**
   (decision 1 on variable names).
3. At startup the agent pairs once: it posts its key to the hub's pairing
   endpoint, which moves a `pending` host to `online` and triggers resource sync
   (`handleAgentBootstrapPairing` in `internal/bootstrap/bootstrap.go`,
   `PairEnvironment` in `internal/environment/handler.go`). Source: Arcane. In M5
   that sync waits for the compatibility check in step 5 (review decision S3,
   **new**).
4. The agent opens the tunnel to `/api/tunnel/connect` or polls
   `/api/tunnel/poll`, chosen by `EDGE_TRANSPORT` (`auto` by default; Arcane's
   snippets set `poll`). Registration rules, rate limits and stale handling are
   described in the threat review (T4, T9). Source: Arcane.
5. Version check, then sync and completion as in direct steps 4 to 6.

The hub supplies `/hub/discovery` to global administrators. It ranks the configured
public `domainUrl` first, then `APP_URL`, `hubAddress` and detected process
interface addresses. Candidates carry reachability hints, not an agent-side
connectivity verdict; loopback is marked same-host and other candidates unknown.
The operator selects or edits the hub URL in onboarding. The reference allows an internal LAN or VPN address
(`hubUrl` in `web/src/domain/agent-install.ts`), and the planned `HubDiscovery`
contract ranks address candidates ([API contract](api-contract.md)). The agent
must reach `/api/environments/pair` and `/api/tunnel/*` without a browser sign-in.
In proxy sign-in mode the operator either routes those paths past the
authenticating proxy or gives agents an internal hub address that bypasses it.
Onboarding states this next to the hub address. Source: **new** text; the
machine routes already keep their own contract ([identity](identity.md)).

## Token lifecycle

| Stage | Behaviour | Source |
| --- | --- | --- |
| Creation | Random environment bootstrap key; `api_keys` keeps its Argon2 hash and environment-scoped grants, `environments.access_token` keeps the raw value | Arcane (`CreateEnvironmentApiKey` in `internal/apikey/service.go`) |
| Display | Returned once by creation or rotation and held only in the open browser flow; list, detail and legacy deployment reads never return it | New in #84; the legacy deployment endpoint returns HTTP 410 |
| On the host | Only in the install directory's `.env`, mode 600, created under a private umask; never a command-line argument, never in `install.sh` or a fixed temporary file | Generator rules below, **new** in #84 (the reference embeds the token in `install.sh` and leaves readable copies, threat review T10); Arcane also reads the token from a file (`options:"file"`) |
| Use | Same value in both directions and as the hub API key for that host | Arcane (threat review T3) |
| Rotation | Regenerate action in the artefact view: new key, host set to `pending`, old key deleted, Edge token cache synced; the operator reruns the install or edits `.env` | Arcane (`RegenerateEnvironmentApiKey`); reference button in `InstallArtifacts.tsx`, live wiring **new** |
| Rotation, sessions | Established tunnels and proxied streams of the host are closed; the new token is never sent through a session authenticated with the old one | **New**, #81 (review decision B2 in the threat review) |
| Pending | A `pending` host's token stays valid with no expiry; deleting the host revokes it | Arcane |
| Removal | Workloads do not prevent removal; containers and stacks keep running on the host. Only the hub registration and credential are removed. Deleting a host cascades its key; the hub API and Edge registration then reject new authentication with it | Arcane |
| Removal, sessions | Established tunnels and proxied streams of the host are closed | **New**, #81 (review decision B2) |

Rotation and deletion produce environment events
(`EventTypeEnvironmentApiKeyRegenerated` and create/delete events in
`internal/environment/service.go`).

## Installation artefacts

The reference generator `buildInstallArtifacts` in
`web/src/domain/agent-install.ts` produces, from one install plan:

- `compose.yaml` for the agent,
- `.env` with connection, token, project directory, PUID/PGID, socket group and
  time zone,
- `install.sh`, which checks the host and then writes both files and starts the
  agent; the reference embeds the `.env` content in it, which the generator rules
  below replace,
- a README,
- a **compose command**: a copyable heredoc that writes both files and runs
  `docker compose up -d`,
- a **script command**: a copyable heredoc that writes and runs `install.sh`.

The UI offers the package as a tar download and each file separately
(`web/src/features/hosts/InstallArtifacts.tsx`). Arcane's snippets
(`internal/environment/children/snippets/service.go`) supply the variables and
mounts but are not used as output: they reference Arcane's agent image with
`latest`, inline the token and carry no Docklet labels. Generation location is
decision 6.

Generator rules for M5:

- The hub supplies the pinned `DOCKLET_AGENT_IMAGE` override, including a digest
  when available. Without an override it supplies `docklet-next-agent:<hub SemVer>`
  (`0.0.0` for development builds), a local-build fallback without a published
  registry or invented digest. An invalid override fails discovery. The agent
  image must be available on the target host before installation.
- The token appears only in `.env` content, never as a command argument, in
  compose, in `install.sh` or in the README. The copyable commands write `.env`
  through a heredoc, so the pasted token can remain in interactive shell
  history; the install step says so and the downloaded package avoids it.
- Every file that holds the token is private from its creation (review decision
  B4, **new** in #84). `.env` is created under `umask 077` and ends with mode 600;
  no `chmod` after a readable creation. `install.sh` reads the token from the
  `.env` shipped next to it and writes the installed `.env` the same way. The
  archive stores `.env` with mode 600 and `install.sh` with mode 700. The script
  command uses a private `mktemp -d` directory for `.env` and `install.sh`,
  removes it on exit through `trap`, and uses no fixed temporary file name. The
  README tells the operator to delete the downloaded archive after installation.
- The compose file mounts the Docker socket, the project directory at the same
  path on both sides, and the agent data directory. Arcane's
  `PathMapper` can translate differing paths through mount discovery
  (`NewPathMapperForConfiguredDirectory` in `pkg/projects/path_mapper.go`); the
  identical mount avoids that translation and keeps relative bind sources
  correct.
- The agent data directory is mounted at Arcane's data path `/app/data`
  (`defaultDataDirectory` in `runtime_identity.go`), not at the reference's
  `/data`.
- `PROJECTS_DIRECTORY` in `.env` equals the mounted project directory.

## Directories and permissions

### Checks before installation

The hub cannot see a host before its agent runs, so checks run in two places.

| Check | Where | Source |
| --- | --- | --- |
| Paths are absolute and normalised | UI, before install | Reference (`absolutePath`, `normalizePath` in `web/src/domain/validation.ts`) |
| Install directory is not the project directory, does not contain it and is not inside a stack folder; its own subfolder of the project directory is allowed | UI and `install.sh` | UI: reference (`installDirProblem`, path relationships only); decision 5. `install.sh` in the reference checks only equality; the other relationships **new** (#84) |
| Install directory holds no unrelated stack: before any write, `install.sh` and the compose command stop if it contains a compose file (`compose.yaml`, `compose.yml`, `docker-compose.yaml`, `docker-compose.yml`) without the label `io.docklet.system=agent`; a recognised agent install is reinstalled in place | `install.sh` and compose command | **New** (#84), review decision S4; the reference truncates `compose.yaml` and `.env` without this check |
| Project directory exists | `install.sh`; it stops otherwise | Reference |
| Project directory is writable for the installing user | `install.sh`, warning only | Reference |
| Agent can read and write the project directory as PUID/PGID | Agent at runtime | Arcane reports unreadable project directories (`pkg/projects/discovery.go`, `pkg/projects/env.go`); write check **new** |

The installer changes no ownership and no modes, except mode 600 on the agent's
own `.env`. Arcane's runtime identity takes ownership of its own data directory
when it starts as root inside a container
(`prepareWritablePathsWithRootsInternal` in
`pkg/libarcane/startup/runtime_identity.go`). The project directory lies outside
that data directory and is not changed. The reference README states that nothing
is changed; its text is corrected to name the agent data directory.

### UID/GID proposal

Without manual values, `install.sh` proposes PUID/PGID from the owner of the
project directory (`stat -c %u` and `%g`). The operator may enter both values
manually in the directories step; both are then required, and root IDs are
allowed with a warning (`usesRootIds` in `onboarding-draft.ts`). Source:
reference. At runtime Arcane requires both values or falls back to its non-root
default of 65532 (`loadRuntimeIdentityRequestInternal`, `defaultRuntimeUID` in
`runtime_identity.go`).

### Docker socket group

The socket group is detected separately from the file owner. Arcane already
adds the socket's group ID as a supplementary group when it re-executes as
PUID/PGID (`runtimeIdentitySupplementaryGroupsInternal` in
`runtime_identity.go`). The reference installer reads the socket group with
`stat` and writes `DOCKER_SOCKET_GID` for compose `group_add`; a manual value is
accepted when detection is switched off. Which mechanism is authoritative is
decision 2.

Under decision 2 (a) a manual group from compose `group_add` does not yet reach
the agent: the re-exec sets the child's supplementary groups explicitly to the
detected socket group or none (`reexecWithRuntimeIdentityInternal` in
`runtime_identity_linux.go`) and drops the groups the container started with. M5
applies the union of both: the groups the root process started with, which
include compose `group_add`, plus the detected socket group, without duplicates.
Detection stays authoritative for the socket and follows group changes; the
manual group is added to it and is the only one when detection finds nothing.
Source: **new** (#84), review decision S1.

### Reported results

The reference promises that the agent reports owner, write access and socket
group to the hub after connecting, and the connect step shows them
(`web/src/features/hosts/onboarding/DirectoriesStep.tsx`, `LocalCheckNote`).
Arcane has no such report. Source: **new**; decision 3.

## Agent as a system component

The agent container must not appear in stack or container lists (#82). Arcane
hides containers labelled `com.getarcaneapp.arcane.hidden=true` from container
lists, dashboard counts and discovered untracked Compose projects
(`HiddenResourceLabel` in `pkg/libarcane/internal_resource.go`; uses in
`internal/container/service.go` and `internal/project/service_discovery.go`).
The generated compose sets this label. The reference additionally marks the
agent with `io.docklet.system=agent` (`SYSTEM_LABEL` in
`web/src/domain/system.ts`) so the Hosts page can show it as the host's system
component; the planned `SystemComponent` mapping reads that label
([API contract](api-contract.md)). Source: Arcane label, Docklet label from the
reference.

File-based project discovery scans the project directory to
`PROJECT_SCAN_MAX_DEPTH` (default 3) and skips the names in
`PROJECT_SCAN_SKIP_DIRS` (`internal/config/config.go`) as well as Arcane's own
scratch and filesystem snapshot directories (`IsIgnoredProjectDirName` in
`pkg/projects/discovery.go`). Neither covers an agent install directory, so an
install directory inside the project directory would be discovered as a stack.
When decision 5 keeps that layout allowed, the agent excludes its own install
directory from discovery. Source: **new**.

## Disconnected host

Arcane tracks host state as `online`, `offline`, `standby` (Edge poll seen, no
tunnel) and `pending`, with `last_seen`, `connectedAt`, `lastHeartbeat` and
`lastPollAt` (`ApplyEnvironmentRuntimeState` in
`internal/environment/service.go`). Edge tunnels are stale after two minutes
without heartbeat and are reaped every minute; on hub start every Edge host is
reset to offline until it reconnects (`ReconcileEdgeStatusesOnStartup`). Source:
Arcane.

Arcane keeps no inventory for an unreachable host: list requests fail or return
nothing. M5 keeps the last successful read per host and list type with its read
time. While the host is not online, lists and the dashboard show those values
marked stale with "as of <time>", and actions on that host are unavailable with a
reason. Reconnection replaces the values on the next successful read. Source:
**new**; storage is decision 4.

## Runtime read contract

The agent contract and minimum are version 3. The version fixture in
`types/contract/testdata/agent-hub-v3.json` owns the fingerprint. The hub checks
`/api/app-version` and, for direct hosts, the token-protected
`HEAD /api/environments/0/system/health` before ordinary requests and registry or configuration
synchronisation. The credential session (token, endpoint and key ID) is revoked
on credential changes, endpoint changes, disablement or removal. Compatibility
verdicts are cached within that session for the concrete Edge tunnel and refreshed
on Edge connect and health checks. Direct hosts become online only after the
protected probe succeeds with the current token. A probe returning 401 or 403 sets
`token-rejected` without advancing `last_seen`; the host card and onboarding show
the credential rejection in English and German. Failed probes are retried on the
next check, and retained inventory snapshots keep their existing stale semantics.
Ordinary Edge requests acquire a concrete tunnel
before checking compatibility; a tunnel change invalidates its verdict without
cancelling requests or streams in the credential session. An update-required verdict
replaces prior compatibility and blocks ordinary forwarding until a fresh check succeeds. Below-minimum agents and same-version fingerprint mismatches
have status `update-required`; health, version and agent self-upgrade stay
available. System health uses HEAD, as its registered operation specifies.
Forwarded HTTP access failures distinguish `agent.update-required` (HTTP 409),
`agent.invalid-token` (HTTP 502 for credential rejection, including forwarded
agent HTTP 401 responses; ordinary agent HTTP 403 remains an authorization response) and
`agent.unavailable` (HTTP 502, or 504 for a timed-out Edge request). An update-required health job reports a partial
result with "Agent update required", rather than offline, and sends no sync.

`Environment.agentReport` contains the last successful process-local version
report and its observation time. Only agent version requests authenticated with the
configured manager credential add nullable
`onboardingChecks`: project directory, numeric owner UID/GID, effective write
access and socket group. Anonymous agent reads and all hub version reads omit
these fields. The on-demand write probe is cached for 30 seconds. Missing checks remain unknown. These fields do not
claim a manual identity source or resolve host account names. The HTTP adapter
maps this report and connection state through `listHosts`; host mutations and
system-component inventory keep their separate capability boundaries. Inventory
metadata absent from this read has `detailsUnavailable` set; numeric fields in
the existing view model are empty sentinels, not host measurements.

`EnvironmentService.ReadSnapshots()` exposes the process-local inventory store
in `backend/pkg/hoststate/`: `Put`, `Get` and `Forget` use a host/list key and
carry the successful read time. Read adapters own their list payloads. Stored
bytes are copied at both boundaries. Disconnects retain snapshots, the next
successful read replaces them, and host removal forgets them. A new hub process
starts with an empty store and no agent reports; persisted `last_seen` remains
the last successful contact, and failed checks do not advance it. Disconnected
Edge inventory reads return a matching snapshot immediately. Without a matching
snapshot, they wait at most one second for tunnel recovery before reporting
unavailable. Pending hosts return a matching snapshot or unavailable immediately,
without agent I/O or tunnel recovery. Inventory readers include pending hosts
because token rotation and Edge reconnect also enter that state. Initial list
queries publish completed hosts progressively; background
refetches retain the cached list until all hosts finish.

## Decisions

The maintainer chose option (a), the recommendation, for every item below on 2026-10-17.

1. **Agent configuration names in generated files.**
   Recommended: (a) generate Arcane's existing variable names (`AGENT_MODE`,
   `EDGE_AGENT`, `AGENT_TOKEN`, `MANAGER_API_URL`, `PROJECTS_DIRECTORY`, `PORT`,
   `PUID`, `PGID`) and change the reference generator accordingly; no
   configuration code changes. (b) add `DOCKLET_*` aliases as used by the
   reference (`DOCKLET_MODE`, `DOCKLET_HUB_URL`, `DOCKLET_AGENT_TOKEN`,
   `DOCKLET_PROJECT_DIR`, `DOCKLET_LISTEN`) and document both sets.
2. **Authoritative socket group mechanism.**
   Recommended: (a) Arcane's runtime detection is authoritative: the agent starts
   as root in its container, detects the socket group and drops to PUID/PGID; the
   installer only reports the detected value, and compose uses `group_add` only
   for a manual value. This follows socket group changes after reboots or Docker
   upgrades. (b) the installer's detected value in `group_add` is authoritative
   and the agent starts directly as PUID/PGID.
3. **Reporting local checks after connecting.**
   Recommended: (a) add owner, write access and socket group as additive fields
   to the agent's existing version or system information response and show them
   in the connect step; this changes the agent contract, so the contract version
   and minimum agent version are raised once in #81. (b) a new agent endpoint for
   onboarding checks, which needs an [agent surface](agent-surface.md) review.
   (c) show only the installer's console output in M5 and change the reference
   text.
4. **Storage of stale values for a disconnected host.**
   Recommended: (a) in memory on the hub: stale lists survive a disconnect but
   not a hub restart; after a restart an unreachable host shows its `last_seen`
   time and no lists. (b) a SQLite snapshot table per host and list type, which
   needs a new migration and size limits.
5. **Install directory inside the project directory.**
   Recommended: (a) allow its own direct subfolder, as the reference does, with
   the agent excluding it from project discovery; default suggestion stays the
   setup value. (b) require the install directory outside the project directory
   and change the reference rule and texts.
6. **Where installation artefacts are generated.**
   Recommended: (a) in the browser with the reference generator; the hub supplies
   token, pinned image reference and hub address candidates, and Arcane's snippet
   endpoint is not used by the UI. (b) on the hub, by porting the generator to Go
   and replacing Arcane's snippet output; downloads then come from the hub.

## Review decisions

The maintainer decided the following after the first independent review of
this record on 2026-10-17. B1 to B3, S2 and S3 are in the
[Edge threat review](edge-threat-review.md#review-decisions).

- **B4. Token only in a private `.env`, #84.** The generator rules under
  [Installation artefacts](#installation-artefacts) apply to every artefact that
  carries the token. The accepted shell-history warning and browser-side
  generation (decision 6) stay.
- **S1. Manual socket group with runtime detection, #84.** Decision 2 (a) stays;
  the agent applies a manual `group_add` group together with the detected socket
  group, as described under [Docker socket group](#docker-socket-group).
- **S4. No overwrite of an existing stack, #84.** A direct subfolder of the
  project directory stays allowed (decision 5). Before any write, the host-side
  check rejects an install directory holding an unrelated stack and allows a
  recognised agent reinstall, as listed under
  [Checks before installation](#checks-before-installation).
