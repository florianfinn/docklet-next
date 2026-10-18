# Runtime control and log lifecycle

## Boundary and baseline

This record defines the target M6 contract for one hub on local SQLite and one
self-hosting operator. It extends [run admission](run-gate.md) only for runtime
controls and outcome reconciliation. It does not change backup/job coordinator
recovery, the hub lifetime lock, identity modes or resource-right policy.
Implementation follows independent design review. Conscious administrator
recovery and the separately owned action-projection prerequisite below are
accepted scope; neither introduces failover or automatic effect replay.

| Web command | Existing operation and suffix under `/api/environments/{id}` | Existing authority |
| --- | --- | --- |
| Stack start | `deploy-project`, `/projects/{projectId}/up` | `projects:deploy` |
| Stack stop | `down-project`, `/projects/{projectId}/down` | `projects:down` |
| Stack restart | `restart-project`, `/projects/{projectId}/restart` | `projects:restart` |
| Stack redeploy | `redeploy-project`, `/projects/{projectId}/redeploy` | `projects:deploy` |
| Container start/stop/restart | `start-container`, `stop-container`, `restart-container`; `/containers/{containerId}/{command}` | Matching `containers:start`, `containers:stop`, `containers:restart` |

Stack start retains Compose deploy/up behavior, including preparation and image
work. Stop retains Compose down with volume removal disabled; it can remove
containers and is not Docker stop. Restart uses the existing whole-project
restart. Redeploy retains the adopted pull/deploy workflow. Do not implement a
new existing-services-only start or infer semantics from a route's summary.
`backend/internal/project/{module,handler,service}.go` and
`backend/internal/container/{module,handler,service}.go` are the current sources.
M6 does not enable service restart/recreate, bulk lifecycle, kill/pause,
update/restore, workspace editing or new destructive options. Existing registered
operations do not expand UI capabilities. Legacy options on a selected route
still take its whole-resource guard; the M6 command form accepts no service list.

## Current gaps that determine the design

At the M5 baseline, `EnvironmentMiddleware.Handle` in
`backend/internal/middleware/environment.go` forwards remote controls before the
local Huma handler runs. `backend/pkg/libarcane/activity/handler.go` returns an
empty ID and the original context when `StartHandlerActivity` persistence fails.
Neither that helper nor the browser's `web/src/data/run-gate.ts` is guarded
admission. The current `backend/internal/activity/gate.go` compares one equal
scope; it has no parent/child exclusion. `repair.go` currently fails orphaned
activities and frees their scopes. None is evidence of remote effect completion.

Single-container handlers return `MessageResponse` with an optional activity ID.
`types/container.ActionResult` (OpenAPI `ContainerActionResult`) is used by bulk
system handlers, with inner partial-failure semantics. It is not a single-action
success or a substitute for an admitted run. Project up/redeploy stream NDJSON;
down/restart use synchronous messages. These callers must survive the new guard.

## Admission, identities and scopes

The hub is the sole public admission authority. Its exact seven lifecycle route
matches are intercepted **before generic environment forwarding**, after original
caller authentication, authorization and browser request protection. Both local
and remote targets enter the same runtime admission service. Agent-only execution
and outcome routes use the existing manager credential and compatibility gate;
a browser-supplied dispatch envelope is rejected, not forwarded as trusted state.
Public legacy lifecycle requests cannot bypass admission by omitting a header.

The hub assigns one random UUID `runId` and commits its activity and runtime
receipt before dispatch. Remote agent acceptance stores that same UUID; no
second user-visible activity identity is created. Database environment identity
is the registered host at the hub and `0` at its executor. The hub rewrites the
source host on projection. Browser resource IDs remain `<host>~<raw-id>` and are
decoded only at the adapter boundary. Gate strings contain canonical raw IDs,
never display names or browser-composed IDs.

A client creates a timestamp-bearing UUIDv7 `Idempotency-Key` per deliberate
command. Its timestamp fixes a ten-minute retry window; the hub rejects expired
keys with HTTP 410 `run.retry-expired`, and malformed or more than one minute
future-dated keys with HTTP 400 `run.invalid-key`, before admitting work. Client
clock errors require correction, never automatic creation of a replacement
intent. The adapter retains the key across CSRF/transport retries only within
that window, discards it after the final response or window expiry, and then
refreshes known run detail rather than replaying the command. These are target
key rules, not current adapter support.

