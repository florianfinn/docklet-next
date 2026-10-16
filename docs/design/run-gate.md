# Run gate and single-hub operation

## Scope and guarantees

The private deployment operates one hub process in one hub container against
SQLite on local persistent storage. A second hub must not start while the first
holds its lifetime file lock. The [SQLite contract](sqlite-database.md) defines
the runtime transition, file/connection policy and schema. Agents are separate
processes; this decision does not limit their number.
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
`success`, `failed` and `cancelled` are terminal. Cancellation is a local
context signal; it is not persisted. There is no automatic operation replay or
atomicity guarantee for external effects. The guarantees cover unique gated
admission and one durable terminal transition per activity row.

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
| `running` → cancellation requested | Hub signals the local worker; status and scope remain active. | None | Existing snapshot code remains. |
| `running` → `running` | Worker reports progress, including while cancellation unwinds. | `progress`, `step`, `updated_at` and optional messages | Validated progress code, default `activity.message`. |
| `running` → `success` | Worker reports success before cancellation. | Terminal fields, `progress=100`, optional final message | `activity.success` |
| `running` → `failed` | Worker reports failure before cancellation. | Terminal fields, `error`, optional final message | `activity.failed` |
| `running` after local cancellation → `cancelled` | Worker returns, whether it reports success or failure. | Terminal fields and final message; no error from the overridden result | `activity.cancelled` |
| `running` → `failed` | Startup repair finds an abandoned run, including one cancelled before a crash. | Terminal fields, interruption reason and final message | `activity.failed` |
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
| `queued` → `running` | Local worker acquires a slot without local cancellation. | `status`, `updated_at`, `message_code` | `activity.running` |
| `queued`/`running` → cancellation requested | Hub signals the local worker; status stays active. | None | Existing snapshot code remains. |
| `queued` after local cancellation → `cancelled` | Waiting worker stops without beginning the operation. | Terminal fields and final message | `activity.cancelled` |
| `queued`/`running` → `success`/`failed`/`cancelled` | Worker returns, or startup repair closes abandoned work. | Terminal fields and at most one final message | Matching `activity.<status>` |

A queued worker whose context was cancelled must not begin the operation;
its caller completes the row as `cancelled`. If slot acquisition races with
cancellation, the worker checks its context before doing work. Ungated rows
use the same terminal status condition and one-final-message rule as gated
rows. Startup repair also closes abandoned ungated queued and running work
before accepting new work. The existing two-minute sweep for ungated
untracked work remains for now; startup repair could replace it in a separate
follow-up.

A job-run snapshot is a projection of its durable coordinator. An explicit
retry may reopen it after a terminal state; that is a new coordinator state,
not a reopened gated run. Its `job.*` codes and
`activity_sync_receipts` follow the coordinator rather than the gated
transition table.

## Admission and gate

Admission uses one short `BEGIN IMMEDIATE` transaction to read active gated
runs and create at most one new run. A partial unique index on active
`(environment_id, gate_scope)` pairs protects the invariant durably. The local
running/cancelling mutex covers commit and worker registration. The lifetime
file lock prevents a second hub; it does not replace atomic scope admission.

| Finding at admission | Result | Database change |
| --- | --- | --- |
| Active run with the same `gate_key` and no local cancellation | `run.joined` with its ID; only the original caller owns a worker. | None |
| Active run with a different `gate_key` | `run.busy` conflict with the active run ID. | None |
| Active run with the same `gate_key` and local cancellation in progress | `run.cancelling` conflict with the active run ID, even for `Check`. | None |
| No active run; `Check=true` and `Recheck=false`; last successful run for this operation and scope ended less than 30 seconds ago | `run.fresh` with that ID. | None |
| Otherwise | `run.started` and a new run ID; only this caller starts work. | New `running` run |

The active-run check always precedes freshness. `Recheck=true` bypasses only
the 30-second freshness result, never an active run. Only success is fresh;
the interval compares integer UTC microseconds using one bound hub clock
sample. At exactly 30 seconds the previous run is no longer fresh. After expiry
or `Recheck`, the worker reads resource state again before acting. Joining, rejecting and
returning a fresh result start no worker and do not change the existing row's
message code.

## Cancellation

The hub signals the registered local worker with a cancelled context. A second
request has the same effect. There is no cancellation timestamp, audit message
or snapshot write. An untracked active activity is completed as `cancelled`
directly. The worker finishes a tracked activity as `cancelled` when it returns.

While cancellation unwinds, the status and gated scope remain active. The hub
keeps an in-memory cancelling flag for admission; the same operation receives
`run.cancelling`, including with `Check`. Progress and metadata may still be
written with a bounded database context after the work context is cancelled.
Only terminal completion releases the scope. A worker that never returns holds
it until the hub exits. A cancelled agent call may already have caused an
external effect; `cancelled` does not promise a rollback.

