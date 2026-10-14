# Run gate and single-hub operation

## Scope and guarantees

Exactly one hub process in one hub container operates against a PostgreSQL
database. A second hub using that database is unsupported and must not accept
work. Agents are separate processes; this decision does not limit their number.
This record covers the live hub, restart and crash recovery. A crashed hub does
not automatically resume an operation. An agent may still finish an action it
already accepted, so the next intake must read the actual resource state before
another intervention.

A gated run has at most one active row per `(environment_id, gate_scope)`.
`gate_key` identifies the operation. Gated runs start as `running`, never
`queued`. An ungated, queue-opted activity can instead start as `queued` while
waiting for a local concurrency slot; it has no gate scope or gate admission
result. A gated `running` row occupies its scope; an ungated `queued` row
waits without holding a slot, and an ungated `running` row holds one.
`success`, `failed` and `cancelled` are terminal. A set
`cancel_requested_at` is a durable cancellation request, **not** another value
of `status`. There is no automatic operation replay or atomicity guarantee
for external effects. The guarantees cover unique gated admission and one
durable terminal transition per activity row.

## States and transitions

`message_code` is the code in the activity snapshot. When mentioned,
`activity_messages.code` is the code in durable history. Every terminal write
sets `ended_at`, `duration_ms` and `updated_at` in the same transaction as
`status` and `message_code`. A repeated or late completion leaves a terminal
row unchanged and writes no second final message.

### Gated runs

`StartGatedActivity` creates `running` with `run.started` and returns the only
executable work context. Gated runs do not wait for an activity slot.

| From → to | Trigger and condition | Written fields | Code |
| --- | --- | --- | --- |
| No run → `running` | Gate admits an operation. | `status`, `gate_scope`, `gate_key`, `started_at`, `message_code` | `run.started` |
| `running` → cancellation requested | Hub accepts the first request and signals its local worker; status stays `running`. | `cancel_requested_at`, `latest_message`, `message_code`, `updated_at`; one audit message | `activity.cancel_requested` in snapshot and history |
| `running` → `running` | Worker reports progress. | `progress`, `step`, `updated_at` and optional messages; without cancellation, also `latest_message` and `message_code` | Validated progress code, default `activity.message`; after cancellation the snapshot keeps `activity.cancel_requested`. |
| `running` → `success` | Worker reports success without a cancellation request. | Terminal fields, `progress=100`, optional final message | `activity.success` |
| `running` → `failed` | Worker reports failure without a cancellation request. | Terminal fields, `error`, optional final message | `activity.failed` |
| `running` with cancellation request → `cancelled` | Worker returns, whether it reports success or failure. | Terminal fields and final message; no error from the overridden result | `activity.cancelled` |
| `running` → `failed` | Startup repair finds an abandoned run without a cancellation request. | Terminal fields, interruption reason and final message | `activity.failed` |
| `running` with cancellation request → `cancelled` | Startup repair finds an abandoned run with a durable cancellation request. | Terminal fields and final message | `activity.cancelled` |
| Terminal → same terminal state | Completion, late worker write or repair repeats. | None | Existing code remains. |

### Ungated activities and job runs

`StartActivity` can create a queue-opted ungated activity as `queued` when its
concurrency slot is unavailable or deferred. With a free slot, it starts as
`running`. `AwaitActivitySlot` changes a waiting row to `running` when the local
worker obtains a slot. The ordinary activity path defaults to
`activity.queued` and `activity.running` when the caller supplies no code;
`run.started` belongs to gated admission. An ungated row has no gate-scope
exclusion.

| From → to | Trigger and condition | Written fields | Code |
| --- | --- | --- | --- |
| No activity → `queued` | Queue-opted work has no slot or defers it. | `status`, `started_at`, `message_code`; no gate scope | `activity.queued` |
| No activity → `running` | Ungated work starts with a slot or needs no queue. | `status`, `started_at`, `message_code`; no gate scope | `activity.running` |
| `queued` → `running` | Local worker acquires a slot without a recorded cancellation. | `status`, `updated_at`, `message_code` | `activity.running` |
| `queued`/`running` → cancellation requested | Hub records the first request and signals the local worker; status stays active. | `cancel_requested_at`, `latest_message`, `message_code`, `updated_at`; one audit message | `activity.cancel_requested` |
| `queued` with cancellation request → `cancelled` | Waiting worker stops without beginning the operation. | Terminal fields and final message | `activity.cancelled` |
| `queued`/`running` → `success`/`failed`/`cancelled` | Worker returns, or startup repair closes abandoned work. | Terminal fields and at most one final message | Matching `activity.<status>` |

