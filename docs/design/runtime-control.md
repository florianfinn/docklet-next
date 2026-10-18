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

A client creates an opaque UUID `Idempotency-Key` per deliberate command; any
UUID version is accepted without reading a client timestamp. The hub measures
the ten-minute retry window from its first-seen binding. The adapter retains the
key across CSRF/transport retries for at most ten minutes of local elapsed time,
discards it after the final response or that window, and then refreshes known
run detail rather than replaying the command. It never creates a replacement
intent automatically. A retained binding always resolves to its original run,
even after the retry window; it cannot admit another effect. These are target
key rules, not current adapter support.

The binding uniquely maps `(environmentId, idempotencyKey)` to run ID, requesting
caller and canonical digest: operation ID, raw target, accepted options and
parent. Within the window, same key/digest returns the same ID and compact
outcome even after output/history pruning; recheck caller identity and current
operation/activity-read permissions. A different caller presenting an already
bound key receives HTTP 409 `run.request-mismatch` without the bound ID or other
run details, even when the digest matches. A changed digest also returns 409
without a worker. Independently authorized callers with different keys join by
active gate key, not by sharing an idempotency key. The activity/receipt keeps
its original actor; each joining key records its own caller and join audit attribution.
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
activity while executor knowledge stays unknown; ordinary waiters cannot. Agents
enforce the same scope predicates on trusted execution envelopes. Their legacy
lifecycle listeners must refuse requests that bypass hub admission; a manager
credential alone cannot create an unguarded worker. This is no distributed lease.

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
dispatch commit time/state, executor state, cancel intent, result/error and last
reconciliation time. Key bindings contain caller, digest and hub first-seen time.
Storage failure refuses admission before effects.

Keep full activity/output retention at **100 finished activities per environment**;
clearing history may remove terminal output immediately. Separate compact
receipts/key bindings/refusal tombstones survive either operation without a
cascading activity FK. Unresolved active and unknown receipts never expire or
clear with history. Explicit administrator disposition makes the hub receipt
terminal for retention while preserving unknown external effect knowledge.
Terminal receipts, joining bindings and not-received tombstones have a
**24-hour minimum retention** from their final write (including a later binding),
which exceeds all supported retry and delivery budgets. A bounded cleanup batch
runs at startup and hourly, deleting eligible compact records deterministically
by final-write time then ID. This bounds terminal evidence by
recent traffic, not by installation age; active/unknown evidence remains until
settled or explicitly disposed. No full output is retained for this grace.

An authenticated one-shot dispatch binds manager, registered target, executor
storage generation, run ID and digest. It carries no wall-clock issue/expiry
fields. The supported delivery budget is at most **30 minutes** on direct and
Edge paths, without automatic retransmission or a persistent dispatch queue.
The generic direct proxy waits up to 30 minutes
(`backend/internal/middleware/environment.go:proxyTimeout`); the remenv fallback
client uses 30 seconds (`backend/pkg/remenv/client.go:NewClient`). Edge waits
five minutes by default (`backend/pkg/libarcane/edge/proxy.go:DefaultProxyTimeout`)
and its pending responses are tunnel-local memory; `CommandClient.Execute`
sends once and removes its pending entry when waiting ends. The CLI's default
wait is ten minutes (`cli/internal/client/client.go:defaultTimeout`). Thus the
30-minute delivery budget plus ten-minute retry budget is well below the
24-hour compact floor. These are bounded supported deliveries, not a claim that
sender timeout prevents an already-running receiver from acting.

The foundation must enforce the short **pre-acceptance** boundary as well:
create a receiver-local monotonic admission deadline at ingress, before Edge
worker scheduling/body assembly or direct handler waits. Cap it at five minutes
and use the request/session cancellation context through receipt admission.
Check cancellation/deadline after waits and inside the transaction before commit.
Before worker registration, resolve whether durable acceptance won under the
admission mutex: failed admission starts no effect; a committed receipt hands
off to application lifetime and wins subsequent duplicate/query races. Never
abandon a committed receipt merely because the caller disconnected. A cancellation
racing with commit may leave accepted work, which is queried rather than assumed
absent.
Even counting a separate five-minute receiver budget conservatively, delivery
plus admission plus retry is far below 24 hours. No handler may wait indefinitely
and then detach a new admission from a cancelled context.