The binding uniquely maps `(environmentId, idempotencyKey)` to run ID, requesting
caller and canonical digest: operation ID, raw target, accepted options and
parent. Within the window, same key/digest returns the same ID and compact
outcome even after output/history pruning; recheck caller identity and current
operation/activity-read permissions. A changed digest is HTTP 409
`run.request-mismatch`, without a worker. Two permitted callers issuing the same
active operation join the first run. The activity/receipt keeps its original
actor; each joining key records its own caller and join audit attribution.
Joining reveals only the authorized run projection and grants no extra cancel
permission. It never creates an executor or changes the original actor.
Legacy requests without a key receive a hub-generated key; an ambiguous
retransmission without that key cannot be deduplicated and is never automatic.

Known key retries and conflicts against held scopes are resolved from durable
receipts first, without requiring a reachable executor. Before admitting new
work, the hub reads current target/Compose identity, not retained inventory.
A failed read refuses new admission. The executor validates identity
again before effect. Container IDs use inspected full IDs. A known Compose child
binds to its persisted project ID; unresolved Compose ownership refuses control
with `runtime.parent-unresolved` rather than inventing an unparented scope.
Docker calls outside Docklet remain an accepted external interference boundary.

| Target | Stored primary scope | Conflict predicate in the same environment |
| --- | --- | --- |
| Stack | `stack:<projectId>` | Same stack or any active child with that parent |
| Container | `container:<fullContainerId>` plus nullable `parentProjectId` | Same container or active parent stack |
| Two sibling containers | Distinct container scopes, same parent | May run concurrently when no stack run is active |

One short `BEGIN IMMEDIATE` transaction reads all relevant active runtime
receipts and inserts the receipt, activity and key binding atomically. Existing
active-scope uniqueness remains a second defense for equal scopes; the
transaction's parent predicate provides the additional exclusion. No transaction
spans network, Docker or Compose work. Dispatch registration uses the same local
mutex boundary as committed admission. Unknown receipts participate in conflict
checks. Only the hub administrator disposition below may terminalize its
activity while executor knowledge stays unknown; ordinary waiters cannot. Agents enforce the
same predicates before executing, including legacy requests reaching their local
listener. This second guard protects local callers; it is not a distributed lease.

`gate_key` is the operation ID plus digest of semantic options. Active same-key
intake returns `run.joined`; another operation/parent conflict returns
`run.busy`; same operation cancelling returns `run.cancelling`. Runtime commands
never use the 30-second `Check` freshness shortcut: start/stop/restart/redeploy
are deliberate effects. Other run-gate users keep their existing freshness rule.

## Durable receipt and writers

A new forward SQLite migration adds a runtime receipt and idempotency bindings,
separate from retained activity output. Use the same schema on hub and agent;
local execution has one receipt, not a mirrored pair in the same database.
Minimum fields are run ID, environment/registration identity, executor storage
generation, original caller, operation/target/parent/digest, admission time,
dispatch deadline/state, executor state, cancel intent, result/error and last
reconciliation time. Key bindings contain caller, digest and retry expiry.
Storage failure refuses admission before effects.

Keep full activity/output retention at **100 finished activities per environment**;
clearing history may remove terminal output immediately. Separate compact
receipts/key bindings/refusal tombstones survive either operation without a
cascading activity FK. Unresolved active and unknown receipts never expire or clear with
history. Explicit administrator disposition makes the hub receipt terminal for
retention purposes while preserving that external effect knowledge is unknown. Terminal receipts, joining bindings and not-received tombstones have a
**24-hour minimum retention** from their final write (including a later binding),
and cannot prune before all bound retry windows and dispatch deadlines expire.
A bounded cleanup batch runs at startup and hourly, deleting eligible compact
records deterministically by expiry then ID. This bounds terminal evidence by
recent traffic, not by installation age; active/unknown evidence remains until
settled or explicitly disposed. No full output is retained for this grace.

An authenticated dispatch carries immutable issued-at/not-after timestamps,
with at most **30 minutes** validity, and binds manager, registered target,
executor storage generation, run ID and digest. Each direct/Edge request also
has a maximum 30-minute lifetime; the generic proxy timeout alone is insufficient.
Before creating a worker, the executor refuses expired envelopes with HTTP 410
`runtime.dispatch-expired`, invalid/future-dated envelopes with 400, and wrong
identity/generation with 409. Permit at most one minute of future clock skew;
clock checks govern envelope validity only, never effect ordering or gate release.
Pruning atomically advances a durable expired-deadline cutoff before deleting
records; reject any envelope at/below that cutoff even after clock rollback.
Only delete records whose dispatch deadline is at/below that cutoff. The hub
uses the analogous expired-key timestamp cutoff when pruning key bindings.
These are two scalar cutoffs per store, not a per-command permanent ledger.

