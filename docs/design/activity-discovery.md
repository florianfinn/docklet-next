# Activity discovery and bounded recovery

Activity discovery uses a list projection and bounded reads for #225 and #223. It keeps
the durable run and admission contracts from [runtime control](runtime-control.md)
and the observation rules from [the HTTP adapter](adapter-live-capabilities.md).

## Producer, transport and consumer map

`ActivityService.ListActivitiesPaginated` in
`backend/internal/activity/service_projection.go` maps stored activity rows into
`types/activity.Activity` and applies hub runtime receipt knowledge. The activity
handler adds source labels. Remote lists merge hub-owned summaries and agent-owned
activities, suppress duplicate runtime identities, restore the environment ID and
report partial transport failure through `X-Docklet-Host-Reason`. Both Direct and
Edge use this path. An incomplete response is useful evidence, never a complete
empty history.

`ActivityService.GetActivityDetail` in `backend/internal/activity/detail.go` adds
messages, a maximum of 256 step transitions, retained output and its truncation
flag. Runtime-owned detail goes through `runtime_activity_handler.go` first; its
receipt/recovery evidence must remain authoritative over an agent observation.
Generic detail and the specialized runtime detail route are different contracts.

OpenAPI operations `list-activities` and `get-activity` expose these DTOs through
`api/openapi.json` and generated `web/src/api/generated/schema.ts`. In the web,
`http-adapter.ts` fetches every list page and supplies the reader. Currently
`activity-reader.ts` calls `mapActivityDetail` once per listed row, then calls
`runtime.project()`. The latter can fetch specialized runtime detail again. The
four-worker limit bounds concurrency, but neither source of total request growth.

The actual consumers receive `CodedActionRun` through `listActions` and
`subscribeActions`; `data/live.ts` shares the subscription and renders codes into
`ActionRun` in the selected locale. `AdapterProvider` separately invalidates event
queries. The notification observer itself is a real activity consumer even when
the Activity Center is closed. An event-topic observer is not an activity consumer.