These checks are target behavior, not existing transport guarantees. Edge's
`client.go:messageLoop` currently ignores cancel-request messages; its local
request timeout starts inside the scheduled handler, and `ServeHTTP` does not
itself stop a handler on context expiry. Removing a hub pending entry therefore
proves nothing about receiver admission. Tests must stall the actual receiver
before its receipt transaction and expire its ingress context on both transports.
Arbitrary manual replay, externally suspended connections/processes delivering
beyond the supported budgets, and replay after retention are outside this bounded
contract; do not rebuild perpetual recognition to cover them.

After compact evidence is pruned an old opaque UUID is no longer recognizable;
absence is not historical success or proof of no effect. Supported clients stop
retrying within their budget and never reuse an old UUID for a new intent.
Within the compact window duplicate execute returns the original receipt or
refusal tombstone with zero additional effects. Query/tombstone races are atomic.
Outside that evidence window, absence stays unknown and uses the recovery rule
below; callers refresh actual state before a new deliberate intent.
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
| `admitted` → `dispatching` or terminal `cancelled` | Hub transaction rechecks cancel intent before any remote send; intent settles cancelled with no send, otherwise commits dispatching | Held for dispatch; released after cancellation commit |
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
with the authenticated identity binding above and attribution, plus
`GET /api/environments/0/runtime-runs/{runId}` (`get-runtime-run-outcome`) and
`POST .../{runId}/cancel` (`cancel-runtime-run`). Query/cancel have hub-facing
`{id}` equivalents served locally from the hub receipt, with no generic proxy
bypass. The hub's worker uses existing direct/Edge request transport to the agent
route; token stripping/injection, compatible-host checks and Edge-only no-direct
fallback stay unchanged. An execute acknowledgment is bounded JSON, not an open
Docker execution stream. A duplicate execution envelope returns the persisted
receipt within its retention window, never invokes the executor again. No
post-pruning recognition or arbitrary old-envelope refusal is promised.
Execution accepts only the seven canonical operations and bounded validated
options, never arbitrary routes or commands. An authenticated agent admission
refusal proves this dispatch started no worker; the hub records a failed/refused
run with the agent conflict reason and releases its own admission. A transport
error while receiving that refusal is uncertain and keeps the scope blocked.

Agent outcome query is authoritative only for the matching run/digest,
registered executor and continuous storage generation. Valid answers use HTTP
200: `active`, `terminal`, or `not-received` backed by a refusal tombstone.
Query of a never-received ID atomically inserts that tombstone before answering;
a racing/delayed execute then refuses. The hub may fail as not dispatched and
release only when that negative proof also satisfies its own recent-dispatch
rule: elapsed age from its dispatching commit is nonnegative and less than
**one hour**, in the same hub process lifetime, with no restore or clock/continuity
uncertainty. Use its local monotonic elapsed time, not agent/browser timestamps.
The persisted hub dispatch time alone cannot re-establish that proof after a
restart or restore. Otherwise treat absence/tombstone as `unknown` with reason
`evidence-expired`, including a query after the agent pruned a terminal receipt;
never relabel an old effect as not dispatched. A matching retained active or
terminal receipt remains usable without this negative-proof age test.
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
before trusting negative evidence. Ordinary browser/agent clock skew, including
plus or minus one hour, refuses no command. Local retention uses each store's
clock; an external forward jump large enough to prune a live-window tombstone
can invalidate the retention premise, just as stale database rollback can.
Backward jumps can delay cleanup. Neither direction persists a refusal cutoff
or blocks new intents until a wall clock catches up. These external interference
limits need no clock service or cross-host clock comparison. No generation
change proves an old executor stopped. The foundation owns this evidence
contract; executor readiness enforces it. This adds no lease, second hub or workflow replay.

An active/terminal answer carries the existing activity snapshot/detail vocabulary
(progress, affected services, coded steps/messages and capped execution output),
plus receipt knowledge fields. Known outcome means a settled workflow disposition,
not verified Docker state after interruption. Runtime retained output lines also carry a
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
| Hub startup finds an `admitted` receipt | Settle failed/interrupted, effect unverified, and release; a restored backup row cannot prove no later send occurred |
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
request received before executor registration. For a remote run still admitted,
the hub's dispatch transaction rechecks that intent and commits cancelled with
no send; this hub writer alone settles pre-dispatch cancellation. Once dispatching
is committed, cancellation follows the remote outcome path. Repeated requests
are idempotent; late cancellation of terminal work returns the terminal run. The executor owns
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
`run.cancelling`; mismatch uses `code:run.request-mismatch`. Reuse by a different
caller never includes the bound ID or run details; same-caller digest mismatch
may include the ID only with current activity-read permission. Translate internal
`GateConflict.runId` explicitly.
Persistence/transport/mapping errors remain typed errors, not fake runs. Return
an admitted ID with unknown knowledge if dispatch becomes ambiguous; losing the
HTTP response is recoverable via the same idempotency key, without a new effect.
Add the mismatch code to the adapter conflict union with localized text; consumers
must not assume every conflict means another running command.