After compact evidence expires an opaque old run UUID is no longer recognizable;
a query reports evidence expired/absent, never historical success. Timestamped
keys and dispatch deadlines refuse late execution without retaining every UUID.
Within the guarantee window, retries/query/tombstone races must be atomic.
Outside it, callers refresh actual state and make a new deliberate intent.
Host removal refuses while active/unknown receipts remain, with links to cancel,
refresh or administrator recovery. Once disposed, removal may delete the target
and full history but retains compact evidence through its grace, keyed to the
removed registration. Retry against that removed target is HTTP 410
`runtime.target-removed`, never dispatch; it may return the authorized old ID.
Re-registration gets a new identity and cannot inherit old commands or conflicts.

Hub timestamps use its bound UTC microsecond clock, one sample per transaction.
Agent execution times use the agent's clock and are labeled executor times;
never compare clocks to infer execution order or release a scope. Local monotonic
timers bound work/query waits. Progress writes reuse the activity writer and
bounded owner-write context. Receipt terminal state and the corresponding local
activity/final message commit together, conditionally once. On the hub, applying
a remote terminal receipt and releasing the gate commit together. Live events
follow commit and can be lost; snapshots remain authoritative. A retry of a DB
write does not invoke Docker again.

| Receipt transition | Sole writer and precondition | Activity projection and scope |
| --- | --- | --- |
| Absent → `admitted` | Hub admission transaction | `running`, `run.started`; held |
| `admitted` → `dispatching` | Hub commits before any remote send | `running`; held |
| Absent executor receipt → `accepted` | Agent commits same ID/digest and local gate before registering worker | `running`; held |
| `accepted` → `executing` | Executor checks cancellation, then starts work once | `running`; held |
| Active → cancellation requested | Hub persists intent; executor persists it before signaling registered context | Still `running`; held |
| Active → terminal `success`/`failed`/`cancelled` | Executor has settled work and persists actual outcome | Matching existing activity state; releasable |
| Dispatch/active → `unknown` | Hub has no authoritative executor answer, or storage/host continuity is lost | `running` with explicit unknown reason; held |
| Abandoned local executor → terminal `failed` | Executor startup proves old process ended, or worker/known children settle after interruption | Interrupted, effect unverified; release after durable commit |
| `unknown` → active/terminal | Hub applies authenticated matching executor answer | Held until confirmed terminal commit |
| `unknown` → manually released | Administrator recovery procedure below | `failed`, interrupted/unknown reason; release audited |

`unknown` is receipt outcome knowledge, not a new terminal ActionRun status.
Add `outcomeState: known|unknown` and `cancelRequested: boolean` to runtime
snapshot projection; preserve queued/running/success/failed/cancelled statuses.
Validate both fields only for runtime activities; unrelated historical rows need
no invented executor knowledge.
An unknown result renders visibly as outcome unknown, never a claim of progress
or success. A manual hub disposition can project failed while executor knowledge
remains unknown; it records the hub writer, reason and acknowledged release
separately. Agent terminal receipts remain immutable. Hub manual release
is a separate disposition and must not overwrite a later recovered executor
result; show that result as late evidence without reopening or rewriting history.

## Dispatch, disconnect and reconciliation

Add agent-only `POST /api/environments/0/runtime-runs` (`execute-runtime-run`),
with the bounded identity/deadline envelope above and attribution, plus
`GET /api/environments/0/runtime-runs/{runId}` (`get-runtime-run-outcome`) and
`POST .../{runId}/cancel` (`cancel-runtime-run`). Query/cancel have hub-facing
`{id}` equivalents served locally from the hub receipt, with no generic proxy
bypass. The hub's worker uses existing direct/Edge request transport to the agent
route; token stripping/injection, compatible-host checks and Edge-only no-direct
fallback stay unchanged. An execute acknowledgment is bounded JSON, not an open
Docker execution stream. A duplicate execution envelope returns the persisted
receipt within its retention window, never invokes the executor again. Expired
envelopes are refused even after that receipt was pruned.
Execution accepts only the seven canonical operations and bounded validated
options, never arbitrary routes or commands. An authenticated agent admission
refusal proves this dispatch started no worker; the hub records a failed/refused
run with the agent conflict reason and releases its own admission. A transport
error while receiving that refusal is uncertain and keeps the scope blocked.

Agent outcome query is authoritative only for the matching run/digest,
registered executor and continuous storage generation. Valid answers use HTTP
200: `active`, `terminal`, or `not-received` backed by a refusal tombstone.
Query of a never-received ID atomically inserts that tombstone before answering;
a racing/delayed execute then refuses, so the hub may fail as not dispatched and
release. Query includes the original dispatch deadline; after evidence expiry,
absence alone is `unknown` (`evidence-expired`), not proof of no old effect.
Use 401/403 for authentication, 409 for identity/digest/generation mismatch, and
503 for an unreadable store. Offline transport is no answer.

