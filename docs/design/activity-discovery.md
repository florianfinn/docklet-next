# Activity discovery and bounded recovery

This proposal covers the activity reader foundation for #225 and #223. It keeps
the durable run and admission contracts from [runtime control](runtime-control.md)
and the observation rules from [the HTTP adapter](adapter-live-capabilities.md).
The proposed behaviour below requires coordinator approval before implementation.

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

| Consumer | Required fields | Already on `Activity` | Detail-only requirement |
| --- | --- | --- | --- |
| Activity Center list and shell run counts | ID, host/resource identity, kind/title/target, status, progress, services, start/end, error, runtime knowledge | All, via existing mapping; `type` supplies kind and coded title | None for collapsed rows |
| `ActionToasts`, `RunToastCoordinator`, `ActionToastView` | Above fields, terminal transitions, phase label, ordered steps and step counts; source completeness for initial-history suppression | Base fields, current `step`, `latestMessage` and `messageCode`; no ordered steps | `stepTrace` for the existing notification designs; raw output and complete message history are unused |
| Expanded `ActivityCenter.RunBody`, `ActionDialog`, `RunFocus`, `RunOutput` | Base fields, complete steps, retained output, truncation, failure diagnosis; runtime rights and recovery controls when applicable | Base fields and runtime outcome/cancel intent | `stepTrace`, `messages`, `output`, `outputTruncated`; specialized runtime evidence and grants |
| `run-gate` and runtime command follow | ID, resource/host, status, kind and timestamps; explicit runtime follow needs authoritative recovery evidence | Base fields and runtime knowledge | Specialized runtime detail for followed runtime controls; discovery must not imply that demand |
| Stack `DeploymentsTab` and `deployRuns` (#198 consumer) | Host-scoped `stackId`, deploy/redeploy kind, status and timestamps for history; selected run uses `RunFocusView` and output | Identity/filter/list fields | Only the shown run needs full detail; older collapsed history does not |

`mapActivityDetail` also maps all messages into `log`, falling back to the latest
coded message. Full `log` is not needed for list/notification decisions. `services`
comes from `affectedServices`, not detail. `restorePointId` has no source in the
current generic activity mapper; this foundation must not invent backup evidence.
The #198 owner still decides which editor/runtime deployments belong in history
and supplies the selected-run wiring. This proposal does not change stack history.

## Proposed list projection and demand

Add one optional `stepTrace` field to `types/activity.Activity`, using the existing
`StepTraceEntry` shape and 256-transition ceiling. Present `[]` means a known empty
trace; absence means that a producer did not supply the projection. Keep the
existing top-level detail fields unchanged. Regenerate OpenAPI and TypeScript in
one isolated contract commit; no migration or new endpoint is needed.

Produce the trace for authorized list rows with a batched transition query scoped
to those row IDs, partitioned by activity ID. Reuse the existing transition and
status semantics, including consecutive-step collapse, coded labels, timestamps
and terminal step state. Runtime-owned list rows must take their trace from the
same retained runtime projection that produces their status; do not mix an agent
trace with hub receipt knowledge. Apply projection after runtime replacement so
`projectRuntimeListInternal` cannot accidentally discard it. Preserve visibility,
source labels, remote ownership and partial-response headers. Do not implement the
projection by calling `GetActivityDetail` per row or loading retained output.

An additive optional field permits older agents to omit it. Omission cannot trigger
bulk detail fallback. Summary consumers keep base progress/status and treat steps
as unavailable; an opened/followed run can request detail. A release requiring
complete notification step parity on every host needs the corresponding agent
compatibility review; optional schema syntax alone does not guarantee that parity.
No protocol version is selected by this design-only step.

Use a separate summary mapper, sharing base validation/status/code/resource mapping
with the detail mapper. Discovery maps list rows directly and performs **zero
generic or specialized detail reads**, including for queued, running and unknown
runtime rows. Do not call the effectful `runtime.project()` during summary mapping.
Merge already retained runtime observations in memory with their ordering and
outcome-knowledge rules; summary data cannot synthesize recovery permissions.

Summary-only output is not a confirmed empty output. Keep detail-load state outside
the durable wire run, keyed by identity generation, host and run ID. Detail surfaces
must show loading, unavailable or error until actual detail arrives; they must not
render an empty-output conclusion from the summary's compatibility arrays. A new
summary preserves unchanged cached output and invalidates its freshness when the
observed run changes. A stale list cannot overwrite a newer detail's status.

Introduce an optional adapter observation method with a concrete demand contract:
`observeActionDetail(hostId, runId, listener) -> unsubscribe`, where the listener
receives a discriminated loading/ready/error state and ready carries the coded run.
The HTTP implementation shares one demand owner per identity/host/run. A focused
activity hook owns subscription cleanup and locale rendering. Demo consumers can
use their already complete runs when the method is absent. Generic activity reads
use `get-activity`; runtime-owned reads reuse the existing specialized runtime
detail and rights path, preserving authorization and explicit refresh semantics.

Demand starts when a dialog opens or a details body expands, and ends when it
closes, changes run/host or unmounts. The notification observer does not demand
full details for every displayed run. A command already explicitly following a
runtime run retains its existing follow lifetime. Merely discovering a running
row does not register that lifetime or start runtime follow timers. The #198 owner
can adopt the same method for the single shown deployment, independently.

Preserve the existing targeted detail refresh for a known run's stream notification
when there is activity demand: that request is caused by an actual notification,
not discovery. Share it with opened/followed detail demand, with one current and
one pending read per run. A known progress/message refresh must not start full
discovery or event history. Unknown IDs and membership/terminal activity changes
notify discovery; terminal `message` envelopes remain targeted. Late detail for a
removed or differently owned run cannot reinsert it. This retains the #220 routing
contract while eliminating the history-sized discovery multiplier.

### Request budget and complete history

Use the existing activity-only `limit=-1` full-window sentinel for discovery:
`start=0`, one logical list GET per environment, including hub environment `0`.
The local paginator, `normalizeRemoteActivityParamsInternal`,
`collectActivityWindowInternal` and remote merge already support that sentinel.
Validate the full-window response and completeness metadata; do not silently
accept a short successful window. Leave event pagination at 100 and sequential.

With H environments, quiet initial discovery is H activity-list requests and zero
detail requests, independent of retained row count N. The existing safe GET retry
can make at most two physical attempts per logical request after a transport
failure. A recovery signal during a held discovery permits one trailing discovery,
so that controlled window has at most 2H logical list requests and zero details.
Environment/capability/auth reads are accounted separately. First SSE open can
legitimately cause this one trailing read; it cannot reproduce the detail fan-out.

The sentinel reduces round trips, not response bytes or database work: list payload
and trace projection still scale with N, and active runs are not retention-capped.
Keep the current full-history product semantics. Measure list bytes and latency as
well as GET counts. If this full window is impractical, a bounded list/load-more
policy is a separate product decision, not an implicit truncation in this fix.
For the action window, count both activity and specialized runtime detail routes;
only targeted notifications and actual detail/follow demand can explain those
requests. Separate existing runtime recovery timers from discovery-caused reads.

## Discovery, coalescer and recovery state table

Use the existing single-flight's one current read and one pending slot. Change
`recover()` to the same non-interrupting invalidation operation as `notify()`.
Recovery signals never abort a useful discovery. Actual identity change, explicit
cancellation and retirement of obsolete host/detail ownership still abort reads.
This selects zero recovery-triggered restarts, rather than trying to classify
which reconnect in a noisy stream deserves another abort.

States are `Idle` (no work), `Reading` (current read, no pending slot),
`Dirty` (current plus one pending slot) and `Inactive` (no authenticated identity).
Each read captures identity generation G, host-membership epoch E and read sequence
S. C means mounted activity subscribers; W means unresolved explicit requests.
Detail demand D is separate and keyed by `(G, host, run)`. Cached rows, health-only
listeners and event subscribers are not C, W or D.

| State / precondition | Event | Next state | Effect and waiter/publication rule |
| --- | --- | --- | --- |
| Inactive | Consumer mounts or stream callback from old G | Inactive | Register lifetime intent only; no private read or replay |
| Inactive | Authenticated new identity | Idle, then Reading if C/W | New G; reattach mounted observers and fetch fresh, never reuse old waiters |
| Idle, valid identity | Consumer mounts or explicit `listActions()` | Reading | One discovery; concurrent mounts/requests join it |
| Idle, C=0/W=0 | Notification, reconnect, gap or malformed envelope | Idle | Mark stale only; zero requests; next mount/request reads fresh |
| Idle, C>0 | Unknown activity, membership/terminal change or recovery | Reading | One current discovery, no detail sweep |
| Reading | Ordinary explicit request | Reading | Share current promise |
| Reading | Discovery invalidation: unknown/member/terminal signal, reconnect, sequence gap, malformed envelope | Dirty | Allocate exactly one pending promise; do not abort current |
| Dirty | More of those signals or explicit request | Dirty | Reuse pending promise; original callers keep current, post-signal callers await pending |
| Reading or Dirty | Known progress/message envelope | Same discovery state | Coalesce targeted run refresh separately; no full discovery/event-history refresh for this envelope |
| Reading | Current succeeds with matching G/E/S | Idle | Apply source/sequence merge and health, resolve current callers |
| Dirty, C>0 or pending explicit W | Current succeeds or fails | Reading | Settle current callers, then clear pending at the next read's START; start that read once |
| Dirty, C=0 and no pending explicit W | Current finishes | Idle | Drop notification-only pending work; resolve it without a new read; next demand reads fresh |
| Any Reading (including a trailing read) | Notification after that read started | Dirty | Re-arm one pending slot; do not lose updates during a trailing read |
| Reading or Dirty | Initial list failure | Idle or pending Reading | Reject affected waiters; publish typed global/host health without inventing successful empty history |
| Reading or Dirty | One host fails or reports incomplete | Idle or pending Reading | Keep its retained rows, accept valid source rows, mark host health; only a complete source may delete missing rows |
| Any | Last activity consumer unmounts | Same current state or Idle | Remove C; do not cancel another consumer's explicit W; suppress later notification-only trailing work; close shared stream only when all its consumers leave |
| Any, G unchanged | Credential renewal for same user | Reading if demand remains | Preserve notification history; retire old credential requests without false host errors; retry within one shared flight, never multiply streams |
| Any | Observed 401, logout or identity replacement | Inactive or fresh Idle | Immediately increment G, clear runs/detail/output/health and RunGate state, reject all old waiters and abort; do not await other hosts or aggregate settlement |
| Reading or Dirty | Host set changes | Dirty if demand, otherwise Idle after retirement | Advance E; reject obsolete membership writes, remove departed hosts and demand owners; queue one fresh host discovery, retain unaffected observations |
| Any | Old list/detail fulfills or rejects late | Unchanged | Check G/E, host ownership, abort and S before state/health publication; late requests cannot clear newer health or reinsert removed runs |
| Reading or Dirty | Explicit abort/cancel | Idle or Inactive | Abort current, reject current/pending explicit waiters, clear pending; cancellation never means host unavailable |

For host changes, do not discard unaffected hosts' valid targeted observations:
use the reader's existing per-host/full/run sequences as well as the membership
epoch. Raw activity UUIDs retain their source-owner validation. A collision from
another host is invalid data, not a reason to change resource identity.

Every promise must settle against the read it joined, rather than awaiting a drain
of all future invalidations. Recovery now lets pre-signal callers receive that
read's observation; post-signal callers await the pending read. Do not label the
first observation post-recovery freshness. No read is guaranteed to include a
write that commits after its snapshot. A finite storm delivered during one held
read yields at most two discoveries. A continuous stream spanning completed reads
can cause further sequential reads, but cannot starve existing callers or restart
an in-flight read. Signals during a trailing read must still queue one later read;
a global lifetime limit of two would reintroduce #220's lost-update bug.

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
| Cold discovery budget | For 0, 1, 100 and about 460 rows per source, including runtime/queued/unknown rows, exactly one logical list GET per source and zero requests to either detail route; concurrent callers share the flight | Restore per-row detail mapping, or restore effectful `runtime.project()` separately |
| Full-window transport | Real local, Direct and Edge handler fixtures return the same complete history for `limit=-1`; web rejects short complete windows, retains partial-host evidence and leaves event limit=100 | Restore activity pagination at 100, truncate a remote window, ignore incomplete header, or change event page size |
| Projection parity | Compare list trace and base fields with real producer detail for ordinary and runtime runs, coded steps, repeated transitions and terminal states; no per-row detail/message/output loading | Drop trace, use agent trace for hub runtime owner, or call detail per projected row |
| Missing projection | Old optional-field fixture gives base summary with unavailable steps and zero eager fallback; opened detail restores real steps | Treat absent trace as complete empty trace or add discovery fallback |
| Detail demand | Closed summaries/toasts read no detail; one opened run reads only that run, duplicate views share; close/switch cancels ownership; selected cached output survives a summary refresh and shows errors honestly | Observe all rows, render unloaded output as empty, clear cached output on list refresh, or remove last-owner cleanup |
| Notification routing | Known progress/message remains targeted and coalesced; unknown/member/terminal triggers list; no event history read without an event consumer | Restore full discovery per known progress or read event history without a consumer |
| Recovery storm (#223) | Hold first list; deliver 20 spaced malformed/reconnect/gap signals through actual stream handlers, then a controlled burst of 75; zero aborts, at most two discoveries, original and pending callers settle after their respective response releases | Restore interrupting `recover()`; assert abort/start counts before releasing responses |
| Trailing notification | After trailing-start barrier, another signal queues exactly one third read and reveals newly committed state | Clear pending at completion, reuse consumed pending, or cap total reads at two |
| No consumers | Cached history plus event-only/health-only listeners produces zero activity reads on open/reconnect/messages; re-mount reads fresh | Count cache/event listeners as demand or auto-start idle recovery |
| Immediate 401 | One responding source/identity-owned request returns 401 while another list/body is held; old runs, outputs and notifications clear before held response is released; old waiters reject | Delay unauthorized handling until aggregate completion or remove immediate identity clear |
| Abort handling (#223) | Abort-honoring list/body double rejects on real cancellation while another response is held; no false unavailable health, stale publication or trailing work; abort-ignoring double also cannot publish | Remove all abort checks, then each distinct protection where independently meaningful; retain an active legitimate request to make missing cancellation observable |
| Identity and renewal | Replacement/loss/new same-user login clears private state; same-user credential renewal preserves seen/dismissed policy; captured callbacks and pending loads cannot act as the old identity | Remove G checks, old-callback checks or renewal guard |
| Host/order/partial state | Removed host's late list/detail cannot reinsert rows; newer targeted result beats older full read, old failure cannot overwrite newer health, partial sources retain rows | Remove E/owner/run/health sequence guards or delete missing rows from incomplete source |
| Initial failure and retry | No manufactured empty success; initial per-host failures seed recovered history once, later outage recovery still notifies real transitions | Initialize successful history on health-only publication or reseed every recovery |
| Restart and missed completion | A completion committed without a delivered event appears after reconnect in subscribed summary and opened/followed detail; never repeat command admission | Skip recovery, trust envelope snapshot as final, or retry command on viewer restart |

Keep all ordering, identity, retained runtime and notification tests. Expected
intentional assertion migrations are: discovery-detail concurrency becomes the
stronger zero-detail budget; list page counts become one full-window read; recovery
restart/waiter-transfer expectations become non-interrupting current/pending routing.
In `activity-resilience` the recovery-401 scenario must obtain 401 from a concurrent
responding request while holding discovery, rather than depending on aborting
discovery to start that request. Its immediate-clear and no-late-publication
assertions remain. Resequence detail-held discovery fixtures to hold the actual
list effect. Each changed assertion needs an individually recorded red proof;
no deletion of an old semantic assertion without its replacement and mutation.

The implementation evidence must enumerate exact changed tests, request counts,
physical retries, red commands and exit codes. Practical #225 acceptance later
measures cold page loads and a 15-second action window against the same retained
history through Direct and Edge; require an order-of-magnitude detail reduction
and fresh running/completion/output observations. Unit counts do not replace that
operator-approved sandbox acceptance.

## Implementation ownership requiring approval

The design needs narrow wiring changes outside the worker's current owned paths:
`web/src/data/http/http-adapter.ts` (full-window read, mapper, demand and stream
routing), `map-dto.ts` (shared summary mapping), `runtime-commands.ts` (remove
discovery-induced runtime demand while preserving retained evidence), and
`web/src/data/adapter.ts` (optional detail observation port). Keep the detail hook
inside `web/src/features/activity`; its loading/error text uses that feature's
English/German catalogues. Associated HTTP/runtime regression fixtures also need
explicit test ownership. `data/live.ts` can retain its current summary role.

These are a dependency-gap for implementation until the coordinator assigns those
specific paths or assigns the shared wiring to its contract owner. The list DTO
and generated contract changes must be coordinated with the parallel contract
worker and kept in one isolated commit. Stack history wiring, image/registry/update
modules, protocol-version selection and running-system acceptance remain with
their owners. This document authorizes no implementation or external action.