Without the preference, preserve project started/output/done/error NDJSON and
successful synchronous MessageResponse bodies with `activityId`. Legacy waiters
subscribe to the admitted worker, never execute it. Started exposes the same ID;
pre-admission errors/conflicts are decided before flush. Once NDJSON is flushed,
non-success emits an error frame carrying `activityId` and the code below,
never done/success; a dead socket can receive nothing. Disconnect only detaches
the waiter; durable admission identity and execution remain intact.

For synchronous project down/restart and single-container callers, only confirmed
success returns HTTP 200 with the existing success MessageResponse and ID.
Every writable non-success response uses
`{success:false,error:{code,message},data:{activityId}}` after admission:

| Result at waiter boundary | HTTP status | Error code |
| --- | --- | --- |
| Executor outcome unanswered/lost continuity | 503 | `runtime.outcome-unknown` |
| Settled process/deadline interruption, effect unverified | 409 | `runtime.interrupted` |
| Settled cancellation | 409 | `runtime.cancelled` |
| Other settled executor failure | 500 | `runtime.failed` |
| Wait budget expires while execution remains active | 504 | `runtime.wait-timeout` |
| Waiter cancelled but response still writable | 408 | `runtime.waiter-disconnected` |

The waiter is bounded to at most thirty minutes and can expire before the worker
settles; this changes no receipt state and releases no scope. If the socket is
already gone, write no HTTP response or invented status: retain the ID in the
receipt and recover it via a valid key retry/targeted read. Pre-admission failures
have no admitted ID. Async admission may still return the admitted ID with
unknown knowledge; its 202 never claims completed success.

`cli/internal/cmdutil/run.go:RunPostAction` prints its configured success text
when `PostJSON` succeeds, so non-success must be non-2xx. Preserve its normal
progress/error handling and test all message callers. The existing project
`printOperationStreamInternal` returns `scanner.Err()` at clean EOF without
requiring done (`cli/pkg/projects/cmd.go`). The executor/legacy compatibility
step owns the narrow fix and tests: EOF before done returns interruption,
including immediately after started; preserve printed progress and terminal
errors. Never fabricate done to mask missing completion. Both response modes
pass before enabling capabilities; no CLI rewrite or CLI edit in the shared
types/schema subpackage.
Do not repurpose bulk `ContainerActionResult.success` or enable bulk controls.

`#102` is required before enabling M6 action UI: the current
`web/src/data/http/http-adapter.ts:loadActions` uses cross-host `Promise.all` and
rejects healthy-host runs if one source fails. A separate action-projection
prerequisite owner supplies healthy-host results plus minimal honest failure
projection and reconnect recovery, preferring existing error/observation
channels. Preserve stable host/run identity without creating a new framework.
Its shared adapter result fields belong to that owner;
the runtime foundation consumes the merged contract rather than editing them
concurrently. This bounded prerequisite does not change other inventory readers.
Targeted hub runtime detail must work even while a remote host is unavailable;
all-host refresh failure must never hide an admitted run. Retained remote activity
reads are observations, not proof of effect completion. This prerequisite is
separately accepted and coordinator-scheduled, before the runtime foundation
freezes shared projection fields and before action UI.

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

Use `(subscription attempt, seq)` for line identity, scoped by host/kind/raw
resource. Increment the client attempt on every connect/reconnect, including
reopening a surviving shared source; no wire sourceId or timestamp-origin field
is required. Seq orders source arrival, never timestamp chronology. A gap emits
a localized loss marker. Reconnect displays a boundary and replaces the bounded
tail; repeated text is not content-deduplicated or claimed as exactly-once replay.