A queued worker whose cancellation was recorded must not begin the operation;
its caller completes the row as `cancelled`. If slot acquisition races with
cancellation, the local worker checks the durable request before doing work
and completes the cancelled activity. Ungated rows use the same terminal
status condition and one-final-message rule as gated rows. Startup repair
also closes abandoned ungated queued and running work before accepting new
work. The existing two-minute sweep for ungated untracked work remains for
now; startup repair could replace it in a separate follow-up.

A job-run snapshot is a projection of its durable coordinator. An explicit
retry may reopen it after a terminal state; that is a new coordinator state,
not a reopened gated run. Its `job.*` codes and
`activity_sync_receipts` follow the coordinator rather than the gated
transition table.

## Admission and gate

Admission locks the scope in a PostgreSQL transaction, reads active gated runs
and creates at most one new run. The existing transaction-scoped advisory lock
per `(environment_id, gate_scope)` serializes concurrent requests within the
single hub. The process-wide lock below serves a different purpose and does
not replace atomic scope admission.

| Finding at admission | Result | Database change |
| --- | --- | --- |
| Active run with the same `gate_key` and no cancellation request | `run.joined` with its ID; only the original caller owns a worker. | None |
| Active run with a different `gate_key` | `run.busy` conflict with the active run ID. | None |
| Active run with the same `gate_key` and cancellation requested | `run.cancelling` conflict with the active run ID, even for `Check`. | None |
| No active run; `Check=true` and `Recheck=false`; last successful run for this operation and scope ended less than 30 seconds ago | `run.fresh` with that ID. | None |
| Otherwise | `run.started` and a new run ID; only this caller starts work. | New `running` run |

The active-run check always precedes freshness. `Recheck=true` bypasses only
the 30-second freshness result, never an active run. Only success is fresh;
the interval compares `ended_at` against PostgreSQL time. At exactly 30
seconds the previous run is no longer fresh. After expiry or `Recheck`, the
worker reads resource state again before acting. Joining, rejecting and
returning a fresh result start no worker and do not change the existing row's
message code.

## Cancellation

The first cancellation request writes `cancel_requested_at` using PostgreSQL
time, `activity.cancel_requested` and an audit message naming the requester in
one transaction. The hub then publishes the message and snapshot and signals
its locally registered worker. A repeated request is idempotent: no second
audit entry and no new timestamp. The local signal is transient; the column
is durable. Local worker registration must exist before cancellation can be
accepted for a newly admitted run.

While cancellation is unwinding, status and scope remain active. Progress and
metadata writes may continue, but preserve `cancel_requested_at`,
`message_code=activity.cancel_requested` and the cancellation text in
`latest_message` until terminal completion. Later progress messages retain
their own codes in history. The snapshot therefore shows cancellation even
while the worker still reports progress. Service write paths for the owning
worker detach their database work from the cancelled work context with a new,
bounded write deadline; callers need not do so individually.

Only the worker's return and terminal write release the scope. A worker that
never returns holds it until the hub process ends; the next hub repairs the
row. A cancelled agent call may already have caused an external effect.
`cancelled` does not promise a rollback.

## Restart, crash and one-time completion

After acquiring the single-instance lock and before accepting work, a new hub
immediately repairs all gated `running` runs left by the prior process. It
does not wait for lease age because no old local worker exists in the new
process. A durable `cancel_requested_at` makes the result `cancelled`;
otherwise it is `failed`. Both receive an interruption message, matching
`activity.<status>` code and terminal timestamp. Repair is idempotent and
also covers a crash between row creation and worker start or during
cancellation. Abandoned ungated queued and running work is repaired too;
special coordinators, including job runs and an occupied self-update
completion, reconcile their own durable state.

