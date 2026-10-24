# HTTP adapter, live channel and capabilities

## Boundary and reuse

The production UI selects the HTTP adapter in `web/src/runtime.ts`; development
uses the demo adapter. The existing
`DockletAdapter` port in `web/src/data/adapter.ts` and `api.ts` defines calls and
`Topic` names. `live.ts` shares action, metric and log subscriptions;
`AdapterProvider.tsx` maps `subscribeChanges` topics to query invalidation;
`run-gate.ts` deduplicates local starts. The disconnected adapter rejects calls
and returns no invented data. The HTTP adapter belongs in `web/src/data/http/`
and keeps these boundaries rather than making features import wire DTOs.

Arcane v2.15.1 already has the multiplexed `GET /stream` SSE operation
`streamClient` for `activities` and `events`. Activity envelopes carry run
snapshots or messages; event envelopes signal a changed event query. The
adopted `backend/api/handlers/stream.go` and `types/stream/` provide this path.
Arcane's `backend/api/ws/` and `backend/pkg/libarcane/ws/` instead serve
per-resource WebSocket logs and stats. Reuse those WebSocket routes when their
adapter methods gain live domain mappings; do not duplicate them for M3 run
invalidation. Their Echo routes are absent from OpenAPI.

## HTTP calls and outcomes

`api/openapi.json` and `web/src/api/generated/schema.ts` are the checked-in
operation and DTO contract. There is no generated runtime client yet. A small
HTTP caller in the adapter uses generated `paths`/`operations` types, then maps
wire DTOs to the existing view models. Route presence alone does not make an
adapter operation available: its mapping and server capability must both exist.

| Adapter call or family | Current OpenAPI operation | Current handling |
| --- | --- | --- |
| `listActions`, run refresh | `list-activities`, `get-activity` per environment | Map recorded activity, message, step and output DTOs; enable the full `CodedActionRun` projection only where required fields have wire sources. |
| `listEvents` | `listEvents` or `getEventsByEnvironment` for the selected scope | Map `EventEvent` to `CodedEvent`; use recorded DTOs. |
| `subscribeActions`, `subscribeChanges` | `streamClient` (`activities`, `events`) | Parse envelopes and re-read changed snapshots/queries. |
| `listHosts`, `listStacks`, `getStack`, `listContainers` | `listEnvironments`, `list-projects`, `get-project`, `list-containers` | Read host identity and aggregate host-scoped projects and containers through the shared agent forwarding path. |
| `getSecurity`, `listSchedule`, `getSettings` | `get-environment-vulnerability-overview`, `get-job-schedules`, `get-settings` | The first two calls remain unavailable until M11 and M10. `getSettings` stays unavailable for unconnected sections; M4 reads connected sections with `getConnectedSettings`. |
| `runStackCommand`, `runContainerCommand` | `deploy-project`/`down-project`/`restart-project`/`redeploy-project`; `start-container`/`stop-container`/`restart-container` | Map the seven guarded operation routes with async admission, stable IDs and targeted hub detail. Enable only scoped stack/container descriptors with the complete operation family and activity-read grants; bulk hub controls remain unavailable. |
| `subscribeLogs`, `subscribeContainerLogs` | Echo project/container log WebSockets (outside OpenAPI) | Scoped log/read grants, JSON source mapping, bounded arrival buffers and conditional reconnect tail replacement. |
| `createVolume`, `deleteVolume`, `renameVolume`, `planVolumePrune`, `pruneVolumes` | Environment volume create/delete/rename and plan/prune operations | Connected mappings with host or volume descriptors; prune retains server planning and confirmation. |
| `createNetwork`, `removeNetwork`, `connectNetwork`, `disconnectNetwork` | `create-network`, `delete-network`, `connect-network-container`, `disconnect-network-container` | Connected hub routes for Direct and Edge, generated DTOs, scoped descriptors and topology/container refresh. |
| Other `DockletAdapter` reads, writes and subscriptions | Existing operation where listed, otherwise no matching operation | Expose unavailable until the owning milestone supplies the complete mapping. Existing WebSockets carry connected logs and stats. |