| `LogLine` target field | Source and rule |
| --- | --- |
| `id` | Resource identity plus subscription attempt and wire seq |
| `ts` | Valid wire timestamp; timestamps requested from runtime, current normalizer may fall back to receive time |
| `service` | Trusted Compose metadata/prefix; otherwise known container identity or localized unknown source |
| `level`, `message` | Only recognized semantic level maps to severity; otherwise info; preserve message text |
| Optional `seq`, `stream`, `truncated` | Log-owner adapter/domain additions; missing metadata uses unknown/false fallback |

Stderr describes a transport stream, not application error severity. TTY is
combined, otherwise stream is stdout/stderr only when available before text
normalization loses it; omission maps to unknown. The log step may add optional
`stream`/`truncated` JSON metadata and matching domain fields. Older agents omit
these fields; that alone needs no mandatory v4 bump. Never infer structured
severity from arbitrary message text. Invalid mandatory fields are mapping errors.

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
payloads to 64 KiB at the producer before these queues, discarding excess
through the newline without building an unbounded string. The log owner edits
adopted `third_party/arcane/docker/logs.go:ReadLogLines` (currently unbounded
ReadString), preserves its notice and records the bounded-reader deviation in
provenance. Cover follow and non-follow buffering as well as Compose writers;
no entire output may accumulate before the bounded reader. Optional truncation
metadata or a visible producer marker exposes the cut. Existing 50-entry batches
stay bounded by the line bound; no independent 512 KiB frame rule is needed.

Keep shared `wshub.Hub.Broadcast` drop behavior and existing slow-client removal;
the hub also serves stats. Seq is assigned before broadcast, so clients detect
subsequent gaps without terminating every source. A final drop without a later
seq is not provably detectable; do not promise lossless delivery. Reconnect is
bounded/delayed and pauses offline or unauthenticated; it cannot fill a lost
interval. Real WS pressure tests prove queue/producer/browser bounds and last-ref
cleanup. Runtime logs remain ephemeral and separate from the existing 256 KiB
retained execution-output cap and bounded remote CLI relay.

## Session-bound stream closure

In local mode the browser session ID is the revocation key. Add a shared
session-to-closers registry at authenticated SSE and WS acceptance, including
activity SSE, logs, stats, shells and direct/Edge proxied viewer connections.
Register then revalidate durable session state before sending data, closing the
accept/revoke race. Explicit logout/revoke uses a shared closer registry to close
matching handles idempotently after durable revocation. Revalidate durable
session/user existence, revoked state and expiry at the existing SSE heartbeat
(15 seconds) and browser-facing WS ping, bypassing token caches. A failed
validation or unreadable durable session closes the viewer. There is no
per-session expiry timer or new logout error policy.

Actual cadence owners are `backend/api/handlers/stream.go`, local
`backend/pkg/libarcane/ws/client.go` (54-second ping), WS handler keepalive for
system stats/exec, diagnostics' own 54-second ping, and
`backend/pkg/libarcane/edge/ws_proxy.go` (54-second browser-facing ping).
The direct bridge `backend/pkg/libarcane/ws/proxy.go` only relays frames: it has
no ping loop. The session step explicitly adds a cancellable 54-second durable
session recheck there, sharing the session helper and closer lifetime; no remote
agent ping can validate a hub browser session. Close both viewer legs on rejection.
This is one bounded connection check where no existing cadence exists, not an
expiry scheduler. Tests exercise idle direct/Edge viewers as well as active ones.

Do not assume all SQL mutations use one SessionService instance. Password change
and administrator recovery call user `SetPasswordAndRevokeSessionsExcept`, wired
to `session.RevokeAllUserSessionsExceptInDB` in `di.go`; restore constructs its
own SessionService in `system/children/backup/restore.go`. Session/user deletion,
expiry cleanup and startup mode revocation must also fail durable revalidation.
The shared registry handles explicit service revocation; cadence validation
catches bypassing transactions/instances without changing their identity policy.
Restore/mode transitions retain their existing maintenance/process boundary.
`AuthHandler.Logout` currently logs durable-revoke failure and continues; this
step does not change that handler behavior or claim durable global logout when
revocation failed. Keep the adopted oversized auth handler read-only: use focused
session/transport helpers, not a substantive handler edit or ceiling increase.

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
ignore callbacks from older generations even if abort is late. Other tabs close
through server revocation checks and existing authoritative identity rejection;
no cross-tab broadcast is required. A stream close alone is not proof of logout: a
subsequent identity check distinguishes transport loss from revoked identity.
A successful login creates a fresh generation; an old close/error
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
semantics, supplied run identity, bounded retry/receipt retention, storage-generation
evidence, receipt/snapshot fields and admission response mode are required together.
Optional log metadata remains owned by the log step. These are additive routes/fields
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
a new forward migration/manifest entry. Include runtime output cursors/codes,
storage-generation and bounded-retention vocabulary in this owner. Optional log metadata
belongs only to the log step; the CLI EOF behavior fix belongs to executor
compatibility. Foundation work can use bounded sequential subpackages under this
same owner, but is reviewed/integrated as one complete foundation before any
consumer starts. There is no second consumer-stage editor of runtime contracts.

