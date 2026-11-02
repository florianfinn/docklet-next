# One-time core adoption

The binding agreement of 2026-10-10 adopts Arcane once as the Go operating core
for both hub and agent. Docklet Next maintains it independently afterwards.
Later upstream fixes, including security changes, require manual assessment,
provenance and tests. Ongoing upstream compatibility is not a goal. The former
Docklet hub supplies governance and behavioural references, not product code.

The planning snapshot is the source of record. Its decisions.md SHA-256 is
`e6fc63beee6458fef44b5dd9a0d0e410f4473a12cac5166263b071edb6035363`;
evaluation-plan.md is
`bc7c85606b7bc0df3ca0f80a57598f918d22c3f5309ff3c5a86af39d5617a554`.
This English consolidation preserves the approved scope; frontend wording is
superseded by [frontend consolidation](frontend-consolidation.md).

Arcane v2.15.1 at `095cb778daba1034e2a39961199e712a75d4ba29` is the examined
reference. M1 must confirm the exact adoption revision and sibling/external module
layout at its ready gate (R2), with complete sources, licence inventory and tests.
The design reference alone does not select a future adoption revision.

## Architecture boundaries

Use one Go binary in hub, agent or Edge agent mode. The hub owns identity,
persistence, scheduling, users, runs and live events. The agent owns the privileged
host boundary. Agent modes register explicitly allowlisted modules.

The target tree is backend/ (Go core), types/ (wire DTOs), third_party/arcane/
(if vendored), web/ (React), api/openapi.json (checked-in contract), deploy/,
scripts/ and docs/. Huma operations and Go DTOs produce OpenAPI and generated
TypeScript types. The HTTP adapter maps DTOs to UI view models; features do not
import generated DTOs. Host capabilities include reasons for unavailable actions;
backend checks remain authoritative.