The new v4 storage-generation identifier is persisted outside activity pruning,
returned by authenticated executor metadata/query and bound before dispatch.
A normal restart preserves it. Lost/replaced storage gets a new generation;
restored/stale storage cannot assert uninterrupted evidence. Existing host IDs,
tokens and version fingerprints do not prove this continuity. An empty new store
must refuse an old-generation query/execute, never manufacture not-received for
it. Undetectable external database rollback is outside the guarantee: after
restore or uncertain continuity, stop intake and use administrator recovery
before trusting negative evidence. No generation change proves an old executor
stopped. The foundation owns this evidence contract; executor readiness enforces
it. This adds no lease, second hub or workflow replay.

An active/terminal answer carries the existing activity snapshot/detail vocabulary
(progress, affected services, coded steps/messages and capped execution output),
plus receipt knowledge fields. Runtime retained output lines also carry a
monotonic executor-local `outputSeq` to avoid repeating progress on repeated
queries and a retained-start cursor to detect eviction before a waiter catches
up; these serve the existing remote CLI, separate from runtime-log WS seq. Replace snapshot output as a bounded window, and relay
only unseen sequence entries to legacy waiters. If entries were evicted before
a waiter read them, emit an explicit truncation output marker. No lossless
execution-output history is promised beyond the existing 256 KiB cap.
The hub applies executor snapshots to its local
runtime activity with the existing conditional activity writer; terminal apply
and receipt release are one transaction. While a known active worker exists,
query at most once every two seconds; an unavailable answer enters the bounded
reconciliation cycle below. Publish existing hub activity SSE notifications after
projection commits. No new SSE channel or remote-event delivery guarantee is
assumed. Hub activity detail/cancel first resolve an owned runtime receipt before
existing remote fallback; list aggregation deduplicates `(host,runId)` when agent
history contains the same identity. Agent history or live messages alone cannot
release a scope without the matching settled receipt.

| Failure point | Required behavior |
| --- | --- |
| Hub cannot commit admission | Fail request; no dispatch and no Docker call |
| Hub has admitted but has not committed `dispatching` | After restart it can prove no send was allowed: fail as interrupted/not dispatched and release |
| Direct/Edge disconnect before acknowledgment | If `dispatching` is committed, acceptance is uncertain even if send reports failure: hold and query by ID |
| Lost acknowledgment after agent acceptance | Agent continues on application lifetime; hub queries same ID and adopts active/terminal result |
| Browser disconnect/logout | Detach viewer only; hub worker and accepted agent work continue |
| Remote worker returns a settled error | Persist failed result, project it, release; failure is not rollback |
| Hub restart | Before readiness, fail abandoned local-executor work as interrupted and release; restore remote exclusions and query them, never resend effects |
| Agent restart with accepted/executing receipt in continuous storage | Before readiness commit failed/interrupted, effect unverified, and release its local exclusion; hub applies that authoritative answer; do not restart workflow |
| Terminal commit or final message fails | Keep exclusion; return persistence error and reconcile later; reuse existing bounded DB retry classifier |
| Terminal commit succeeds, event/response is lost | Query returns same terminal result; conditional projection adds one final message |
| Token rotation or incompatible agent | End viewers per existing policy; keep unanswered remote receipt; no compatibility bypass or automatic success |
| Lost executor storage/replaced host | Keep old run unknown until matching authenticated evidence or accepted administrator recovery; a new empty store is not negative proof |

Reconcile on hub startup, transport recovery and explicit run refresh. For each
outstanding run, allow one query in flight, a 5-second query deadline and at most
three attempts spaced 2 and 5 seconds in one reconciliation cycle. Thereafter
leave a visible unknown/blocked run and offer explicit refresh. Do not poll a
failed host indefinitely or let it block healthy-host reads. An active answer
can be refreshed while viewed; no age expires an unknown gate. A hub restart
settles its own abandoned local work; it cannot settle a remote agent worker. Queries may retry; dispatch is never
automatically replayed on startup or ambiguous transport failure.

