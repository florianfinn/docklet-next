# Agent HTTP surface

The agent and Edge agent register the same reviewed module set. The list in
`backend/api/api.go` is the registration source; agent Huma routes and all Echo
routes are captured by tests. The agent receives privileged host operations
from the hub. The hub validates user permissions before forwarding, and the
agent validates its manager credential. A compromised manager credential could
invoke the host operations below, so additions require an explicit review.

| Module | Host exposure and reason |
| --- | --- |
| health | Readiness endpoint for the hub and local supervisor. |
| version | Public version metadata for protocol compatibility. Only the configured manager credential on an agent adds project-directory ownership, effective write access and socket-group checks; the on-demand write probe is cached for 30 seconds. The hub never includes these checks. |
| project | Reads and changes Compose projects and project files; needed for stack management. |
| container | Reads state, logs and processes and controls lifecycle and configuration. |
| image | Pulls, removes, exports, patches and scans images; needed for image management. |
| image-update | Checks and records available container image updates. |
| volume | Reads and changes named volumes and workspace files; admits asynchronous backup/restore and repository checks, receives target metadata, reports references and verifies attached local recovery keys (contract v9). |
| network | Reads topology; validates create, protected deletion and membership changes on the Docker host (agent contract v6). |
| port | Reports host port use for deployment decisions. |
| upload | Accepts bounded uploads used by image and restore operations. |
| auto-heal | Reads and updates typed per-host Auto Heal settings (contract v10); effective environment overrides refuse writes. |
| settings | Reads and updates environment settings, including host operational configuration. |
| updater | Checks and applies configured host resource updates. |
| system | Reports Docker host state, runs Docker-wide lifecycle/prune operations, and exposes agent self-upgrade checks and trigger. A manager credential can replace the agent binary; hub backup and update-all remain excluded. |
| diagnostics | Exposes host actor diagnostics for support and fault investigation. |
| registry | Accepts registry credential synchronization from the hub for private pulls. Listing, editing and testing credentials are excluded; a manager credential can still replace agent pull credentials. |
| job | Exposes environment-scoped schedules, jobs and run controls for hub orchestration. A manager credential can trigger host jobs, retry runs and alter schedules. |
| activity | Exposes environment-scoped activity history, detail and cancellation. A manager credential can read task output, cancel work and clear history. |
| vulnerability | Scans images and records vulnerability findings and ignore rules. |
| WebSocket routes | Streams container/project logs, stats and shell traffic; the Echo route snapshot protects these non-Huma endpoints. |

Identity, users, roles, API keys, environment enrollment and management,
notifications, webhooks, dashboard, search and hub event ingress
remain hub routes. Agent mode also omits the browser UI, API documentation,
pprof profiling endpoints, test-only Playwright routes and manager Edge tunnel
listener. An Edge agent
connects outward; this allowlist does not open a public Edge listener.

## M10 PR1 image boundary

Contract 7/minimum 7 adds guarded pull/tag/load/remove/prune, persisted image
protection, write-only registry projections and read-only update information.
Interactive, system and scheduled image deletion share host admission and
protection checks; normal Compose pulls remain outside that lease. Only fully
validated Docker-save tar/gzip archives load; unsupported OCI archives refuse
before Docker. Internal retention references never reach registry lookups.
Registry management remains hub-only; only authenticated manager sync reaches
agents, with local key validation and atomic replacement. Agent upgrade requests
have an explicit target checked against the running hub on both forwarding paths;
agents reject blank targets. No update/restore/policy/schedule capability is added
by this PR1 boundary; PR2a/PR2b own that activation.

Vulnerability scanning entered the agent contract at v8; the current minimum is v11. The hub feature veto
covers local and remote vulnerability routes; agent feature-disabled and busy
refusals remain typed. Host-wide runs are agent-owned evidence read through the
run route. Agents refresh KEV/EPSS intelligence daily and register no local image
scan job. Legacy remote scan/risk jobs are retired before delivery.
See [vulnerability scanning](vulnerability-scanning.md).

Hub, Direct agent and Edge agent processes exit non-zero when their embedded
actor host ends unexpectedly, for example after a freeze outlasts the 90-second
registration deadline; otherwise it would keep serving HTTP while its scheduler
and Edge tunnel have stopped. The shipped hub compose file and the
Direct, Edge and mTLS snippets use `restart: unless-stopped`, so the container
restarts with a fresh registration. `GET`/`HEAD /api/health` return 503 once the
application lifecycle has ended, so the `health` command reports the state even
before the process exits. Intentional actor-host stops, such as during a hub
database restore, cancel the runtime context and are not treated as failures.

## Volume backup contract v9

Volume backup requires contract v9; the current minimum is v11. Overview, repository metadata/check, restore
admissions and target host routes are part of the allowlisted volume module in
both agent modes. Hub-only target CRUD and administrative key export are absent
from agent registration. The target-set receiver is agent-only and accepts the
current pairing credential again inside its transaction. Target sync carries
metadata and encrypted-at-rest credentials; it never mounts or tests a target.

The hub uses the existing authenticated Direct or explicit Edge command transport.
It sends a complete target set on changes, connect/pairing and before policy
writes; a policy request must match the accepted authority and set version.
Target tests return 202 before any mount and publish safe structured results with
the admitted configuration version. Capacity values can be absent. Mount/helper
failure diagnostics expose stable codes, with no passwords or daemon text.

Upload restore needs upload and backup permissions at the hub before forwarding;
local admissions apply both before staging. Recovery-key routes disable caching
and record an audit event containing only target identity. Generic volume routes
hide the credential-bearing target mount volumes.

Synchronous backup reads, download preparation and deletion participate in request
cancellation over Direct and the Edge command registry. Accepted backup, restore
and check workers keep their independent activity lifetime. Downloads also stay
registered with maintenance until streaming and helper cleanup finish. The agent
bounds framed engine output before payload allocation and serialized JSON before
response commitment; oversized responses never become truncated successes.

## Notification and Auto Heal contract v10

Contract 10/minimum 10 adds the typed Auto Heal GET/PUT surface on Direct and
Edge agents. Notification settings and provider tests stay hub-only; obsolete
Edge notification commands are removed. Host-side backup/restore failure and
Auto Heal events use authenticated notification dispatch with captured credential
and session identity, strict payload validation and hub insertion deduplication.
Older Auto Heal dispatch shapes are invalid. Shared agent-access refusals keep
their stable status and code envelope on both transports.


## Container and host operations contract v11

Version and minimum are 11. The fingerprint includes the strict recreate edit,
replacement result, origins, stop-project, stopped definition services, hub stop
setting and the reserved host/shell vocabulary. Version 10 and mismatched
fingerprints are update-required before ordinary Direct or Edge sends.

Both agent modes register recreate/redeploy and stack stop through the existing
runtime module and manager-authenticated execute/query transport. Origin
assignment/reset remain hub-only. The retired edit route refuses with
`update.path-retired`; headerless redeploy has the same refusal. Compose-child
redeploy is also refused at the retained webhook effect boundary.

`system.agent.restart` reserves POST `/api/system/agent/restart` in the Edge
command registry. This foundation registers no answering agent restart route;
host maintenance owns its implementation. Shell query and protocol v1 frame DTOs,
restart states/codes and host availability codes are fingerprinted in advance;
this does not enable their behavior. `/health` now includes the per-process
`instanceId`. Migration 014 reserves credential revision and restart-request
persistence for those owners. `stackStopMode` stays in environment 0 on the hub;
agent settings responses omit it and remote or agent writes refuse before
forwarding.
