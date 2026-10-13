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

PostgreSQL is the only database (R6). SQLite migrations, drivers and tests are
removed in M1. New installations use a fresh baseline; neither Arcane nor former
Docklet databases or registrations are imported. Forward migrations run at startup.

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
| Unraid or uncertain origin | Automatic detection and manual per-container assignment | Restricted on uncertainty; Unraid permits status, logs, shell, usage and start/stop/restart, not editor/recreate |
| Users and sign-in | No login, local admin or reverse proxy; optional additional users with simple resource rights | No granular per-feature permissions or large role editor; rights remain a separate milestone |
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
excludes its open questions. Remaining evaluations include Unraid classification,
lossless Compose/invalid YAML handling, import metadata, mount restrictions,
SMB/NFS consistency, topology sources, navigation and combined activity history,
and Proxy/Games integration. No demo result proves a live backend implementation.