Cancellation uses existing `activities:cancel` authorization at the hub and the
new agent cancel receipt path. It records intent before signaling, including a
request received before executor registration. Repeated requests are idempotent;
late cancellation of terminal work returns the terminal run. The executor owns
an application-lifetime context, detached from the HTTP caller, with the existing
Compose/Docker options and a 30-minute maximum workflow deadline. Docker stop's
existing 30-second grace stays unchanged. Deadline/cancel requests interrupt
local waiting but cannot reverse accepted Docker work. Once the worker and
known children have settled so they cannot issue more commands, commit failed
with reason `interrupted` for deadline/process interruption, or cancelled for
settled cancellation; effect state remains unverified. Release and re-read actual
Docker/Compose state before the next action. A daemon effect already accepted
may still complete; no rollback or exactly-once external effect is promised.
A live worker still unwinding retains its exclusion. Unanswered/unreachable
remote executors and lost storage/host continuity remain unknown, not settled
failure merely because the hub waiter or transport returned.

For a permanently unknown result, the accepted recovery is administrator-only
`POST .../runtime-runs/{runId}/release` (`release-runtime-run`) with explicit
risk acknowledgment and nonempty reason. Cancel/join any hub dispatch task and
close its transports first. Require the operator to stop the affected executor,
end its old handlers/connections, inspect actual Docker state and acknowledge
that no prior workflow can issue further commands; reopen only afterwards.
The hub cannot mechanically prove those operator-controlled facts. Persist the
hub interruption disposition and audit atomically before freeing the scope.
This is conscious administrator recovery, never a timer or automatic success.
Late executor evidence is recorded separately against that disposition without
reopening/overwriting history or replaying effects. Data loss/stale restore
requires this boundary when authoritative evidence cannot be recovered.

## HTTP and adapter compatibility

Keep the seven existing operation IDs/paths. Add an explicit
`Prefer: respond-async` mode for the HTTP adapter. Its JSON success envelope is
`{success:true,data:{activityId,code,activity}}`, HTTP 202 for `run.started`,
200 for joined/terminal retry. `activityId` equals `activity.id` equals the
hub/agent run ID. Admission confirms acceptance only. `runStackCommand` and
`runContainerCommand` still resolve `Promise<string>` to this ID; progress and
outcome arrive through targeted detail reads and existing activity SSE reloads.

HTTP 409 uses `{success:false,data:{code,activeRunId}}` for `run.busy` or
`run.cancelling`; mismatch uses `code:run.request-mismatch` with the bound ID
only when authorized. Translate internal `GateConflict.runId` explicitly.
Persistence/transport/mapping errors remain typed errors, not fake runs. Return
an admitted ID with unknown knowledge if dispatch becomes ambiguous; losing the
HTTP response is recoverable via the same idempotency key, without a new effect.
Add the mismatch code to the adapter conflict union with localized text; consumers
must not assume every conflict means another running command.

Without the preference, preserve project started/output/done/error NDJSON and
synchronous MessageResponse bodies, including `activityId`, for the shipped CLI
(`cli/pkg/projects/cmd.go`, `cli/pkg/containers/cmd.go`). Legacy waiters
subscribe to the admitted worker rather than executing it; disconnect ends only
that wait. Stream started frames expose the same ID. Error/conflict status must
be decided before headers flush. A loss after started is not a done frame. The
new adapter opts into admission JSON; CLI need not interpret a 202 as completed
success. The existing CLI `printOperationStreamInternal` returns scanner.Err()
at EOF and therefore accepts a clean EOF without a done frame. A narrowly owned
CLI compatibility correction must require done or return interruption, including
EOF immediately after started; keep its printed log and terminal-error formats.
Own that parser and its focused integration tests explicitly in the foundation,
before changing streamed execution behavior. Do not mask missing completion by
inventing a done frame. Cover both response modes before enabling capability.
Do not repurpose bulk `ContainerActionResult.success`, alter its array semantics
or enable bulk capabilities. No new Huma log route is needed.

`#102` is required before enabling M6 action UI: the current
`web/src/data/http/http-adapter.ts:loadActions` uses cross-host `Promise.all` and
rejects healthy-host runs if one source fails. A separate action-projection
foundation owner must supply healthy results plus typed per-host failure/last
observation, stable host/run deduplication, authenticated failure handling and
reconnect recovery. Its shared adapter result fields belong to that owner;
the runtime foundation consumes the merged contract rather than editing them
concurrently. This bounded prerequisite does not change other inventory readers.
Targeted hub runtime detail must work even while a remote host is unavailable;
all-host refresh failure must never hide an admitted run. Retained remote activity
reads are observations, not proof of effect completion. Scheduling this
prerequisite requires the coordinator's explicit scope/owner decision.

## Runtime logs