The HTTP boundary classifies an operation into these outcomes. Existing
`Promise<T>` port methods return the `ok` value or reject with the typed
non-`ok` outcome; the UI can use the same shape for error rendering.

| Outcome | Required fields | Meaning |
| --- | --- | --- |
| `ok` | `kind`, `value` | Confirmed server result, including `run.joined` and `run.fresh` when returned by admission. |
| `not-available` | `kind`, `operationId`, `reasonCode`, `resourceRef` | No request is executed; UI renders the action disabled with a localized reason. |
| `conflict` | `kind`, `code: "run.busy" \| "run.cancelling"`, `activeRunId` | Server refused admission; keep the active run visible. |
| `error` | `kind`, `code`, `status` when HTTP responded | Request or mapping failed; never turn it into an empty collection or successful action. |

`run.started`, `run.joined` and `run.fresh` are admission results. A cancelling
run remains `running` until completion. The HTTP adapter never exposes the
server's executable work context. The client-side `run-gate.ts` remains a UI
deduplication aid; server admission decides cross-tab conflicts and freshness.
Runtime controls use deliberate command admission without the generic freshness
shortcut, as defined in the [runtime record](runtime-control.md).

Raw run output is a retained debugging feature. The hub persists it per run in
`activities`, separate from coded messages: `backend/pkg/libarcane/activity/writer.go`
captures execution output before its message normalization, and
`backend/internal/activity/service.go` writes the original lines in batches with
their timestamp, stream, text and optional step/level. The stored output is
capped at **256 KiB of UTF-8 serialized lines per run**; on overflow, discard
the oldest complete lines (or the oldest UTF-8 characters of a single oversized
line) and set `outputTruncated`. The activity detail returned by `get-activity`
gains `output` (an array of these lines) and `outputTruncated` DTO fields.
The adapter maps the lines into web `ActionRun.output`; `RunOutput` shows a
localized truncation marker when `outputTruncated` is true. This requires a
schema change for the activity row, matching Go and generated OpenAPI/TypeScript
DTO changes, and an `ActionRun` truncation flag. The
[SQLite baseline](sqlite-database.md#fresh-migration-baseline-and-schema-proof)
includes the retained raw-output fields.
Output disappears with its run under the fixed 100-finished-runs-per-environment
retention; active runs remain. The detail response is also proxied from agents,
so these additive fields change the agent/hub contract and require a contract
version and minimum-agent-version review before release.

The web run states map directly from the wire, except `success`:

| Wire status | `ActionRun.status` | German label | English label |
| --- | --- | --- | --- |
| `queued` | `queued` | Wartet | Queued |
| `running` | `running` | Läuft | Running |
| `success` | `succeeded` | Erfolgreich | Succeeded |
| `failed` | `failed` | Fehlgeschlagen | Failed |
| `cancelled` | `cancelled` | Abgebrochen | Cancelled |

The activity feature catalogues supply these labels; queued and cancelled
render as distinct states throughout run surfaces, including the result view.

## Read resilience and request bounds

The shared identity-owned HTTP caller retries a GET exactly once when fetch
rejects before receiving a response with a TypeError or NetworkError. Both
attempts use the same abort lifetime. An aborted request, an HTTP status, a body
or mapping failure, and POST/PUT/PATCH/DELETE never enter this read retry. The
runtime command admission policy below remains owned by its command transport.

Each activity discovery uses at most four concurrent detail reads per host; each
host advances independently. Known activity notifications read their targeted
detail without waiting for discovery workers. Concurrent full activity reads
share work. Known-run log/progress messages refresh only their targeted detail;
they never invalidate discovery or event history. Unknown activities and terminal
state changes notify discovery; terminal `message` envelopes remain targeted.
Reconnect or detected loss notifies the same discovery loader.

Event history retains the complete history used by Dashboard and Activity Center.
Pages request the effective size of 100 and run strictly sequentially. Event stream
messages carry invalidation only, so they cannot supply incremental event rows.
Only events-channel messages and stream recovery invalidate history. HTTP and
mapping failures remain visible and a deliberate retry can start a new load.

Discovery, recovery and history share a generation-scoped single-flight policy: idle requests or notifications start a read; concurrent requests join it until a notification marks one pending read.
Each start clears pending, so bursts queue one read and notifications during any trailing read can queue one more; callers after a notification await the next result, earlier callers receive the current result.
Every start and publication checks the identity generation; replacement or logout cancels old waiters, rejects old results and prevents old trailing work from starting.

## Runtime command mapping

The [runtime-control record](runtime-control.md) defines the M6 specialization,
including the command specialization: command calls opt
into admission JSON and resolve the same durable hub/agent ID, preserve coded
conflicts (including request mismatch), and refresh targeted hub runtime detail.
A command keeps one opaque UUID across CSRF and at most one network retry, bounded
by ten minutes of local elapsed time. A final response discards that key. A lost
or malformed final admission remains explicitly ambiguous and prevents automatic
replacement, including after the browser gate's ordinary missing-run timeout.
Known IDs are refreshed through hub detail; list absence cannot settle them.
Detail reads while observed share in-flight work and a two-second cadence. Three
unconfirmed or failed reads stop automatic observation until explicit refresh or
an activity notification; confirmed active work stays observed until terminal.
Identity changes abort this consumer's requests and reject late callbacks.

Token preflight failures are pre-admission errors and allow a later deliberate
intent. The consumer identifies raw token-read transport failures at their source;
the existing CSRF client retains its header and retry policy. A definitive CSRF
refusal followed by a failed token refresh also leaves no admission hold, unless
an earlier command send already had an uncertain response. Such earlier uncertainty
remains held with the original key; a later denial cannot prove it never ran.

Remote unknown knowledge with no targeted recovery reason is unconfirmed delivery,
not outcome loss. A present `recovery.unknownReason` shows blocked uncertainty;
missing detail never manufactures either loss or completion. Cancel intent remains
nonterminal. Administrator release uses existing effective global-admin and
activity read/cancel permissions, explicit acknowledgment and a nonempty reason;
its failed/unknown disposition and later executor evidence remain separate.
Failed targeted detail during aggregate or single-run projection retains the
same host/run's cached recovery evidence, cancellation knowledge and current-intent
marker. A newer successful targeted answer replaces that observation under the
existing sequence guard. Identity reset clears this consumer cache. A terminal
unknown run without recovery detail says its outcome is not confirmed, without
claiming confirmation is pending or that an already released target is blocked.

Permission reads share in-flight work and cache validated successes for at most
thirty seconds of local elapsed time. Failed reads are evicted for the next access;
unknown grants are shown as unavailable rather than a fabricated server denial.
A failed rights read does not discard readable run evidence. Cancellation and
release still require the existing grants, and actual server denials remain
authoritative. No grant or authentication policy changes at this boundary.

Notification history seeds unseen rows of initially incomplete sources silently,
while already observed transitions and identifiable current claims/admissions
notify normally. The first complete recovery still seeds unseen history. No
cross-device clock comparison is used: indistinguishable unseen history and new
remote actions during initial incompleteness cannot reliably be separated.
Unanswered/lost-continuity executor knowledge stays visibly blocked; settled
local interruption fails with effect unverified and releases. Accepted admin
recovery is a separate hub disposition, never inferred success. Legacy CLI
NDJSON/message callers retain their response modes. Bulk `ContainerActionResult`
is not the wire source for single-container admission.

Runtime logs use the existing Echo WS routes. Subscription attempt plus seq
identifies lines; seq orders arrival within that source, not timestamps across
services or stdout/stderr. Resource changes clear hook state, reconnect establishes a tail
boundary and bounded buffers expose loss/truncation. Runtime logs remain separate
from retained run output. Local session revocation closes SSE and all viewer WS
connections through explicit closers and bounded durable session revalidation,
clears adapter/live caches and rejects late identity-generation callbacks;
accepted Docker work continues. Other tabs observe server closure/identity
rejection without a new broadcast mechanism.

The cross-host action-load failure in #102 requires a bounded, separately owned
projection prerequisite before M6 action UI activation: healthy runs plus explicit
source failures, targeted detail availability and reconnect recovery. No failed
source may become an empty successful read. Its owner controls shared partial-read
fields before runtime consumers adopt them; this does not redesign inventory.
The runtime foundation owns the other shared port/domain/capability additions
and required v4 execution/outcome contract changes; optional additive log metadata
belongs to the log owner. Opaque UUID retries use ten minutes of client-local elapsed time only. Hub
first-seen time is audit attribution, never an age rejection or replacement rule;
retained server bindings still return their original run. Compact outcome
evidence lasts at least twenty-four hours independently of full output history.
Supported delivery and receiver admission are bounded before acceptance; accepted
work survives caller timeout. Only recent, continuous-generation negative proof
may release a scope; old or uncertain absence stays unknown.
Only complete scoped command and log mappings enable capabilities. Browser
identity lifetime also governs every live consumer and query observation.

## Bounded runtime log mapping

The HTTP adapter subscribes to the existing hub-facing project/container Echo
routes, with `format=json`, `follow=true`, `tail=100`, `timestamps=true` and
`batched=false`. Direct and Edge selection remains behind the hub route. It sends
no browser credential to an agent. Only the matching scoped log and resource-read
grants enable each descriptor; the endpoint retains its existing log permission
and Origin checks. Existing text/default clients and 50-entry JSON batches remain
compatible. JSON arrays preserve received order; malformed mandatory fields stop
mapping visibly. Optional stream/truncation omissions map to unknown/false.

Docker frames are consumed in fixed chunks, with one bounded prefix per stream;
excess is discarded through the newline. Follow and non-follow use the same
incremental reader, without whole-output buffering. Completed lines serialize
arrival, including non-follow stdout/stderr; neither timestamp nor stream implies
chronology. Compose bounds both TTY and framed input before its own text writer.
Its existing inspect metadata supplies service and TTY identity, without loading
Compose/environment files. Structured viewers receive records before normalization;
legacy text uses the existing stderr marker. Only recognized wire semantic levels
become severity. Stderr alone stays informational and application text is retained.
This changes older structured stderr severity to `info`; optional `stream` identifies stderr.
Older agents remain accepted with unknown stream metadata and their recognized
semantic levels. No in-repository external consumer requires the former levels.
Compose v5.6.0 inspects the same full container ID before opening logs. Missing
inspect Config or unknown TTY metadata now fails visibly before opening a log
response, rather than passing an unbounded body into Compose. Recheck that ordering
when upgrading Compose. Docker system-error frames retain at most 1,024 bytes of
daemon diagnostic, stop without draining oversized frames, and never become
application rows. A failed shared source emits one server warning with bounded
kind/resource identity, omitting raw errors, credentials and request bodies.

The browser shares one source per resource/options and identifies entries with
resource, a monotonically increasing client connection attempt, and source seq.
An observed seq gap produces a localized entry. Reconnect clears old rows,
requests a tail and displays a boundary. The tail is supplied only when creating
a new server source: a source already held by another tab/user sends future lines
only, including on pause/resume or reconnect. The server does not retain a replay
buffer. Local listeners sharing the browser channel receive its bounded snapshot.
Equal text is retained. The log owner makes at most five delayed reconnect attempts
(1, 2, 5, 10, 30 seconds) before explicit retry. Valid delivery followed by thirty
seconds of continuous connection replenishes this budget; brief deliveries,
open-only connections and empty batches do not. It pauses offline and after identity
rejection. A stream close alone triggers a bounded identity read rather than
assuming logout. Last unsubscribe closes its socket and cancels its frame,
identity read and reconnect timer. The existing
adapter identity reset also clears this log owner's channels and paused-hook views;
the shared session registry supplies server closure, while adapter identity reset also clears metrics, actions and RunGate state.

The complete browser resource budget is 2,000 entries and 1 MiB of serialized
UTF-8 data. The published immutable snapshot and the mutable next-frame window
each reserve half (1,000 entries / 512 KiB), so even a suspended animation frame
cannot double that budget. Oldest complete entries are evicted visibly; each line
is at most 64 KiB including JSON escaping. The source has no other pending array,
and publishes at most once per animation frame. Pause retains only the current
resource snapshot. Resource/adapter/identity changes clear it. Clear view records
arrival IDs, so a later message with an older timestamp remains visible.

Source queues remain raw/mapped 256, broadcast 1,024 and per-client 256. The shared
hub keeps its drop and slow-client removal behavior. Seq is assigned before
broadcast; a final dropped line without a later seq cannot be detected. Logs are
ephemeral, with no durable cursor, content deduplication or lossless guarantee.
The separate retained command-output cap remains unchanged.

## Capability descriptor

The server is the source of truth for each operation's descriptor:
`{ operationId, available, disabledReasonCode?, resourceRef: { kind, id } }`.
The demo adapter synthesizes the same descriptor from its in-memory supported
operations and `DemoDb` fixture resources in `web/src/data/demo/demo-adapter.ts`.
`operationId` names the adapter operation, and `resourceRef` identifies its hub
or target resource without defining rights. An unavailable descriptor requires
a reason code; an available one has no disabled reason. The checked-in
`get-capabilities` operation serves descriptors for the requested resource ref.
The HTTP adapter exposes the descriptor to the UI, merges it with its implemented mapping, and treats missing
or unreadable entries as unavailable with a generic reason code. UI action controls read that result to set
`disabled` and show a translated reason; direct calls still return
`not-available`. Resource-right evaluation and policy remain M15's decision.

## Live channel and recovery

M3 subscribes only to `activities` (runs) and `events` (query invalidation)
through Arcane's existing `streamClient` SSE transport. A start, progress,
completion or completion message means “snapshot changed”, not guaranteed
delivery. `subscribeActions` re-reads the affected run/list via
`list-activities`/`get-activity`; `subscribeChanges` reports affected existing
`Topic` values, at least `events`, to `AdapterProvider.tsx`. No event is treated
as authoritative state or a replay log.

On first connect, reconnect, malformed envelope or detected event loss, re-read
the subscribed run and event topics, including the active run detail. This
recovers a committed completion whose event was lost in a hub crash. Where an
envelope names a narrower affected topic, invalidate that topic; otherwise
refresh the subscribed M3 topics. Preserve WebSocket logs and stats as separate
later streams. The deployment has one hub process per database, so no
multi-hub fan-out protocol is needed.

## Identity, settings and production routes (#27)

The 2026-10-22 whole-app audit measured 262 `import.meta.env.DEV` matching
lines across 47 non-test files, 59 empty production catalogue values per locale,
and 32 `QueryError` call sites (30 without the source error). The production
bundle guard already rejects demo, design-lab and test modules across the full
module graph, including modules removed by tree shaking.

M3 provides the HTTP adapter for events and activities. M4 connects `/auth/mode`,
`/auth/me`, local login/logout, password changes, profile language and setup status
through that adapter. Only `/auth/mode` selects the active policy; the legacy
`authLocalEnabled` value is not a mode source. Production login/setup routes use
complete catalogues. Route gates require sign-in and setup before the dashboard.
Local first-run setup changes the initial password before completion. Switching
an already configured hub to local also forces any outstanding password change.

`getConnectedSettings` reads General and Activity through
`GET /environments/0/settings`; section writes use `PUT` on the same path. Sign-in
mode is read-only. The process `TZ` drives backend schedules and is projected as
read-only `effectiveTimeZone` for `IntlProvider` and schedule display. General
explains that operators change `TZ` in the startup environment and restart the
hub. Setup stores agent-host project defaults under `agentProjectsDirectory`,
separately from the hub's own `projectsDirectory`. Important-action presentation
uses the stored Activity value. Profile language is stored per administrator via
`PUT /auth/me/profile` and applied on load. Other settings sections retain their
existing boundaries.

The UI exposes no editable history limits. Retention stays at 500 events and
100 finished runs per environment; active runs remain. Host onboarding and its
installer use the connected M5 routes and catalogues.

Unsupported reads preserve their typed error through `QueryError`: missing
adapter mappings render a neutral unavailable state, while transport/mapping
failures remain load errors. This includes the dashboard schedule (M10), security
(M11) and unconnected settings sections. Volume backup plans, history and actions
remain owned by M12. Their gates prevent unsupported reads and actions from
mounting; list/detail/storage-card badges say that backup status is unavailable,
counts stay unknown and backup navigation is disabled with a reason. A direct
backup URL also renders that reason. Missing backup DTO fields never establish
that a volume has no backup. Stack restore points retain the same M12 boundary.

The host-management owner supplies the remaining host edit, reconnect and agent
restart mappings; the runtime owner supplies per-service restarts. Until then,
these controls stay disabled with capability reasons. Update installs/checks and
automatic policies (M10), container origin assignment (inventory owner), source
imports (stack-authoring owner), backup target tests (M12) and notification tests
(notification owner) follow the same rule. Hub database backups keep their own
implemented access/maintenance contract; they are separate from volume backups.

Unchecked image results retain an incomplete update status instead of claiming
that every image is current. Product CPU/RAM accessible values, deployment empty
states, image confirmations
and host messages have complete English and German production text. Dashboard
app entries open HTTP/HTTPS links. Network query parameters cannot replace live
inventory with benchmark data; enlarged topology fixtures belong only to isolated
performance tests and the production graph guard rejects their module.

The production catalogue test evaluates both catalogues with `DEV=false` before
importing them. It rejects every empty string except the exact, reasoned list in
`production-empty-exceptions.test-support.ts`, and rejects stale exceptions.
The 2026-10-22 final audit has 51 such keys per locale, zero empty product values,
and 33 `QueryError` call sites all carrying their source error. The 263 `DEV`
matching lines in 48 non-test files retain these development-only categories:

- Preview and design-lab routes/catalogues, shell live/lab controls and fixture
  user identity; the production runtime removes those controls and uses sign-in.
- Simulation badges, failure-injection buttons, simulated onboarding connection
  controls and the development-only host installer sheet. Connected installation
  remains available through onboarding.
- Synthetic dashboard service widgets and their configuration. Product app links
  and personal dashboard layout preferences remain available.
- Demo run-text encoder messages and fixture-only report labels. HTTP activity
  mappers use wire codes and raw output, and never synthesize simulation markers.
- Explicit alternate development wording whose production branch is complete.
  The lab override note itself cannot render outside development.

The full production module graph guard rejects demo adapters, design-lab/dev/test
modules and benchmark generators even when later tree shaking removes them.
Its negative builds inject the demo adapter and design-lab modules to prove the
boundary. Production runtime route tests cover login to dashboard, hosts,
containers, stacks, volumes and file reads using HTTP DTO fixtures and the
sanitised recorded workspace envelope. They also cover unavailable capabilities,
backup deep links, blocked preview routes and ignored benchmark parameters.

## Contract test matrix for #38

Each numbered case runs against the demo adapter and HTTP adapter with the same
scenario; HTTP responses and streams use recorded DTOs, while the demo supplies
the equivalent view-model state. The last column names a mutation that must
make the case fail.

| # | Required observation | Red mutation |
| --- | --- | --- |
| 1 | Recorded activity and event DTOs produce the same coded run/event fields in both adapters, including steps, affected services and message codes. | Drop `affectedServices` or a message code in the HTTP mapper. |
| 2 | An unsupported operation is disabled with the server reason and direct invocation returns `not-available` without a request or run; the demo descriptor comes from its in-memory supported operations and `DemoDb` fixtures. | Return a synthetic action ID or omit the disabled reason. |
| 3 | A descriptor missing an entry or failing to load leaves that operation unavailable; the demo adapter uses its in-memory descriptor with an entry removed or a forced load failure. | Treat missing capability as available. |
| 4 | `run.busy` and `run.cancelling` preserve their code and active run ID; neither starts another action. | Convert a conflict into `ok` or erase `activeRunId`. |
| 5 | A transport or DTO mapping error renders as error, with no empty or successful fallback. | Catch the error and return `[]` or a fake ID. |
| 6 | Start, progress, terminal and completion-message notifications re-read the affected run; event invalidation refreshes the event query. The fixture commits a newer run detail than the stream envelope carries, so content is stale at notification time. | Trust the stale stream payload as final state or drop the `events` invalidation. |
| 7 | Reconnect and detected event loss re-read subscribed runs and events, revealing a committed completion that had no delivered event. | Remove reconnect refresh or omit active run detail. |
| 8 | The production route uses the HTTP adapter for M3 data and offers no simulated success or empty simulation label. | Select the demo adapter or restore the empty `activity.output.simulated` value. |
| 9 | Settings expose no editable history-limit fields. | Restore a numeric history-limit control. |
| 10 | Recorded raw execution lines survive a detail re-read and appear in `ActionRun.output` with their timestamp, stream and text. | Drop output from `get-activity` or map it to an empty array. |
| 11 | A run exceeding 256 KiB retains the newest output within the cap and displays a truncation marker. | Keep the oldest lines, exceed the cap or omit `outputTruncated`. |
| 12 | Pruning a finished run removes its output while a retained active run keeps its output. | Store output without activity-row ownership or delete active output. |
| 13 | Wire `queued` renders the distinct Wartet/Queued label and stays nonterminal. | Map `queued` to `running` or reuse its label. |
| 14 | Wire `cancelled` renders the distinct Abgebrochen/Cancelled label and is terminal. | Map `cancelled` to `failed` or reuse its label. |

## Host inventory observations

The hub retains successful project and container JSON reads in its process-local
hoststate store after caller authentication and resource authorization. Keys
include host ID, resource suffix and the normalized pagination/filter query.
`X-Docklet-Observed-At` carries the successful read time. On
`agent.unavailable`, `agent.invalid-token` or `agent.update-required`, a retained
response keeps that time and adds `X-Docklet-Host-Reason`. Fresh successful reads
replace the retained value and omit the reason. Other failures, including caller
authorization failures, never become cached successes. These headers are hub
projections and do not change the agent contract or its compatibility boundary.

The HTTP adapter gives inventory resources IDs of `<host-id>~<resource-id>` to
prevent collisions across agents; raw IDs are restored at the HTTP boundary.
Activity and event resource links use the same host-scoped IDs; events without
a host identity expose no project link.
The backend owns both IDs and neither contains the separator. The UI keeps
retained rows visible and displays their timestamp and distinct host reason.
Without a successful snapshot it shows the host reason and states explicitly
that no retained values exist. A hub restart clears snapshots. Ordinary reads
still pass through the compatibility check before either direct or Edge
forwarding; an update-required host receives no ordinary agent request.

Compose project tags map to stack labels; runtime containers supply service
status, health and published ports. Complete Compose project/service labels
identify Compose containers. Partial Compose metadata or evidence of an external
manager leaves origin unclear. Containers without these labels map to Run.
Recreation restrictions include Compose ownership, unclear origin and the
server's protected-container flag. Agent-labeled workloads are excluded from
both lists; the backend excludes their tracked Compose projects before pagination.
The seven guarded lifecycle commands, stack authoring, volume workspace writes
and network writes have connected mappings. Each write family checks its own
scoped capability before dispatch.

Images, volumes, network details/topology, published ports and dashboard reads
use the same retained-response store and observation headers. Dedicated agent
resources are hidden before pagination; shared resources and Docker's built-in
networks remain visible. Image consumers include stopped containers. Network
edges describe membership and configured Compose `depends_on`, never traffic.

The HTTP adapter reads host resource measurements through the existing
compatibility-gated `/ws/system/stats` forwarding route and collects container
samples through resource-sorted container reads. Percentages and byte units
are mapped explicitly. Host histories retain up to 60 observed samples in the
adapter session; a disconnect preserves the last sample with its time and host
reason. A new browser session has no retained metric samples. Unreported disk
capacity, growth, image metadata and volume sizes remain explicitly unavailable.
Image writes retain their unavailable mappings. Volume create, delete, rename,
planned prune and file workspace writes are connected; volume backup and restore
remain outside these mappings.

Network writes use `/api/environments/{hostId}/networks` for both Direct and Edge.
Create checks `createNetwork` at `{ kind: "host", id: hostId }`; remove, connect
and disconnect check their operation at `{ kind: "network", id: inventoryId }`.
The inventory ID contains the host and Docker network ID; transport restores the
raw network/container IDs. A host-scoped descriptor does not grant those three
network operations. Hub descriptors aggregate availability and do not authorize
a specific target. The Create toolbar control is enabled when any online host
permits create. It prefers the displayed host when permitted, otherwise another
permitted online host. Row, detail and dialog controls use their target refs.

Create maps enabled families, labels, driver options and auxiliary reservations
with generated DTOs, including explicit `enableIPv4=false` for IPv6-only networks
and the LAN parent under `options.parent`. The backend remains authoritative for
validation and delete protection. Topology takes built-in state from the network
summary's `isDefault` flag and keeps stopped endpoint IPAM reservations visible
and unavailable to pool hints. Individual deletion includes unused networks;
there is no network prune UI. See [network management](network-management.md).

Network commands report the existing `topology` and `containers` change topics,
which clear shared inventory reads and invalidate subscribed queries without
polling. Known `network.*` errors become translated domain errors, including
`network.overlap`; unknown codes retain the generic error path. A lost or
malformed write response reports `network.outcome-unknown`, instructing the user
to inspect state before retrying, without implying rollback or retrying the write
automatically. Network and volume writes also classify HTTP 502/504 with
`agent.unavailable` as outcome unknown and invalidate their inventories; these
proxy responses cannot establish that the agent did not apply the write.
Read failures keep their existing classification. Volume prune reports unknown
outcomes per submitted candidate. HTTP 409 `volume.exists` maps to the existing
translated volume name-conflict message for create and rename; unknown codes
retain generic handling. Results and toasts from a closed dialog, a previous host
or an expired browser identity are discarded.


## Browser session lifetime

The local browser session ID binds viewer handles through the process-shared
`backend/pkg/sessionlife` registry. SessionService supplies durable session and
user validation independently of token caches. Authentication contexts carry
that validator to SSE, local WebSockets and the Direct/Edge viewer bridges.
Registration precedes validation. Explicit service revokes close matching handles
after persistence; transactional password/recovery writes, deletion, expiry and
mode changes are detected by the existing 15-second SSE or 54-second viewer
cadence. Direct forwarding owns its one cancellable 54-second check. Viewer
closure releases both proxy legs and source references, preserving the host
Edge tunnel and application-owned runtime execution. API-key streams without a
session and proxy/none requests retain their existing credential/host lifetime.

The adapter's identity generation aborts old requests, rejects late responses
and captured callbacks, closes SSE/log/metric sockets and their timers, and
clears private runs, histories and buffers. RunGate entries and listeners are
reset on identity loss/replacement, including durable unresolved runs and
ambiguous intents without a known ID. Mounted live/query observers reattach for
a fresh identity; paused log views clear through the accepted log lifetime.
The provider cancels old queries and records `currentUser` as null on loss.
A socket close alone starts a bounded durable identity read before reconnect;
credential-check form failures remain inline. Known same-user renewal retains
its existing observation/notification policy. Server execution remains durable
and does not depend on any viewer lifetime.