| Consumer | Required fields | Summary source | Detail-only requirement |
| --- | --- | --- | --- |
| Activity Center collapsed rows | ID, host/resource identity, kind/title/target, status, progress, services, start/end, error, runtime state labels | Base `Activity` fields plus receipt `disposition` and `unknownReason` in the list-only projection | No output/history or recovery grants needed |
| `ActionToasts`, `RunToastCoordinator`, `ActionToastView` | Above fields, terminal transitions, phase label, ordered steps and counts; source completeness for initial-history suppression | Base fields, list `stepTrace`, receipt recovery presentation and source-observation metadata | Raw output and complete message history are unused; a toast is summary demand |
| Expanded `ActivityCenter.RunBody`, `ActionDialog`, `RunFocus`, `RunOutput` | Base fields, complete steps, retained output, truncation, failure diagnosis; runtime rights and recovery controls when applicable | Base fields and runtime outcome/cancel intent | `stepTrace`, `messages`, `output`, `outputTruncated`; specialized runtime evidence and grants |
| `run-gate` and runtime command follow | ID, resource/host, status, kind and timestamps; explicit runtime follow needs authoritative recovery evidence | Base fields and runtime knowledge | Specialized runtime detail for followed runtime controls; discovery must not imply that demand |
| Stack `DeploymentsTab` and `deployRuns` (#198 consumer) | Host-scoped `stackId`, deploy/redeploy kind, status and timestamps for history; selected run uses `RunFocusView` and output | Identity/filter/list fields | Only the shown run needs full detail; older collapsed history does not |
| `SecurityPage` | Running scan identity/status and running step label | Base fields and list `stepTrace` | None; an older agent without the projection retains the existing generic scanning label |
| Onboarding `ConnectSteps` | Run identity/status, step labels and states | Base fields and list `stepTrace` | None; an older agent without the projection has no detailed onboarding steps |
| Dashboard `attention` | Latest failed run per stack, title, host, timestamps and link IDs | Base fields; restore-point evidence remains absent when unsupported | None |
| Shared update-time panels (`update-times.ts`) | Successful update kind/status, `stackId`, `finishedAt` | Base fields | None |
| `use-host-commands` | Host ID, action kind, running status for command availability | Base fields | None |
| `AppShell` and `use-nav-counts` | Queued/running status counts | Base fields | None |

`mapActivityDetail` also maps all messages into `log`, falling back to the latest
coded message. Full `log` is not needed for list/notification decisions. `services`
comes from `affectedServices`, not detail. `restorePointId` has no source in the
current generic activity mapper; this foundation must not invent backup evidence.
The stack-history membership decision is separate from this discovery contract.

## List-only projection

`ListActivitiesOutput` and its remote list decoder use a new `ActivitySummary`
item, embedding the existing `Activity` and adding optional list-only fields:

```go
StepTrace []StepTraceEntry        `json:"stepTrace,omitzero"`
Recovery  *ActivitySummaryRecovery `json:"recovery,omitzero"`
```

`StepTrace` uses the existing entry shape. A nil slice is absent (producer has no
projection); a non-nil empty slice serializes as `[]` (known empty), including
after an agent response is decoded, merged and re-encoded by the hub. `omitempty`
is unsuitable because it erases that distinction. The wire has no output or
message history in a list item. The new OpenAPI summary component is referenced
only by `list-activities`; generated DTOs follow it in the same contract change.

The summary projection runs on the HTTP list-handler path, after authorization
and runtime ownership replacement. `ListActivitiesPaginated` remains the base
activity source for stream snapshots and poller fingerprints. Those producers do
not acquire traces or recovery fields. The HTTP list requests the optional
`includeSummary=true` query flag, forwarding it to the agent only for UI listing.
Without that opt-in the agent returns base rows without trace/recovery queries,
including remote stream-poller requests. Older agents ignore the unknown flag and
omit the projection. Remote HTTP listing decodes agent summaries
and merges them with projected hub-owned rows; its base stream-poller path keeps
decoding base activities. Preserve source labels, filtering, duplicate ownership,
resource IDs and `X-Docklet-Host-Reason` through both paths.

The shared `types/activity.Activity`, `Detail`, `StreamEvent`, `RuntimeSnapshot`
and strict runtime transport decoders do not change. In particular, no nested
`Detail.activity.stepTrace` appears. The selected `agent-hub-v6.json` contract and
fingerprint remain unchanged: these optional list fields do not become required
agent values. A same-version older agent yields step-less remote rows until
upgraded; there is no eager detail fallback or silent promotion to complete steps.
New agent to old hub safely loses these optional fields on tolerant list decoding.

### Trace and recovery sources

Read authorized IDs in chunks of **100** in both the existing receipt projection
and every new trace/receipt query. Full-window lists can exceed SQLite parameter
limits because retained history can be configured unlimited and active runs are
not capped. Chunking must not change ordering or per-activity limits.

| Owner | Trace source and parity reference |
| --- | --- |
| Ordinary local or agent-owned activity | Message transitions used by `GetActivityDetail`: nonempty steps, consecutive-step collapse, newest 256 transitions, reverse to chronological order, then `stepTraceInternal` terminal status/timestamps |
| Hub runtime receipt with environment `0` | Local message-transition branch of `runtimeDetailSnapshotTxInternal`: exclude step values over 4,096 bytes before finding transitions; newest 256, chronological projection, then trim oldest entries until encoded trace is at most 64 KiB |
| Hub runtime receipt for a remote environment | Retained executor window in `activities.runtime_step_trace`, not a new generic message query or an agent trace; apply the same 256-entry / 64 KiB bounds as runtime detail |

For batched message queries, calculate `LAG(step)` within each activity first,
filter actual transitions, then keep `ROW_NUMBER() OVER (PARTITION BY activity_id
ORDER BY created_at DESC, id DESC) <= 256`. Ranking raw messages before collapsing
transitions loses older distinct steps. Receipt-owned parity is measured against
`getRuntimeActivityInternal`, which serves the generic detail route through runtime
detail; ordinary parity uses `GetActivityDetail`. Share trace/bounds helpers with
those producers without reading retained output per summary. Runtime detail's
additional whole-body cap can trim a trace further when messages/output consume
its body budget; summary trace parity compares the common bounded trace before
that detail-only body-pressure trim and tests the exception explicitly.

For receipt-owned rows only, `ActivitySummaryRecovery` carries `disposition` and
`unknownReason` directly from the same authorized receipt row used for status.
The summary mapper sets `runtime.released` from `disposition == "released"` and
preserves `unknownReason`; `runtime.resultCode` comes from the base `messageCode`.
Thus a cold list renders released, unknown, unconfirmed, interrupted and cancelling
labels exactly as before, without retained browser evidence or detail reads.
`ActivitySummary` is also the extension point for later list-only fields; those
fields follow the same optional `omitzero` and older-agent compatibility rules.
Recovery permissions, disposition rationale, executor late-result evidence and
release controls remain detail-only. The projection never manufactures grants.

## Summary demand, detail demand and storage

C is the count of mounted `subscribeActions` consumers, including the shared live
channel and the notification observer. W is an unresolved explicit `listActions`
caller. C or W permits discovery. D is a separate count for an actually opened
dialog/details body or an explicit command-follow owner of a particular run.
Cached rows, health listeners and event-only listeners do not count as C, W or D.
An initial list containing a running or unknown run never creates D or starts
runtime follow timers.

Discovery maps list summaries directly, sharing base validation, status, message
code and resource mapping with detail. It performs **zero generic or specialized
detail reads** and never calls the effectful `runtime.project()`. In-memory retained
runtime evidence is merged under the existing sequence/ownership rules, without
creating follow demand. The shared summary store keeps base fields, steps and
minimal recovery presentation, never fetched output, log or message history.
Where the existing `ActionRun` port requires arrays, the boundary supplies empty
compatibility arrays; those are not evidence of a loaded empty history and do not
receive data from a targeted detail response.

`observeActionDetail(hostId, runId, listener) -> unsubscribe` supplies separate
loading/ready/error state, with ready carrying the coded full run. Its owner is
keyed by identity, reader lifetime, host and run. A feature hook handles cleanup
and locale rendering. A new summary preserves separately cached output and marks
its freshness appropriately; a stale list cannot overwrite newer detail/status.
Details display loading/unavailable/error until real detail arrives. Generic reads
use `get-activity`; receipt-owned reads use the specialized runtime detail path.
Rights checks occur for D and are not inferred from a list or a summary refresh.
Command/control refresh modes retain their existing semantics; only equivalent
read modes coalesce, so an explicit runtime refresh cannot join a weaker read.
Demo consumers without this optional method already hold complete runs.

Known-run stream progress/messages follow these rules:

- With D>0, share a targeted read with that detail owner, one current and one
  pending read. Full output/history goes only to the detail store. Publish just
  summary fields into the shared rows. Existing explicit runtime recovery timers
  keep their own bounded follow policy and are counted separately.
  A changed host-list summary or recovery from host failure also invalidates that
  run's existing D owner. This refreshes opened remote details after poller
  snapshots, even when there is no individual message envelope. One list merge
  queues at most one refresh per demanded run; it does not create new D owners.
- With C>0 and D=0, refresh the known run through a per-run trailing throttle:
  nonterminal read starts are at least **one second** apart. The first signal
  can read immediately; later signals share one current/pending slot and one
  trailing timer, so the newest state is read even when notifications stop.
  A distinct terminal activity transition bypasses the timer once and cancels a
  redundant timer. If the current read predates that transition, it schedules one
  immediate pending flush; it cannot trust that stale current read as terminal
  evidence. Duplicate signals join the same pending flush.
  Duplicate terminal envelopes do not each get an immediate bypass. A terminal
  message stays targeted; it does not imply a new membership discovery.
- With C=0 and D=0, zero targeted reads, even if the stream stays open for events.
  Timer, abort, identity/lifetime and source-owner checks also precede publication.

Both targeted routes return detail today, but the C-only path discards their
output/messages immediately and imports only status, progress, error, steps and
minimal runtime presentation. Targeted results update only fields their contract
authoritatively carries; list-only fields absent from that contract remain from
the newest list observation. It does not register an explicit runtime follower.
Known progress does not invalidate full discovery or event history. Unknown runs
and actual membership/terminal activity changes invalidate only their known host;
an unknown host requires a fresh authorized membership read before host requests.
Late targeted detail cannot reinsert a removed or differently owned run.

## Stream routing and request budgets

The local producer emits a base-activity `snapshot` at connect and after detected
loss. The remote producer polls immediately and every five seconds, publishing a
`snapshot` on fingerprint change and an `error` once per distinct failure message.
Remote agent-owned activities have no individual message envelope; this poller is
their freshness source. Snapshot arrays are bounded windows, not complete history
or authoritative replacements for the HTTP list.

Route `snapshot` to a coalesced **host-only** full-window list refresh using its
validated `environmentId`; do not start an all-host discovery or fetch every
snapshot row's detail. That list merge revalidates only already demanded details
as described above; with D=0 it causes zero detail requests. An activity `error`
marks that host's health unavailable,
retains its rows and invalidates the same host. Only a later complete successful
list clears that signal. A valid heartbeat does nothing; a sequence gap detected
on that heartbeat is still recovery. Unscoped malformed/gap recovery marks the
subscribed host reads stale. Events-channel invalidation remains consumer-gated.
Missing/invalid host identity uses bounded membership/global recovery, never a
request to an unvalidated host ID.

### Request budget and complete history

Use the existing activity-only `limit=-1` full-window sentinel for discovery:
`start=0`, one logical list GET per environment, including hub environment `0`.
The local paginator, `normalizeRemoteActivityParamsInternal`,
`collectActivityWindowInternal` and remote merge already support that sentinel.
Validate the full-window response and completeness metadata; do not silently
accept a short successful window. Leave event pagination at 100 and sequential.

Host list flights are independent; explicit `listActions` joins their aggregate,
while a snapshot for host A never lists host B. With H hosts and no stream signals,
initial discovery makes H logical list GETs. With one SSE initial open and one
staggered initial snapshot per host, the conservative budget is **3H**, not 2H:
H initial reads, up to H open-triggered trailing reads, and up to H host-only
snapshot reads. Coalesced overlaps reduce this count. More generally cold reads
are bounded by `H + H*O + sum(S_h + E_h)`, where O counts distinct open recovery
cycles and S/E count host snapshot/error invalidations not absorbed by current
work. This is O(H) for the ordinary initial producer set, independent of row count.
Repeated failures/reopens are reported as recovery cycles, not a quiet page load.

For a 15-second half-open action window, a changed remote host emits at most three
five-second poll ticks (plus an immediate initial poll if it falls in the window).
Each snapshot is responsible for at most one host list read per tick; unchanged
hosts add zero snapshot-driven reads. An actual-window count also permits one
pending refresh carried in from an earlier tick. Distinct error signals, terminal
membership invalidation, stream opens and explicit user requests are counted
separately. A snapshot never creates unsolicited detail demand; refreshes of
existing D owners are counted on their targeted routes. A single-read
malformed/gap storm queues one trailing read per affected host; the reopen rule
below allows one replacement, never repeated interruption of that replacement.

With C only, a continuously running run has at most **15 regular targeted starts
plus one terminal flush** in that window: at most 16 on its one applicable route,
generic `get-activity` or specialized runtime detail, not 16 on each. Reopened job
execution epochs count their distinct terminal transitions explicitly. Fifty
nonterminal envelopes in one held-read burst share one current/pending slot;
spaced envelopes still obey the one-second start interval. With D>0, report detail
reads by route and the number of explicitly followed runs; their load is not a
cold-discovery cost. Measure response/output bytes as well as counts.

The safe GET retry permits at most two physical attempts per logical read, within
the same deadline. Environment, capability and auth reads, server-side poller
requests, and existing explicit runtime-follow recovery timers are separate
counters. A repeated-open storm can replace a pre-error read once per lineage,
so its held-lineage test starts at most two reads per affected host.

The sentinel reduces round trips, not response bytes or database work: list payload
and trace projection still scale with N, and active runs are not retention-capped.
Keep the current full-history product semantics. Measure list bytes and latency as
well as GET counts. If this full window is impractical, a bounded list/load-more
policy is a separate product decision, not an implicit truncation in this fix.
The full-window sentinel also removes the remote pagination pattern in which the
hub repeatedly requests growing agent windows (100, 200, ...). It does not remove
the producer's five-second poll itself or promise constant backend database work.

## Discovery, coalescer and recovery state table

Each host has a list single-flight with one current read and one pending slot;
membership reads and concurrent aggregate `listActions` calls also share work.
A host-only notification joins that host's flight, not another host's list. Each
pending slot records whether an explicit request joined it (`explicitJoiner`),
independently of notification-only interest. Starting any trailing read consumes
its pending slot before accepting more invalidations.

Initial aggregate publication still waits for the source results to settle and
validates cross-host UUID claims before seeding consumers. Independent host list
progress does not publish an unchecked initial claim. Once initialized, scoped
host refreshes merge against the retained source-owner map and cannot erase
another host's rows or overwrite its newer targeted observation.

States are `Idle`, `Reading` (no pending), `Dirty` (one pending) and `Inactive`
(no authenticated identity). Reads capture **I**, the adapter identity generation;
**L**, the activity reader/subscription lifetime generation; credential renewal
epoch **R**; host-membership epoch **E**; and per-host/run read sequence **S**.
Identity loss/replacement increments I and L and clears private observations.
Last action/change listener departure increments only L and cancels old work,
including explicit W, while retaining same-identity cached summaries. It is not
logout and does not reset the same-user notification history or RunGate identity.
A detail owner also retains an activity listener, so D cannot outlive that stream
lifetime accidentally. Event-only listeners keep the stream, but do not enable
idle activity reads when C=0.

### Reopen interruption and deadlines

Malformed envelopes and sequence gaps notify without aborting. Stream `onerror`
records an ordered error marker against current I/L. On `onopen`, interrupt a read
only if it began **before that preceding error** and its lineage has not already
used its one reopen interruption. Abort its fetch/body, transfer both current and
pending waiters once, and replace it with a protected read. A read begun after the
error is not obsolete merely because open arrived later. Initial open without a
preceding error is ordinary invalidation, never an interruption.

The replacement inherits a consumed interruption token. Further error/open pairs
cannot abort it or allocate another reopen-only replacement; they coalesce into
that already requested recovery. Thus a reopen-only storm starts at most the
original and one replacement per host, and original callers settle on replacement.
A genuine snapshot, membership change or malformed/gap invalidation during the
replacement still re-arms one ordinary pending read. That can be a third sequential
read, not another interruption: its explicit callers settle independently. Counts
must distinguish these producer invalidations from a reopen-only storm; suppressing
them to enforce a universal two-read limit would lose updates from #220.

Every activity list, targeted detail and membership HTTP read has a **40-second
deadline**, including response-body consumption and both safe-GET attempts. This
is greater than the dedicated **30-second hub-to-agent activity budget plus a
five-second required merge/transport margin**. The hub bounds agent sub-requests for HTTP
activity list and targeted remote detail with that activity-specific budget,
independent of the configurable general proxy timeout (60 seconds by default).
After expiry, listing still merges hub-owned receipt rows and returns
`X-Docklet-Host-Reason: agent.unavailable` within the browser deadline. The stream
poller retains its separate 15-second budget. The invariant is
`browser deadline > activity agent budget + required merge/transport margin`;
40 seconds exceeds 30 + 5 seconds and leaves a further five seconds of headroom. A host read lineage, including its one
replacement, has an absolute **80-second** bound from its first start; an explicit
aggregate `listActions` waiter has a **90-second** bound including membership.
Deadline expiry aborts owned work and rejects affected waiters with typed
`activity.read-timeout` / unavailable health, retaining observations. It is not
identity loss or a silent cancellation and does not automatically retry forever.
A pending notification-only slot armed by a genuine signal starts once after the
current timeout, with a fresh read/lineage deadline while C remains positive.
The expired read itself never arms a retry. Pending explicit work follows the
same start rule with its own aggregate-waiter deadline; dropping all demand drops
notification-only pending work.
Caller/lifetime cancellation remains `CancelledError` without false unavailable
health. Apply these limits to activity owners, not to the entire multi-page event
history loader. Inject clocks in deadline/throttle tests; do not wait real seconds.

| State / precondition | Event | Next state | Effect and waiter/publication rule |
| --- | --- | --- | --- |
| Inactive | Consumer mounts or stream callback from old I/L | Inactive | Register lifetime intent only; no private read or replay |
| Inactive | Authenticated new identity | Idle, then Reading if C/W | New I/L; reattach mounted observers and fetch fresh, never reuse old waiters |
| Idle, valid identity | Consumer mounts or explicit `listActions()` | Reading | One discovery; concurrent mounts/requests join it |
| Idle, C=0/W=0 | Snapshot, activity error, reconnect, gap or malformed envelope | Idle | Mark stale/health as applicable; zero requests; next mount/request reads fresh |
| Idle, C>0 | Host snapshot, unknown run, membership/terminal activity change | Reading | One scoped host list; unknown/unscoped membership goes through the shared membership owner |
| Any, current source/host | Activity `error` | Dirty or Reading with demand, otherwise Idle | Advance host health sequence, retain rows, mark unavailable and invalidate host; no old list can clear this newer health signal |
| Any | Valid heartbeat without gap | Unchanged | Zero reads; gap detection precedes the no-op heartbeat handler |
| Reading | Ordinary explicit request | Reading | Share current promise |
| Reading | Host snapshot or ordinary unknown/member/terminal/gap/malformed invalidation | Dirty | Allocate one pending promise for affected scope; no abort |
| Dirty | Further ordinary signals or explicit request | Dirty | Reuse pending; a request sets `explicitJoiner`; original callers keep current, later callers await pending |
| Reading or Dirty | Stream `onerror` | Same state | Save ordered error marker, run existing identity check; no activity restart on error alone |
| Reading or Dirty, unspent token, started before preceding error | Stream reopen | Reading, protected | Abort current once, transfer unresolved current/pending waiters to replacement, reject obsolete publication |
| Protected replacement | Further reopen-only signals | Same state | Reuse protected recovery promise; no further abort or reopen-only pending work |
| Reading or Dirty, started after error / initial open | Stream open | Dirty | Ordinary invalidation; no interruption of the newer useful read |
| Reading or Dirty | Known progress/message | Same discovery state | D feeds coalesced detail; C-only uses one-second trailing throttle; no discovery/event refresh for progress |
| Reading | Current succeeds with matching I/L/R/E/S | Idle | Apply source/sequence merge and health; resolve this read's callers |
| Dirty, C>0 or pending explicit W | Current succeeds or fails | Reading | Settle current callers, then clear pending at the next read's START; start that read once |
| Dirty, C=0 and no pending explicit W | Current finishes | Idle | Drop notification-only pending work; resolve it without a new read; next demand reads fresh |
| Any Reading (including a trailing read) | Ordinary data/malformed/gap notification after that read started | Dirty | Re-arm one pending slot; do not lose updates during a trailing read |
| Reading or Dirty | Initial list failure | Idle or pending Reading | Reject affected waiters; publish typed global/host health without inventing successful empty history |
| Reading or Dirty | One host fails or reports incomplete | Idle or pending Reading | Keep its retained rows, accept valid source rows, mark host health; only a complete source may delete missing rows |
| Any, change listeners remain | Last activity consumer unmounts | Current state or Idle | C=0; explicitJoiner pending work may finish; drop notification-only pending work; D and its timers clean up with their owners |
| Any | Last action and change listener leaves | Idle, new L | Preserve current teardown policy: abort and immediately reject current/pending explicit W too; no old trailing work, timers, callbacks or publication |
| Any, I unchanged | Credential renewal for same user | Reading if demand remains | Increment R, preserve seen/dismissed history, retire old credential requests and replay within shared work; old R cannot clear current auth/data |
| Any, current I/R | Observed 401, logout or identity replacement | Inactive or fresh Idle | Immediately increment I/L, clear runs/detail/output/health and RunGate state, reject all old waiters and abort; do not await aggregate settlement |
| Reading or Dirty | Host set changes | Dirty if demand, otherwise Idle after retirement | Advance E; reject obsolete membership writes, remove departed hosts and demand owners; queue one fresh host discovery, retain unaffected observations |
| Any | Old list/detail fulfills or rejects late | Unchanged | Check I/L/R/E, owner, abort and S before state/health publication; no removed run or newer health can be overwritten |
| Reading or Dirty | Read or lineage deadline | Idle, or Reading once for a genuinely armed pending slot | Abort and reject current callers with typed timeout/unavailable; genuine pending notification starts once with fresh deadline while C>0, or explicit pending W starts within its own deadline; expiry alone never retries or clears identity |
| Any explicit aggregate waiter | Its 90-second deadline | Waiter rejected | Do not await a drain of future signals; shared work needed by other owners retains its own deadline |
| Reading or Dirty | Explicit abort/cancel | Idle or Inactive | Abort current, reject current/pending explicit waiters, clear pending; cancellation never means host unavailable |

For host changes, do not discard unaffected hosts' valid targeted observations:
use the reader's existing per-host/full/run sequences as well as the membership
epoch. Raw activity UUIDs retain their source-owner validation. A collision from
another host is invalid data, not a reason to change resource identity.

Ordinary notification waiters settle on the read they joined, even while a newer
read is pending; an eligible reopen transfers waiters exactly once instead. An
ordinary old observation is not labelled post-signal freshness. No HTTP snapshot
is guaranteed to include a later commit. Signals spanning completed reads may
cause more sequential reads; per-read deadlines and waiter ownership prevent an
infinite drain from keeping an existing caller pending. The same I/L/owner checks
guard abort-honouring errors and abort-ignoring late completions.

Detail owners use the same current/pending rule. First D starts one read; another
D joins it; stream invalidation queues one pending refresh. Last D closes that
owner unless a targeted notification read still has its own active consumer.
Closing or switching a view prevents late state/toast effects. Discovery never
recreates D. Restarting the hub is observed through recovery and ordinary reads;
no browser state is evidence that server work stopped or should be restarted.

## Behavioural test and red-mutation matrix

Use controlled deferred responses and observable read-start/publication barriers,
not wall-clock sleeps. Fetch doubles must honor `AbortSignal` both before response
and during a held body. Keep a separate deliberately abort-ignoring double to
prove generation/ownership guards. A red proof must exit 1 on an asserted behaviour,
not compilation, timeout, hanging promise or an equivalent mutation.

| Guard | Required observation | Red mutation |
| --- | --- | --- |
| Cold discovery budget | Without stream signals, 0/1/100/about 460 rows per source, including runtime/queued/unknown rows, yield exactly one logical list GET per source and zero requests to either detail route; concurrent callers share work | Restore per-row detail mapping, or restore effectful `runtime.project()` separately |
| Staggered producer snapshots | Hold/release different hosts, emit H initial snapshots during and after initial/open-triggered reads, and assert exact per-host counts including the conservative 3H case; D=0 gives zero details; one existing D owner gets fresh output after its remote summary changes | Route snapshot to all-host discovery, per-row detail or immediate uncoalesced reads; omit demanded-detail invalidation on list change |
| Stream health and heartbeat | `error` retains rows, advances only its host's health and triggers its coalesced list with demand; a newer host list clears it, an older one cannot; heartbeat reads zero unless its ID reveals a gap | Ignore error, clear newer health on old success, or refresh on every heartbeat |
| Full-window transport | Real local, Direct and Edge handler fixtures return the same complete history for `limit=-1`; web rejects short complete windows, retains partial-host evidence and leaves event limit=100 | Restore activity pagination at 100, truncate a remote window, ignore incomplete header, or change event page size |
| List-only serialization and compatibility | Non-nil `[]` survives agent decode/merge/re-encode while nil remains absent; only list response schema changes; detail/stream/runtime carrier schemas and v6 file/fingerprint stay unchanged | Use `omitempty`, add fields to shared Activity, or attach projection in the stream/poller base list |
| Projection parity and chunking | Compare ordinary summary to `GetActivityDetail`, local/remote receipt summary to `getRuntimeActivityInternal`, including 4,096-byte steps, 256 transitions, 64 KiB and detail-only body trimming; more than 1,000 IDs respect chunks of 100; SecurityPage or ConnectSteps renders a coded step | Drop trace, rank raw messages before transitions, mix remote agent trace into receipt rows, omit bounds/chunking, or call detail per row |
| Recovery labels on cold load | With no retained browser evidence, released and active-unknown receipt summaries render released/unknown in toast and collapsed row with zero detail requests; absent reason still gives unconfirmed; grants remain absent | Drop disposition/unknownReason or infer permissions from summary presentation |
| Missing projection | Old optional-field fixture gives base summary with unavailable steps and zero eager fallback; opened detail restores real steps | Treat absent trace as complete empty trace or add discovery fallback |
| Detail demand and storage | Quiet closed rows/toasts read zero detail; one opened run reads only it, duplicate views share; close/switch cancels ownership; C-only targeted results never store output/log/messages in summaries; separately cached detail survives list refresh | Observe all rows, copy detail histories into summaries, render unloaded output as empty, clear cached output on list refresh, or remove last-owner cleanup |
| C-only targeted rate | Fifty activity/message envelopes for one known running run, under fake clock and read-start barriers: at most 15 regular starts plus one terminal flush in 15 seconds on its applicable route, one trailing timer, no output retained; D follows the separate detail owner | Remove throttle, bypass once per duplicate terminal envelope, or create runtime follow demand from a list row |
| Notification routing | Known progress/message remains targeted and coalesced; unknown/member/terminal triggers list; no event history read without an event consumer | Restore full discovery per known progress or read event history without a consumer |
| Recovery storms (#223) | Malformed/gap storm during one held read: zero aborts, one pending. Separate error/open storm over an abort-honouring held body: one eligible abort, at most two started reads, waiters transferred once, replacement completes; a read begun after error is not interrupted | Restore unrestricted recover aborts; remove start-before-error or consumed-token guard; assert abort/start/publication counts |
| Agent forwarding deadline | Hold the agent past its dedicated 30-second budget with the general proxy timeout at 60 seconds: hub returns receipt rows and `agent.unavailable` before the 40-second browser boundary; cold web load shows these rows with partial health and no timeout, for Direct and Edge | Remove the activity-specific forwarding budget: the held agent outlives the browser deadline and the cold receipt-row assertion fails |
| Stranded read and deadline | Reopen unsticks pre-error held body; fake-clock 40-second expiry aborts response/body and rejects with timeout health, no stale publish; 80-second lineage and 90-second aggregate bounds reject existing waiters without another signal; a genuine pending notification starts exactly once after expiry with fresh deadline, expiry alone starts zero reads | Remove deadline timer or body abort, or swallow timeout as cancellation; use an assertion at the injected clock boundary |
| Trailing notification | After trailing-start barrier, another signal queues exactly one third read and reveals newly committed state | Clear pending at completion, reuse consumed pending, or cap total reads at two |
| No consumers | Cached history plus event-only/health-only listeners produces zero activity reads on open/reconnect/messages; re-mount reads fresh | Count cache/event listeners as demand or auto-start idle recovery |
| Lifetime versus identity | Last action/change listener leaving increments L, rejects both current/pending W and prevents late callbacks; leaving only C while change listeners remain preserves I/L and permits explicitly joined pending W, but drops notification-only pending work | Treat teardown as identity loss, omit L guard, or omit pending explicitJoiner flag (test both flag values) |
| Immediate 401 | One responding source/identity-owned request returns 401 while another list/body is held; old runs, outputs and notifications clear before held response is released; old waiters reject | Delay unauthorized handling until aggregate completion or remove immediate identity clear |
| Abort handling (#223) | Abort-honoring list/body double rejects on real cancellation while another response is held; no false unavailable health, stale publication or trailing work; abort-ignoring double also cannot publish | Remove all abort checks, then each distinct protection where independently meaningful; retain an active legitimate request to make missing cancellation observable |
| Identity and renewal | Replacement/loss/new same-user login clears private state; same-user credential renewal preserves seen/dismissed policy; captured callbacks and old credential work cannot act as the current identity | Remove I/L checks, old-callback checks or R guard |
| Host/order/partial state | Removed host's late list/detail cannot reinsert rows; newer targeted result beats older full read, old failure cannot overwrite newer health, partial sources retain rows | Remove E/owner/run/health sequence guards or delete missing rows from incomplete source |
| Initial failure and retry | No manufactured empty success; initial per-host failures seed recovered history once, later outage recovery still notifies real transitions | Initialize successful history on health-only publication or reseed every recovery |
| Restart and missed completion | A completion committed without a delivered event appears after reconnect in subscribed summary and opened/followed detail; never repeat command admission | Skip recovery, trust envelope snapshot as final, or retry command on viewer restart |

## Existing assertion migrations

Locations refer to the original test tree; test titles identify cases after line
movement. Keep every protected ordering, immediate-clear, notification and runtime
assertion. The following are assertion/fixture migrations, each requiring its
named red proof; a compilation failure, timeout or a filter matching no test is
not evidence. Any additional assertion migration is enumerated with its red proof
in implementation evidence before review, rather than silently changing tests.

| Existing location | Replacement observation and synchronization | Red mutation |
| --- | --- | --- |
| `activity-resilience.test.ts:22` (four details per host) | Hold host list responses instead; assert zero details, one list per host, and healthy host list progress while another is held; initial publication still validates all source claims | Restore detail loop, serialize host list starts, or publish unchecked initial collisions |
| `activity-resilience.test.ts:47,58` (shared and trailing full loads) | Keep shared-promise/old-versus-fresh status assertions using summary response gates; discovery detail count becomes zero | Remove single-flight or clear pending at completion |
| `activity-resilience.test.ts:77` (known run during discovery) | Hold list response; a permitted targeted refresh wins before release and remains newer afterwards | Drop per-run/full sequence guard |
| `activity-resilience.test.ts:120` (recovery 401) | Hold a list body, not discovery detail; either use eligible pre-error reopen or an independent current-identity request to return 401; assert clear/rejection before held release | Delay unauthorized until aggregate settlement or remove immediate clearing |
| `consumer-lifecycle.test.ts:91` (both ordinary/recovery 401 variants) | Same held-list barrier and explicit error marker for eligible reopen; preserve immediate empty publication, 401 waiter rejection and unchanged request/publication counts after late release | Remove identity clear, I/L guard or permit obsolete trailing read |
| `single-flight.test.ts:109` (recovery during held replacement) | One eligible abort transfers waiters; repeated error/open pairs cannot abort the protected replacement; separately show a real notification during it queues a third sequential read | Remove interruption token, transfer or trailing-pending guard |
| `single-flight.test.ts:131` (resolved obsolete continuation) | Supply preceding error marker; eligible reopen suppresses obsolete continuation and resolves both callers from replacement | Omit abort/work-identity check before publication |
| `single-flight.test.ts:140` (recovery during publication) | Pure notify during publication preserves current caller's result and queues one read; eligible pre-publication reopen transfers unresolved callers; both boundaries use explicit barriers | Reuse consumed pending or transfer already settled waiters |
| `single-flight.test.ts:32,47,96` and `consumer-lifecycle.test.ts:68,81` | Preserve current/pending routing and no-demand guards; add explicitJoiner true/false departure branches, with actual teardown rejecting both W when all action/change listeners leave | Remove consumer check, explicitJoiner or lifetime cancel |
| `notification-budget.test.ts:10,34` (ten messages, twelve detail GETs) | Hold discovery list; use fake-clock 50-envelope test and start barriers; exact one-host discovery count, zero discovery details, throttled targeted counts and no events invalidation | Remove throttle, restore discovery fan-out or refresh event history |
| `notification-budget.test.ts:39` (renewal and event-only listener) | Split C-present targeted freshness from C-absent zero activity reads; both keep events invalidation only on actual events message | Count event-only listener as C or disable legitimate C-targeted refresh |
| `stream-read-freshness.test.ts:24-40` routing cases | Add snapshot/error/heartbeat and staggered host counts; distinguish notify-only malformed/gap from eligible error→open; retain fresh post-signal result assertions | Treat snapshot as all-host discovery or allow unrestricted recover interruption |
| `activity-reader.test.ts:14-26,60,214` (failure/identity matrix) | Inject malformed/missing/misattributed summary rows during discovery; move detail404/mapping failures to targeted/detail-owner cases; preserve valid rows, typed health and collision checks | Accept malformed source data, erase source ownership or ignore targeted errors |
| `activity-reader.test.ts:93,127,173` and `activity-ordering.test.ts:27,57` | Replace detail-held full reads with held list summaries; retain newer-targeted/newer-full ordering, teardown rejection and immediate 401 assertions | Remove run/full/health sequence or I/L guard |
| `activity-reader.test.ts:151` (one targeted hub read) | Keep one permitted target request and host-scoped identity, synchronize with throttle eligibility; summary resource IDs now come from list fixture | Drop scoped resource mapping or make progress rediscover hosts |
| `activity-reader.test.ts:190` and `activity-lifecycle.test.ts:80,108,121,145` | Keep targeted lifetime assertions; list-held query barriers replace implicit discovery details; logout/unsubscribe still reject without successful empty cache results | Remove lifetime/identity cancellation or swallow CancelledError as success |
| `activity-partial.test.ts:47` (marked page 0/1) | One full-window response per host; cover marked initial/refreshed response and retain prior rows; reject short unmarked full windows instead of accepting a second page | Ignore partial header or accept truncated full window |
| `activity-partial.test.ts:83` (late healthy host) | Observe partial host's list mapping/publication barrier instead of its discovery detail request; preserve host-local metadata assertion | Share completeness state between hosts |
| `activity-renewal-ordering.test.ts:28,51,68,86` | Hold old credential list/membership effects, replay under R once, preserve every unseen-row/health assertion; replace incidental full-load counts with explicit lineage counts | Remove R or source/global health sequence checks |
| `runtime-retained.test.ts:14,34,45,55,61,69,79,89,105,135` | Put receipt presentation in summary fixtures; no `project()` call on discovery; preserve released/unknown and newer terminal/late evidence from a separately followed run, including failure retention | Restore effectful project, drop summary recovery or let older list overwrite retained evidence |
| `loader-state.test.ts:14` fixture and its discovery/recovery cases | Gate the list effect instead of detail; apply the notification/reopen distinction and the same pending/start, explicitJoiner and deadline observations | Restore detail-on-discovery or lose pending state |

Shared `activity-reader.test-support.ts`, runtime fixtures and activity feature
notification/query-lifecycle fixtures carry real list DTO fields, including optional
trace/recovery. Their existing UI assertions stay intact. Move any discovery-held
detail barrier to a list/read-body or publication barrier. In particular preserve
initial failed-source history seeding, recovery notifications and immediate 401
clearing in the notification, source-notification, observation and query-lifecycle
suites. The consumer parity test also asserts a SecurityPage or ConnectSteps label.

The read-resilience section of `adapter-live-capabilities.md:103-119` changes
atomically with reader implementation. Replace the four-concurrent-detail policy
with zero discovery detail, list-only projection and host-scoped snapshot refresh;
replace unconditional reconnect abort/transfer with the eligible pre-error,
once-per-lineage reopen rule and deadlines; qualify caller routing as ordinary
current/pending versus one eligible waiter transfer. Link this record for rates,
consumer/lifetime rules and budgets. Event history's sequential 100-items-per-page policy
and consumer gate remain. The older description must not coexist with shipped
reader behaviour that contradicts it.

The implementation evidence must enumerate exact changed tests, request counts,
physical retries, red commands and exit codes. Practical #225 acceptance later
measures cold page loads and a 15-second action window against the same retained
history through Direct and Edge; require an order-of-magnitude detail reduction
and fresh running/completion/output observations. Unit counts do not replace that
operator-approved sandbox acceptance.