Reuse Echo `/api/environments/:id/ws/projects/:projectId/logs` and
`/containers/:containerId/logs`, permissions `projects:logs`/`containers:logs`
from `backend/api/ws/proxy_permissions.go`. Request JSON with `follow=true`,
`tail=100`, `timestamps=true`, `batched=false`; defaults/text/batched clients
retain their current formats. JSON decoding also accepts existing arrays and
preserves array order. Shared-source keys continue to include resource and all
options. The broadcaster and zero-subscriber cancellation remain shared.

Current `backend/pkg/libarcane/ws/bridge.go:LogMessage` has seq/level/message/
timestamp and optional service/containerId, with no source epoch or explicit
Docker stream. `backend/api/ws/log_streams.go` assigns seq per shared-source
instance; missing time uses receive time. Compose serializes service writers;
Docker non-TTY readers merge stdout/stderr, and non-follow drains streams in
implementation order. Thus seq is arrival serialization, not chronology across
services, stdout/stderr or hosts. Reopening a source can restart seq and repeat
the requested tail; it is not a resumable cursor.

Extend JSON frames additively with `sourceId` (random shared-source-instance ID),
`stream: stdout|stderr|combined|unknown`, `timestampOrigin: runtime|received`
and `truncated: boolean`.
Populate these before existing text normalization loses source information. Keep
service/containerId when known; use trusted Compose metadata for service identity,
not arbitrary log message text. TTY is combined; missing stream stays unknown.
Do not classify stderr as application error: wire `level` currently can encode
transport classification. Only an explicitly recognized semantic level maps to
`LogLine.level`; otherwise use info and retain stream separately. Do not guess
structured severities by parsing arbitrary application messages.

| `LogLine` target field | Source and rule |
| --- | --- |
| `id` | Tuple of subscription host/kind/raw resource, `sourceId`, `seq`; same source/seq deduplicates |
| `ts` | Valid wire timestamp; preserve timestamp origin; invalid required fields terminate as mapping error |
| `service` | Wire Compose service; otherwise known container display identity, or localized unknown source |
| `level`, `message` | Semantic level rule above, message text unchanged by adapter |
| Add `sourceId`, `seq`, `stream`, `timestampOrigin`, `truncated` | Shared adapter/domain fields, not feature imports of wire DTOs |

Within a source append by seq; never sort logs by ts. Gaps produce a visible
localized lost-lines marker, not interpolation or synthetic runtime entries.
Each reconnect is a new subscription attempt; a surviving shared source may keep
its epoch, a new one gets a new epoch. Display a reconnect boundary and replace
its tail buffer rather than claiming exactly-once history. No cross-epoch
content-based deduplication: identical text may be real repeated output. No
cross-source chronological guarantee or durable replay is offered.

The browser retains at most 2,000 lines and 1 MiB of serialized UTF-8 payload
per resource subscription, dropping oldest complete entries and displaying a
truncation marker; a single line is bounded to 64 KiB with a visible truncation
flag. Batch rendering uses at most one scheduled flush per animation frame.
Resource, adapter or identity generation change clears hooks and pending flushes
before accepting new lines. Pause closes/unsubscribes and keeps only that
resource's bounded visible buffer; resume requests a new tail with a boundary.
Unmount/last unsubscribe cancels timers and source references.

Keep current source queues (raw/mapped 256, broadcast 1,024, per-client 256;
`log_streams.go` and `backend/pkg/libarcane/ws/{hub,client}.go`). Bound line
payloads to 64 KiB at the producer before these queues, discarding
excess bytes until the next newline without building an unbounded string; carry
a truncation flag through normalization. Enforce the corresponding JSON frame
bound including metadata/escaping (up to 512 KiB per unbatched frame, and bounded
50-entry batches for legacy batch consumers). A full source broadcast queue
currently drops data;
change it to terminate that source visibly as a gap/error, and close slow
consumers rather than accumulate. Closing on error is sufficient even if the
error frame cannot be queued. Reconnect uses a bounded delay, pauses when offline
or unauthenticated, and never claims to fill a missing interval. Real WS pressure
tests must prove cancellation and memory limits, including batches and a huge
line. Runtime logs are ephemeral viewer data; the separate 256 KiB retained
`ActionRun.output` is execution debugging output and keeps its existing policy.

## Session-bound stream closure

In local mode the browser session ID is the revocation key. Add a shared
session-to-closers registry at authenticated SSE and WS acceptance, including
activity SSE, logs, stats, shells and direct/Edge proxied viewer connections.
Register then revalidate durable session state before sending data, closing the
race with revocation. Successful `SessionService` revocation invalidates auth
cache and closes matching handles idempotently. All revocation paths, including
logout, password recovery, expiry and revoke-all, share this mechanism. Register
an expiry timer for accepted sessions; startup mode changes/restore already end
connections with process shutdown. A failed durable revoke must return an error,
close locally known handles and not report successful global logout. Cookie
clearing alone is insufficient: `AuthHandler.Logout` currently logs revocation
failure and continues (`backend/internal/auth/handler.go`).