Contract version/hash and route exports cannot advertise executable v4 semantics
before behavior exists. The complete foundation therefore also owns the minimal
admission/execution/query/cancel/release behavior and legacy compatibility needed
to enforce that contract, reviewed together before merge. Its focused behavior
subpackages precede final v4 activation/exports; no partial foundation merges.
Capability descriptors and adapter mappings remain honestly unavailable until
complete UI consumers pass. Later execution consumers may add hardening/tests
within the frozen contract, never deferred routes/exports/version edits. Fixtures
alone do not implement the protocol.

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
| Opaque-key retry/history clear and compact pruning; receipt test | Hub-first-seen 10min retry budget; retained key returns original ID/outcome; cross-caller reuse gives non-disclosing 409; new-key authorized join preserves original actor; duplicate within 24h has zero additional effects; active/unknown protected | Delete compact evidence with history, leak bound ID to another caller or start another worker |
| Both legacy and async seven-route callers; HTTP/CLI tests | Correct run ID, preserved NDJSON/messages, coded 409 before flush | Gate only local handler or treat 202 as completion |
| Persistence/audit refusal; injected storage fault | Zero dispatch/effect, no fabricated ID | Fall back to StartHandlerActivity |
| Cancellation before dispatch/registration, during call, twice and after terminal | Hub dispatch transaction settles admitted cancellation with zero sends; later intent holds until worker settles; cancellation distinct from rollback | Skip dispatch intent recheck or release on request cancellation |
| Deadline/cancel while worker unwinds then settles; controlled executor | Held until worker/children settle; then failed/interrupted or cancelled, effect unverified, re-read | Release at timer before worker settles |
| Direct and Edge response lost before/after acceptance; real transports | Same run ID queried; no retransmission or duplicate effect | Replay POST on reconnect |
| Hub crash before send, after dispatch mark, after terminal commit, restore of admitted row; child processes/store fixture | Startup settlement says interrupted/effect unverified; remote ambiguity held; one final message; restored row never claims not dispatched | Label restored admission not dispatched or let generic repair release unanswered remote work |
| Agent crash/reopen with continuous store; child processes | Failed/interrupted before readiness, effect unverified; no replay | Block ordinary own-process restart indefinitely |
| Outcome/tombstone race, delayed receiver, lost store/host, old query and clocks; real transport/store tests | Same-generation recent negative proof fences execute; ingress expiry prevents delayed admission; accepted work survives caller timeout; pruned/old query stays unknown; browser/agent ±1h refuses nothing; hub/agent jumps never latch refusal | Ignore ingress cancellation, trust old/uncertain negative proof or empty replacement store, or restore timestamp-based refusal |
| Completion DB busy/full; temporary file fault injection | Bounded retries, scope retained after failure | Swallow terminal persistence error |
| Accepted administrator release and late receipt | Admin acknowledgment audited; original disposition retained | Allow ordinary user or overwrite released history |
| #102 two-host reads, direct/Edge rejection and reconnect | Healthy runs retained, source failure explicit, targeted run visible | Restore cross-host Promise.all rejection |
| Runtime JSON/text/batch, TTY and two Compose services; real WS | Service and stream exact, attempt/seq identity, arrival order only | Sort by timestamp or label stderr as error |
| Reconnect/new attempt and resource switch/pause/unmount | No ID collisions/stale hook rows; tail boundary explicit; last ref cancels | Reuse seq as global ID or retain old hook state |
| Slow reader/full source/huge line; real WS and bounded heap probe | Caps, visible loss/truncation, existing slow-client removal and subsequent seq gap | Unbounded producer read or hide seq gaps |
| Local logout/revoke/expiry, password/recovery/restore bypasses; SSE/local/direct/Edge WS | Immediate explicit close or next bounded durable check, another session/worker survive | Use auth cache, omit direct-proxy cadence or close whole Edge tunnel |
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