## Restart, crash and one-time completion

After acquiring the single-instance lock and before accepting work, a new hub
repairs gated `running` runs left by the prior process. A local cancellation
signal is lost at a crash, so repair marks these runs `failed`, with an
interruption message and terminal timestamp. Repair also covers crashes during
cancellation and abandoned ungated work. Special coordinators, including job
runs and an occupied self-update completion, reconcile their own durable state.

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
history. Keep the receipt until the coordinator run has demonstrably left. A recent
`synced_at` protects a receipt synchronized after the coordinator retention
snapshot but before receipt pruning.
These rules do not guarantee exactly one external agent action; its actual
outcome must be checked after a crash.

If a normal database write encounters transient contention while the lifetime
file lock remains held, completion retries its transaction up to three times
within the existing 10-second owner-write deadline; exhausted SQLite busy errors
use the existing retry classifier, with no additional retry layer. After
the final failure it returns the error; it must not
claim success or free the durable scope. The row remains active and blocks
another gated admission on that scope. An operator restart triggers startup
repair and releases the scope.

## Single-instance lock

At startup, before opening/migrating either database, repair, background jobs
and HTTP readiness, the hub acquires `flock(LOCK_EX | LOCK_NB)` on Linux, or an
equivalent descriptor-owned lock on another supported target, on the resolved
main database path plus `.lock`. It holds the descriptor through the
whole process lifetime. If held, startup fails immediately with
`hub.already_running`, before HTTP or jobs and without waiting for release.
This is a startup error, not a run message code.