Revoking one session closes every tab using it; other sessions remain. SSE cancels
its send loop; local WS closes the client; remote WS closes both viewer proxy
legs and releases its source reference. Do not close the host's shared Edge
tunnel or cancel accepted operations when a viewer signs out. Browser sessions
and Edge session keys are distinct. Server cleanup is required even for a client
that ignores logout. No request credentials are forwarded to agents.

On logout success or authoritative session rejection, synchronously invalidate
the adapter identity generation, close EventSource and every WS/reconnect timer,
clear current runs, RunGate entries/listeners, metrics histories, live channel
values and log buffers, and set currentUser to null. Abort pending reads and
ignore callbacks from older generations even if abort is late. Broadcast a
same-origin logout signal to other tabs to clear their retained views too;
server closure and a fresh identity check remain authoritative if that signal
is missed. A successful login creates a fresh generation; an old close/error
callback cannot invalidate it. Identity rejection during a credential-check
form remains inline under the existing identity policy.

Proxy/none mode has no local browser session to revoke. Continue request/Origin
checks and existing effective administrator identity; do not invent local logout
or claim proxy sign-out. Proxy authentication lifetime remains the proxy's
responsibility. Cookie-less local explicit-token streams use their validated
session when present; API-key streams have no browser session and keep the
existing credential/host lifecycle. UI exposes logout only for local mode.

## Permissions, audit and version boundary

Apply the existing operation permissions before hub admission and before any
read/join/conflict detail is exposed; agent execution trusts only its manager
credential. Activity reads/cancel use existing `activities:read`/`activities:cancel`.
Manual release requires effective global administration in addition to activity
access. This adds no resource-right model or user role. Proxy/none remain the
persisted administrator as defined by [identity](identity.md).

Use the existing activity writer and event audit path. Record actor (including
request-scoped proxy attribution), host, target, operation, admitted run/key
reference, refused conflict/authorization, cancellation request, settled failure,
interruption/unknown reconciliation and manual release reason. Never record
credentials or raw request bodies. Refusal creates no execution activity; use
a bounded structured audit event, with no unauthorized active-run details.
Conditional terminal writes produce one final message; repeated joins/queries
must not create duplicate success events. Audit failure during admission refuses
the action; failure after effects retains an unresolved receipt and visible error.

Current `types/contract/agent.go` advertises version/minimum **3**; its fingerprint
hashes the bytes of `types/contract/testdata/agent-hub-v3.json`, not OpenAPI.
The target is agent contract **4**, minimum **4**: durable execute/query/cancel
semantics, supplied run identity, new receipt/snapshot fields, admission response
mode and JSON log metadata are required together. These are additive routes/fields
but a breaking behavioral compatibility boundary: a v3 agent cannot promise
idempotent execution or outcome reconciliation. Preserve the v3 fixture and add
v4 bytes/fingerprint; do not calculate the fingerprint from generated OpenAPI.
`tunnel/v1` transport remains unchanged. Shared product SemVer is advanced by
release packaging, not by treating the agent contract integer as SemVer.

Advertise/enforce v4 before forwarding ordinary operations; old or mismatched
agents stay update-required with existing health/version/self-upgrade exceptions.
Recovery queries are not an excuse to bypass compatibility. A rolling upgrade
can temporarily disable commands; no fallback to unguarded v3 execution. Test
synthetic v3 refusal on both transports; acceptance with a real released old
agent remains the separate first-release boundary.

## Shared foundation and verification boundaries

One runtime foundation owner changes the following together and merges before
consumers: `types/{activity,project,container,contract}` wire vocabulary and v4
fixture/hash; runtime route registrations, agent allowlist/permission snapshots;
current OpenAPI, generated TypeScript and spec provenance hash; capability/adapter
port/domain shared fields; receipt models, schema registry/integrity proofs and
a new forward migration/manifest entry. Include JSON log metadata declarations, runtime output cursors
and runtime codes in this owner. Include the focused CLI NDJSON EOF correction
and compatibility test so legacy consumers cannot report unknown as success.
Keep capabilities disabled until their complete
executor, adapter and UI paths pass. Foundation acceptance must compile every
consumer with unavailable mappings; fixtures alone do not implement the protocol.