The repair frees the gated scope. The next intake reads the resource state
before acting, following the current Arcane behaviour. An agent may still
finish an action accepted before the crash, so the next action can overlap
that external action. Blocking a scope until the agent confirms the outcome,
with a query by run ID, timeout and manual release, is a separate follow-up
requiring a hub–agent contract change.

A terminal write atomically checks `status IN ('queued', 'running')` and
writes status, code and latest message in one transaction. A second
completion finds no active row and adds no final message. If the hub crashes
after commit but before the live event, the new hub reads the terminal
snapshot and clients reload it after reconnect. If it crashes before commit,
the new hub repairs the still-active row. A job coordinator's completion also
uses `activity_sync_receipts`: the run ID and last applied coordinator
version are written transactionally with the projection and message.
Replaying the same version adds no second effect, even after pruning activity
history. Keep the receipt until the coordinator run has demonstrably left.
These rules do not guarantee exactly one external agent action; its actual
outcome must be checked after a crash.

If a normal pool connection fails while the dedicated lock connection still
holds its session, a completion retries its write up to three times for
retryable errors. After the final failure it returns the error; it must not
claim success or free the durable scope. The row remains active and blocks
another gated admission on that scope. A later explicit completion retry can
finish it after database access recovers; otherwise an operator restart
triggers startup repair. Pool failure alone is not proof of lock loss.

## Single-instance lock and database loss

At startup, before migration, repair, background jobs and HTTP readiness, the
hub opens one dedicated physical connection (`*sql.Conn` or pgx
`Acquire`) and acquires a fixed, namespaced PostgreSQL **session** advisory
lock with `pg_try_advisory_lock`. It holds that same connection for the entire
process lifetime and does not use it as a general query pool connection. A
one-off query through the pool or a transaction advisory lock is insufficient.
If the lock is held, startup fails immediately with the stable operational
code `hub.already_running`, without opening HTTP or starting jobs; there is no
wait for release. This is a startup error, not a run message code.

The hub records the connection's backend PID and probes that same dedicated
connection every second. Each probe has a one-second deadline and verifies
both the unchanged backend PID and the granted advisory lock in `pg_locks`;
a successful pool query does not count. A failed probe, missing lock, changed
PID or probe timeout immediately clears readiness and starts fail-stop.
Independently, a monotonic watchdog requires a successful proof at least once
every two seconds, so a stalled probe cannot extend the deadline. Admission
checks readiness and the watchdog deadline both before and after its database
transaction, before returning an executable work context. Fail-stop rejects
new work, cancels local workers and exits with `hub.db_lock_lost`. It must not
reconnect and silently continue on a new pool connection. Shutdown gives
workers at most one further second
to stop; then the process terminates instead of releasing the session lock
while a local worker can still write. Controlled shutdown releases the lock
only after all workers have stopped writing.

With a scheduled process, loss of the session is detected within two seconds
and the old hub process exits within one further second: the hub-side overlap
window is at most three seconds. An arbitrary operating-system pause can
delay execution of this watchdog; admission must check its monotonic deadline
on resumption before accepting work. A detached agent action can continue
beyond process exit, so its overlap with a newly admitted action has no
finite bound from the hub lock alone. A new hub can acquire the released lock
and repair the old rows before opening its API. If PostgreSQL still considers
the old session alive during a network partition, the new hub fails fast with
`hub.already_running`. It never bypasses the held lock.

No process epoch is added: terminal status conditions prevent stale database
completion after repair, while an epoch would not stop an already accepted
external agent action. The residual external overlap is recorded above and
requires the separate hub–agent follow-up.

## Reconciliation with the multi-process mechanism from PR #42