Controlled shutdown stops work and closes every database handle before unlocking.
A crash releases the OS lock; a new hub recovers SQLite and repairs abandoned
runs before accepting work. Never unlink/replace the lock inode or use network
storage. Opening and closing a second descriptor of the lock file must not
release the held lock. Never open live database/WAL/SHM files through ordinary
descriptors; use `Lstat`/`Stat` for path checks. The
[SQLite contract](sqlite-database.md#single-hub-and-run-admission) also covers
path aliases, excluded VFS options and administrative writers, including offline
`admin reset-password`. Terminal status conditions
prevent duplicate completion after repair, while an already accepted external
agent action can continue beyond the old hub's lifetime.

## Reconciliation with the multi-process mechanism from PR #42

The decision in [#43](https://github.com/florianfinn/docklet-next/issues/43)
changes the operating assumption of
[PR #42](https://github.com/florianfinn/docklet-next/pull/42). Unreleased
feature migrations can be simplified before the first release; shipped
migrations remain immutable.

| Component | Decision | Reason and target state |
| --- | --- | --- |
| Atomic run-scope admission | Keep with SQLite | An immediate transaction plus active-scope uniqueness serializes admission through commit in the one hub. |
| Durable cancellation request, audit message and immediate live snapshot | Remove | The sole hub cancels its registered local worker; restart repair fails abandoned work. |
| Owner token `gate_owner` and fencing of progress and completion writers | Remove | The private single-operator deployment accepts the loss of writer fencing; terminal status conditions still guard against duplicate completion. External effects retain the stated overlap risk. |
| `gate_lease_at` and 30-second heartbeat | Remove | The startup lock admits one hub while held; lease expiry could release a live worker that is slow to unwind cancellation. |
| Two-minute sweep with token rotation and takeover of gated runs | Remove | A new hub repairs after exclusive startup. The periodic sweep of ungated untracked runs remains for now. |
| Remote cancellation observation by another service's heartbeat | Remove | Cancellation signal and worker are in the sole hub; restart repair handles abandoned work. |
| One clock for gated start, terminal time and 30-second freshness | Keep with SQLite | Bind UTC Unix microseconds from the single hub clock; use the same sample per transaction. Cross-hub clock-skew tests are unnecessary. |
| Atomic terminal status transition and `activity_sync_receipts` | Keep | They prevent duplicate final messages and duplicate job projection on replay or restart. |
| Receipt `synced_at` grace | Keep | It protects a newly synced receipt from pruning against an older coordinator snapshot. |
| Configurable event and activity count limits | Remove | Cleanup keeps 500 events and 100 finished activities per environment. |
| Job run age retention | Keep | Finished job history is limited to 100 runs and seven days. |

During normal operation, a scope stays occupied until its local worker
returns or the hub process exits.

## Review notes for the implementation step

The following points come from [review round 5 on PR #42](https://github.com/florianfinn/docklet-next/pull/42#issuecomment-6175086772):

| Note | Implementation and acceptance |
| --- | --- |
| N14 | Owner write paths `UpdateActivity`, `AppendMessages` and `PatchActivityMetadata` detach the database context inside the service from the cancelled work context while retaining owner identity or the local run reference. The worker can keep logging during unwinding; callers need no `context.WithoutCancel`. |
| N15 | Shorten and correct the `CancelActivity` comment to its current contract. |

## Test matrix for the implementation step

Each row needs an automated test and a targeted counterexample: the named
mutation must make that test fail. Restart tests use two consecutive hub
lifetimes against the same temporary SQLite files, actually ending the first
process and releasing its file descriptor. A second service in one process is
not a single-instance test.

| Case | Expected result | Mutation that must turn the test red |
| --- | --- | --- |
| 1. New ungated queue-opted activity without a free slot | `queued` and `activity.queued`; no gate scope is occupied. | Save it directly as `running`. |
| 2. New gated operation | `running`, `run.started`, exactly one worker. | Start the worker twice. |
| 3. Ungated slot becomes free | Its `queued` row becomes `running` with `activity.running`. | Omit the status or code update. |
| 4. Same operation meets an active gated run | `run.joined`, same ID, no second worker. | Create a second run. |
| 5. Different operation meets an active gated run | `run.busy`, active ID, no change. | Admit the conflict. |
| 6. Same operation meets a cancelling gated run | `run.cancelling`, including with `Check`, no work. | Return `run.joined`. |
| 7. First cancellation of an ungated `queued` activity | Local waiter is signalled; status remains `queued` until completion; no cancellation message is written. | Do not cancel the registered context. |
| 8. Ungated cancellation races with slot acquisition | The waiting worker starts no operation after cancellation and completes as `cancelled`. | Skip the local cancel signal before slot acquisition. |
| 9. First cancellation of a gated `running` run | Local context is cancelled; persisted snapshot stays `running`; scope stays occupied. | Release the scope on cancellation. |
| 10. Repeated cancellation | Status remains active and no cancellation message is written. | Finalize the run on the second request. |
| 11. Progress without cancellation | Step, progress and validated code appear in snapshot and history. | Omit progress code update. |
| 12. Progress after cancellation | Message and progress remain writable; latest progress appears in the snapshot. | Discard the latest message batch's progress write. |
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
| 24. Concurrent admission on one scope | One run ID and at most one worker across competing requests. | Split active lookup/insert across transactions and remove active-scope uniqueness. |
| 25. Crash during gated `running` | New hub marks run `failed` before API readiness; scope is free afterwards. | Delay repair by two minutes. |
| 26. Crash during ungated `queued` | New hub marks the orphaned row `failed` without executing it. | Restart the waiting operation on startup. |
| 27. Crash during cancellation | New hub marks abandoned run `failed` with one final message. | Mark abandoned runs `cancelled` instead. |
| 28. Crash before completion commit | New hub writes exactly one repaired completion and final message. | Remove repair from startup. |
| 29. Crash after completion commit but before live event | Terminal run is unchanged; reconnect reloads the snapshot. | Reconnect without a snapshot reload. |
| 30. Job completion resynchronised after restart or pruning | A new receipt survives a stale coordinator retention snapshot; replay after activity pruning adds no projection. | Remove the `synced_at` grace or receipt check. |
| 31. Second hub starts while lock is held | No HTTP or jobs; immediate `hub.already_running`. | Skip the file-lock check. |
| 34. First hub crashes while holding the file lock | Its OS lock releases; a new process starts and repairs abandoned runs before readiness. | Treat a leftover lock filename as a live owner instead of acquiring the OS lock. |
| 35. Ordinary pool completion fails while the file lock stays held | After three retryable write failures, completion returns an error and the run remains active; same-scope admission stays blocked until restart repair. | Treat the exhausted write as success and admit another gated run. |

## Consequences for #38: HTTP and live adapter

The adapter can use `id`, `status`, `messageCode`, `latestMessage`,
`startedAt`, `endedAt` and `affectedServices` in the snapshot. A cancelling
active run remains `running` until worker completion. Cancellation is not
recoverable from a snapshot after a crash.
`run.started`, `run.joined` and `run.fresh` are admission results.
`run.busy` and `run.cancelling` are coded conflicts with the active run ID.
Only `run.started` contains an executable work context; the HTTP adapter
never exposes it to the client.

The live channel publishes activity snapshots at start, progress and
terminal completion, plus completion messages. A live event announces a
changed snapshot; it is not a delivery guarantee. After reconnect or detected
event loss, the adapter reloads affected runs and topics. This also reveals a completion whose
database commit preceded a crash but whose event was never sent. The adapter
does not interpret `cancelled` as proof that an agent reversed an accepted
external action. `hub.already_running` appears in startup diagnostics only,
never as a #38 action response code.