SQLite is the only hub database under [decision #60](sqlite-database.md),
superseding R6's PostgreSQL choice. Database files require local persistent
storage. New installations use a fresh baseline; no Arcane, former Docklet or
PostgreSQL development databases or registrations are imported. Forward
migrations run at startup; shipped migrations remain immutable.

The product runs at the root of its own domain or subdomain (R12). There is no
sub-path deployment: the contract covers assets, router, API, WebSockets,
authentication and generated links together.

## Scope ledger

Each row corresponds once to the binding decisions table, including its removal
or limitation. Removal applies to UI, API, backend, exclusive jobs, settings and
dependencies; hiding a menu does not satisfy it. Scope limitations do not silently
remove shared infrastructure needed by retained features.

| Area | Adopt or extend | Removal or boundary |
| --- | --- | --- |
| Stack creation | Compose with nested Easy wizard and Advanced raw editor sharing one draft | No initial choice of three creation modes; no Arcane template catalogue |
| Import | Docker-run conversion and GitHub/DockerHub import inside Easy; prefill known values | One-time import only; no permanent Git connection or Git Sync |
| Compose coverage | Broad forms; preserve additional raw fields | Never discard unknown fields on switching; completeness remains an evaluation gate |
| Standalone containers | Arcane management, editor/recreate with warning | Subject to the origin restrictions in the next row |
| Unraid or uncertain origin | Automatic detection, Unraid as its own origin, and manual per-container assignment | Restricted on uncertainty; Unraid permits status, logs, shell, usage and start/stop/restart, not editor/recreate |
| Users and sign-in | No login, local admin or reverse proxy; optional additional users with simple resource rights | No granular per-feature permissions or large role editor; rights are milestone M15 |
| Simple resource rights | Full assigned-application access including updates/editor, or limited lifecycle/logs/shell | Full access is not a promised security boundary for untrusted users; evaluate mount constraints |
| Dashboard | Separate landing page for metrics, problems, updates, host status and optional service widgets | Separate from application inventory |
| Widgets | Select, hide and reorder standard widgets | No freely resizable initial dashboard; later approved whole-grid-cell sizing is the React reference |
| Application inventory | Stacks only, dynamic existing label chips, dedicated stack pages | No expandable stack rows or standalone containers in this list |
| Stack page/editor | Information, combined logs and Easy/Advanced editor in one workspace | No detached editor as a separate main section |
| Host filter | Navigation filter; All Hosts by default; shared host-related lists and metrics | No mandatory host switch; hub settings remain unfiltered |
| Profile/hub settings | Bottom-left user icon opens settings/profile | Detailed menu follows the approved UI |
| Deployment flow | Action-specific steps, phase, progress, result and affected services | Exact step sequence must reflect the actual action |
| Stack logs | Combined service log stream with visible source | Individual container logs cannot be the sole stack log access |
| Swarm | Ordinary Docker/Compose multi-host management remains | Remove Swarm completely from UI, API and backend |
| Images/networks/volumes | Full management, particularly topology | Navigation grouping follows the approved UI |
| Image builds | Compose build within a stack | Remove standalone build workshop and its exclusive functions |
| Updates | Detection/manual installation; optional scheduled installation per stack/container | Check only by default; never install automatically without activation |
| Recovery after updates | Show failures and offer previous image/configuration for manual recovery | No automatic rollback and no promise to restore application data |
| Backups | Volume/system backups, manual/scheduled restore and retention; add bind mounts | Remove S3 |
| Backup destinations | Local directories, existing host mounts and dedicated SMB/NFS connections | Evaluate exact permission and mount behaviour |
| Vulnerability scans | Optional manual/scheduled scans, findings and dashboard widget | Keep distinct from image-update detection |
| Notifications | Retain Arcane channels; choose channel when adding a target | Show configured targets only, no permanent tabs for unused channels |
| Auto Heal | Optional bounded unhealthy-container restarts with limits/exceptions | Unexpected stops use Docker restart policies; no additional Docklet self-healing for them |
| Files | Project files, volume browser and bind-mount files accessible from editors | Own volume page versus direct access follows approved UI |
| Variables | Ordinary stack .env and service variables | Remove central variable editor entirely; no .env.global extension |
| Host connection | Direct by default; optional Edge connects outbound to hub | No public Edge agent port; remove Docklet-managed WireGuard requirement |
| Host onboarding | Clear fields, visible project directory, Compose command/download/copyable installer | Distinguish installation and project directories |
| Host file permissions | Installer checks rights and suggests UID/GID; advanced override including Compose download | Socket group detection stays automatic and distinct from file PUID/PGID |
| Proxy | Later Traefik/Caddy evaluation; existing installation/migration, HTTP/HTTPS routes, targets, middleware and certificates | TCP/UDP deferred; project base and precise integration remain open |
| Games | Later Docker Attach/RCON console plus status/player count; general management handles lifecycle/files/backups | Special game administration and external project choice remain open |
| Activities | Compact top-right progress popups and explicit activity centre with details/errors/history | No automatic sidebar opening for every action; combined event/run history is evaluated |

Notification channels are email, Discord, Telegram, Signal, Slack, ntfy, Pushover,
Gotify, Matrix, Google Chat and a generic channel.

Modules not explicitly named in the cut, including API keys, webhooks, registry
browser and CLI, are kept initially and evaluated individually in M1 (R5).
Neither an unspecified removal nor unconditional long-term retention is implied.

## Cross-cutting product decisions

Personal language, colours, views and dashboard layout persist in the hub database
on the user profile, or as hub preferences without login (R7). Browser storage is
only provisional. Design-code import/export remains. Identity/admin belongs to
M4; additional resource rights extend its port in M15 (R11). Simple users and real
service widgets are first-release requirements (R8). Every signed-in user sees all
widgets; configuration and credentials remain admin-only (R10). Credentials and
service polling remain server-side. Proxy and Games remain explicit later previews.

Runs persist phases, service effects and message codes; completion happens exactly
once server-side and independently of history pruning. Retain 500 events and 100
completed runs by default, never pruning running operations. Docker state is read
from the host, not inferred from desired state stored in the database.

Topology shows structural connections, status and utilisation. It does not show
throughput; animation is decorative, not measured communication. Network membership
and measured traffic are different facts. Use selection and the information panel
for readable addresses. Arcane supplies the security presentation reference;
Docklet supplies shared host views, onboarding and targeted workflow extensions.

## Evaluation order

Before adoption: source/licence map and functional cut, then host/file operations,
Compose synchronisation and origin protection, UI acceptance, module boundaries,
and technical integration. Each milestone's ready gate resolves or explicitly
excludes its open questions. Unraid classification is decided in
[container origin](container-origin-recreate.md). Remaining evaluations include
lossless Compose/invalid YAML handling, import metadata, mount restrictions,
SMB/NFS consistency, topology sources, navigation and combined activity history,
and Proxy/Games integration. No demo result proves a live backend implementation.

## Decision ledger

Later decisions that reverse, confirm or extend this record, newest first. Each
entry names the owning milestone; the scope ledger above stays the target, and
narrower behaviour in an older note is an interim state until that milestone.

### 2026-11-01: M16 bind mounts and backup completion

The maintainer fixed the M16 scope ([bind mounts](bind-mounts.md),
[backup completion](backup-completion.md)). Single-file binds get the approved UI's
single-file mode with in-place writes and backups; bind sizes come from an on-demand
scan and the latest snapshot. Only paths a workload container mounts are admitted,
system, Docker, Docklet and target paths are denied including their ancestors, and
overlapping paths are not mutually locked. Hub archive copies go to hub-reachable
targets only; deleting an archive by hand removes only the local file. Direct and
hub-local backup downloads become browser downloads. No schema migration; agent
contract v12.

### 2026-10-30: release roadmap and approved-interface review

The maintainer approved the release roadmap M14–M21 and reviewed every
deviation of the implemented product from the approved React interface
([frontend consolidation](frontend-consolidation.md)). Release target: every
page and action of that interface works live against hub and agents, together
with the scope above, except the Proxy and Games previews, the design lab,
simulation controls and the confirmations below. No new function starts after
M20.

| Milestone | Theme |
| --- | --- |
| M14 | Container and host operations |
| M15 | Simple users with resource rights |
| M16 | Bind mounts and backup completion |
| M17 | Dashboard, history and service widgets |
| M18 | Updates and schedules as approved |
| M19 | Import, bulk actions and settings parity |
| M20 | Pre-release hardening |
| M21 | First release readiness |

Reversals and additions (the approved interface applies):

- **M14**: container shell in the web UI; Unraid as its own origin with badge
  and filter, its dashboard ring segment off by default; manual origin
  assignment; recreate and edit of standalone containers; single service
  restart; Stop runs `compose down` by default with a hub-wide switch to
  `compose stop`, and a stack that is down stays listed as stopped with its
  services; host edit, check, reconnect, agent restart, agent components and
  Settings › Hosts; installation files per host in production; truthful host
  availability. See [container and host operations](container-host-operations.md).
- **M15**: two fixed rights levels per assigned application; a reverse-proxy
  account becomes a user (without email or group headers); sign-out in every
  sign-in mode, with a configurable logout address for the proxy; personal API
  keys with expiry and revocation in the profile.
- **M16**: bind-mount files in the same browser as volumes and bind-mount
  backups, under shared path rules; a backups overview with metrics, an
  unprotected marker, back up now per row and a targets panel; target
  connection tests while drafting; a default retention count for new plans
  (no size limit); hub database archive copies to targets.
- **M17**: service widgets (Plex, Overseerr, Minecraft) and an app links widget;
  personal preferences on the user profile instead of browser storage; health
  and resource history for containers, stacks and hosts with capacity trends
  and sustained-load alerts.
- **M18**: updates check only by default, every 12 h, for all stacks and
  containers; one global check interval (6, 12 or 24 h) and daily or weekly
  windows per stack and container; the `autoUpdate` master switch and the
  per-host cron jobs are removed; "Check all now" and the approved settings
  layout; standalone container updates through recreate with a policy per
  container, next to stack updates; single service updates next to whole-stack
  updates; restore points also before deploy and redeploy, offered for recovery
  after a failure; a selectable hub time zone (`TZ` stays the initial value
  until a choice is saved; remote hosts keep their own zone for volume backups);
  one daylight-saving rule for scans, backups and updates (a time in the spring
  gap runs one hour later, the autumn fold runs once); a manual-only scan mode.
- **M19**: one-time import from GitHub and Docker Hub inside the Easy wizard;
  bulk start, stop, restart, redeploy and update; image architecture, OS and
  layers; several named notification targets per channel; a "container
  unhealthy" event independent of Auto Heal; an Auto Heal cooldown of 1–60 min
  (default 10) next to the window limit; notification links follow the link
  base setting; configurable activity retention; global search with Ctrl/⌘+K
  over visible resources and settings.
- **M20**: webhooks and the remote CLI are removed (the hub binary takes over
  `generate`); Proxy and Games appear as "coming soon" without function or
  simulation.
- **M21**: fresh migration baseline, pinned images with SemVer and digest, the
  installation package, documentation and release acceptance.

Confirmed as implemented (no change):

- the one-time backup mode stays removed; manual plans cover it;
- a backup target's location is fixed after creation;
- the sign-in mode remains startup configuration from the hub's Compose
  environment and is shown read-only;
- "Scan now" covers images used by containers;
- Auto Heal stays per host in its own section;
- a host with running workloads can be removed after a warning;
- equivalent or additive differences: language in the profile, installer
  variable names with the token only in `.env`, additional host and run states,
  log boundary markers, forced password change at setup, network extensions,
  protected images during prune, five retained restore points, typed channel
  forms, Auto Heal exclusions by name and read-only environment-managed hosts.

Contradictions resolved by this entry:

| Topic | Narrower note | Resolution |
| --- | --- | --- |
| Update default and standalone updates | [images and updates](images-updates.md) starts stacks `off` and gives standalone containers update information only | Check-only default and container updates in M18 |
| Recovery after failures | restore points only before updates and restores | Also before deploy and redeploy in M18 |
| Import | [stack authoring](stack-authoring.md) keeps the GitHub/Docker Hub import as a demo | Live in M19 |
| Bind-mount files | no bind-mount browser | M16 |
| Unraid detection | the web maps Unraid labels to an unclear origin | Own origin in M14 |
| Time zone | [language](language.md) promised time-zone settings; the hub reads `TZ` at startup | Selectable zone in M18 |
| Local backup repository | the M12 milestone text said the built-in repository is re-keyed | It keeps its existing password as recovery key and is never re-keyed automatically ([volume backups](volume-backups.md)) |