The decision in [#43](https://github.com/florianfinn/docklet-next/issues/43)
changes the operating assumption of
[PR #42](https://github.com/florianfinn/docklet-next/pull/42). Shipped
migrations remain immutable; schema removal requires a new forward migration.

| Component | Decision | Reason and target state |
| --- | --- | --- |
| Transaction advisory lock per run scope | Keep | It serializes concurrent admission through commit in the one hub. |
| Durable cancellation request, audit message and immediate live snapshot | Keep | Cancellation survives a hub crash and remains visible to clients. |
| Owner token `gate_owner` and fencing of progress and completion writers | Remove | The dedicated session lock and fail-stop bound old hub writes; terminal status conditions still guard against duplicate completion. External effects retain the stated overlap risk. |
| `gate_lease_at` and 30-second heartbeat | Remove | The lifetime lock identifies the hub; lease expiry could release a live worker that is slow to unwind cancellation. |
| Two-minute sweep with token rotation and takeover of gated runs | Remove | A new hub repairs after exclusive startup. The periodic sweep of ungated untracked runs remains for now. |
| Remote cancellation observation by another service's heartbeat | Remove | Cancellation signal and worker are in the sole hub; the durable database request remains for recovery. |
| PostgreSQL time for gated start, cancellation, terminal time and 30-second freshness | Keep | `NOW()` is available on the required database paths and keeps the interval consistent. Cross-hub clock-skew tests are unnecessary. |
| Atomic terminal status transition and `activity_sync_receipts` | Keep | They prevent duplicate final messages and duplicate job projection on replay or restart. |

Implementation may remove owner and lease code only once the single-instance
lock, fail-stop and immediate repair are effective and tested. Intermediate
states must neither admit two hubs nor free an active scope before its local
worker returns or the old process exits.

## Open review notes for the implementation step

The following points come from [review round 5 on PR #42](https://github.com/florianfinn/docklet-next/pull/42#issuecomment-6175086772)
and are decided here but not yet implemented:

| Note | Implementation and acceptance |
| --- | --- |
| N13 | Add `cancelRequestedAt` to the run snapshot and generated DTO. Progress and metadata updates after cancellation must preserve `messageCode=activity.cancel_requested` and `latestMessage`; individual progress messages remain in history. |
| N14 | Owner write paths `UpdateActivity`, `AppendMessages` and `PatchActivityMetadata` detach the database context inside the service from the cancelled work context while retaining owner identity or the local run reference. The worker can keep logging during unwinding; callers need no `context.WithoutCancel`. |
| N15 | Shorten and correct the `CancelActivity` comment to its current contract. |

## Test matrix for the implementation step

Each row needs an automated test and a targeted counterexample: the named
mutation must make that test fail. Restart tests use two consecutive hub
lifetimes against the same test database, actually ending the first process or
its session. A second service in one process is not a single-instance test.

| Case | Expected result | Mutation that must turn the test red |
| --- | --- | --- |
| 1. New ungated queue-opted activity without a free slot | `queued` and `activity.queued`; no gate scope is occupied. | Save it directly as `running`. |
| 2. New gated operation | `running`, `run.started`, exactly one worker. | Start the worker twice. |
| 3. Ungated slot becomes free | Its `queued` row becomes `running` with `activity.running`. | Omit the status or code update. |
| 4. Same operation meets an active gated run | `run.joined`, same ID, no second worker. | Create a second run. |
| 5. Different operation meets an active gated run | `run.busy`, active ID, no change. | Admit the conflict. |
| 6. Same operation meets a cancelling gated run | `run.cancelling`, including with `Check`, no work. | Return `run.joined`. |
| 7. First cancellation of an ungated `queued` activity | Durable request and one audit message; local waiter is signalled, status remains `queued` until completion. | Omit `cancel_requested_at`. |
| 8. Ungated cancellation races with slot acquisition | The waiting worker starts no operation after cancellation and completes as `cancelled`. | Let it start work despite `cancel_requested_at`. |
| 9. First cancellation of a gated `running` run | Request, audit message, snapshot and local signal; scope stays occupied. | Cancel only the context without a durable request. |
| 10. Repeated cancellation | Timestamp, audit count and status remain unchanged. | Write a second audit message. |
| 11. Progress without cancellation | Step, progress and validated code appear in snapshot and history. | Omit progress code update. |
| 12. Progress after cancellation | Message is stored; `cancelRequestedAt`, snapshot code and cancellation text remain visible. | Replace `latestMessage` with progress. |
| 13. Cancelled worker keeps writing | Service methods accept its cancelled work context. | Tie database access back to `ctx.Done()`. |
| 14. Worker reports success | One `success` completion, `activity.success`, `progress=100` and one final message. | Omit the terminal code. |
| 15. Worker reports failure | One `failed` completion with error text and `activity.failed`. | Store `success` instead. |
| 16. Worker reports success or failure after cancellation | One `cancelled` completion with `activity.cancelled` and no overridden error text. | Ignore cancellation in completion. |
| 17. Gated worker does not return promptly after cancellation | Scope stays occupied until return or process end, even after two minutes. | Sweep a gated run by age. |
| 18. Duplicate or late completion | Exactly one terminal state and final message. | Remove the status condition from `UPDATE`. |
| 19. `Check` within 30 seconds after success | `run.fresh`, same ID, no worker. | Remove the freshness comparison. |
| 20. `Check` at exactly 30 seconds | New run at the exclusive interval boundary. | Use `>=` instead of `>` in the interval predicate. |
| 21. `Check` after failure | New run; only success is fresh. | Count failure as fresh. |
| 22. `Recheck` within the interval | New run if none is active. | Ignore `Recheck`. |
| 23. `Recheck` with an active run | Join, `run.busy` or `run.cancelling` takes precedence. | Check freshness or recheck before active runs. |
| 24. Concurrent admission on one scope | One run ID and at most one worker across competing requests. | Remove the scope transaction lock. |
| 25. Crash during gated `running` | New hub marks run `failed` before API readiness; scope is free afterwards. | Delay repair by two minutes. |
| 26. Crash during ungated `queued` | New hub marks the orphaned row `failed` without executing it. | Restart the waiting operation on startup. |
| 27. Crash during cancellation | New hub marks run `cancelled` with matching code and no duplicate audit message. | Ignore `cancel_requested_at` during repair. |
| 28. Crash before completion commit | New hub writes exactly one repaired completion and final message. | Remove repair from startup. |
| 29. Crash after completion commit but before live event | Terminal run is unchanged; reconnect reloads the snapshot. | Reconnect without a snapshot reload. |
| 30. Job completion resynchronised after restart or pruning | Receipt prevents another projection or message for that version. | Remove the receipt check. |
| 31. Second hub starts while lock is held | No HTTP or jobs; immediate `hub.already_running`. | Skip the session-lock check. |
| 32. Running hub loses its dedicated lock connection | Probe detects loss within two seconds; readiness falls, admission stops, workers are cancelled and process exits within one more second with `hub.db_lock_lost`. | Disable the dedicated-connection probe or its monotonic deadline. |
| 33. Database returns after lock loss | Only a new hub acquires the lock and repairs before API readiness; the old process cannot continue hub writes or admission after fail-stop. | Let the old process reconnect and resume progress writes or admission. |
| 34. Old session remains locked during a network partition | New hub fails immediately with `hub.already_running` and accepts no work. | Wait on `pg_advisory_lock` instead of failing on `pg_try_advisory_lock`. |
| 35. Ordinary pool completion fails while lock connection stays healthy | After three retryable write failures, completion returns an error and the run remains active; same-scope admission stays blocked until retry or restart repair. | Treat the exhausted write as success and admit another gated run. |

## Consequences for #38: HTTP and live adapter

The adapter can use `id`, `status`, `messageCode`, `latestMessage`,
`startedAt`, `endedAt`, `affectedServices` and the new
`cancelRequestedAt` in the snapshot. A gated active run with
`cancelRequestedAt` remains `running`; an ungated queued activity can remain
`queued`. The timestamp is the authoritative cancellation indicator.
`run.started`, `run.joined` and `run.fresh` are admission results.
`run.busy` and `run.cancelling` are coded conflicts with the active run ID.
Only `run.started` contains an executable work context; the HTTP adapter
never exposes it to the client.

The live channel publishes activity snapshots at start, progress,
cancellation request and terminal completion, plus messages for cancellation
and completion. A live event announces a changed snapshot; it is not a
delivery guarantee. After reconnect or detected event loss, the adapter
reloads affected runs and topics. This also reveals a completion whose
database commit preceded a crash but whose event was never sent. The adapter
does not interpret `cancelled` as proof that an agent reversed an accepted
external action. `hub.already_running` and `hub.db_lock_lost` appear in
startup or exit diagnostics only, never as #38 action response codes. The #38
adapter can observe failed readiness after lock loss, but does not receive
either operational code.