Preserve `api/openapi.before-cut.json`, shipped SQLite `001`/`002` and their
hashes. Update current route snapshots/functional-cut reviewed additions rather
than historical baselines. Split adopted oversized
`backend/internal/project/handler.go` and `types/container/container.go` on
substantive edits; do not raise ceilings or give helper files inherited exemptions.
Read-only neighbors remain inventory/observations, identity/CSRF policy,
backup/restore, scheduler/coordinator receipts, unrelated resource mutations,
Docker/Compose semantics and deployment packaging. Any needed change there is
an explicit prerequisite, not an incidental runtime patch.

Each acceptance row needs executable regression evidence. At least three
independent targeted mutations per runtime package must fail its tests; restore
mutations before checks. Process tests use separate real lifetimes/temp SQLite;
transport tests traverse real direct HTTP and Edge forwarding rather than mocking
the final executor call. CI needs no real Docker or agent; practical acceptance
supplies actual effects separately.

| Case and method | Required observation | Mutation/counterexample |
| --- | --- | --- |
| Competing exact/parent/sibling admission; SQLite service test | One worker for same command/key; parent excludes child both ways; siblings independent; hosts independent | Remove parent predicate or split read/insert transaction |
| Duplicate key after terminal/history pruning; receipt test | Same ID/result, no second Docker effect; changed digest 409 | Delete receipt with activity or ignore digest |
| Both legacy and async seven-route callers; HTTP/CLI tests | Correct run ID, preserved NDJSON/messages, coded 409 before flush | Gate only local handler or treat 202 as completion |
| Persistence/audit refusal; injected storage fault | Zero dispatch/effect, no fabricated ID | Fall back to StartHandlerActivity |
| Cancellation before registration, during call, twice and after terminal | Persist intent, no early release; cancellation distinct from rollback | Release on request cancellation |
| Deadline with indeterminate Docker effect; controlled executor | Unknown held until proof/recovery | Map timeout directly to terminal failed/release |
| Direct and Edge response lost before/after acceptance; real transports | Same run ID queried; no retransmission or duplicate effect | Replay POST on reconnect |
| Hub crash before send, after dispatch mark, after terminal commit; child processes | Proven unsent may release; ambiguous held; one final message | Let generic startup repair fail runtime receipts |
| Agent crash/reopen with executing receipt; child processes | Unknown/interrupted survives, no effect replay or age release | Treat missing worker as settled failure |
| Outcome active/terminal/not-found, wrong digest/token and unreachable | Release only matching terminal proof; honest unknown | Treat 404/transport error as terminal success |
| Completion DB busy/full; temporary file fault injection | Bounded retries, scope retained after failure | Swallow terminal persistence error |
| Manual release and late receipt, if accepted | Admin acknowledgment audited; original disposition retained | Allow ordinary user or overwrite released history |
| #102 two-host reads, direct/Edge rejection and reconnect | Healthy runs retained, source failure explicit, targeted run visible | Restore cross-host Promise.all rejection |
| Runtime JSON/text/batch, TTY and two Compose services; real WS | Service and stream exact, stable epoch/seq, arrival order only | Sort by timestamp or label stderr as error |
| Reconnect/new epoch and resource switch/pause/unmount | No ID collisions/stale hook rows; tail boundary explicit; last ref cancels | Reuse seq as global ID or retain old hook state |
| Slow reader/full source/huge line; real WS and bounded heap probe | Caps, visible loss/truncation, source/clients close | Unbounded queue or silent broadcast loss |
| Local logout/revoke/expiry during accept; SSE/local/direct/Edge WS | All matching session viewers close, another session and worker survive | Skip registry recheck or close whole Edge tunnel |
| Two tabs/logout/relogin with delayed callbacks; browser tests | Buffers/timers cleared, old identity cannot overwrite new | Omit generation check or leave RunGate listener |
| Admin/denied-user, local/proxy/none; permission tests | Existing grants and Origin/CSRF policy preserved; denied has no effects | Authorize only after dispatch |
| v3 and fingerprint mismatch, direct/Edge; contract tests | Update-required, no ordinary forwarding, recovery exceptions intact | Ignore minimum/fingerprint |

Practical acceptance uses the exact reviewed/merged package in an explicitly
approved sandbox: real stack start/down/restart/redeploy, container start/stop/
restart on direct and Edge hosts, conflict and duplicate IDs, interrupted hub/
agent reconciliation, unknown recovery, log sources/order/pressure and session
closure. Exercise EN/DE at desktop and 390 px, permissions, localized reason/
unknown/truncation text, run links after one host fails, and verify the served
web bundle/package SHA. Record effect observations, audit/snapshot agreement,
cleanup and honest transport/restart limits. No local test, synthetic older agent
or demo run substitutes for that exact-package acceptance.
