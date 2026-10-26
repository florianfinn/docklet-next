# Hub-owned stack update policies

This refines the accepted [images and updates contract](images-updates.md) for
policy scheduling and notifications. The hub owns policy, schedule and notification
state. Executors retain the existing image update/restore runtime and its safety
checks. Standalone containers receive observations only.

## Policy trigger state table

| State / event | Admission and durable result |
| --- | --- |
| Policy missing or `off`; check due | Skip policy-triggered check, notification and apply. Manual checks and lifecycle actions keep their existing rights. |
| Polling or apply master gate disabled | The corresponding hub job is disabled. An explicit trigger records a skipped outcome without executor effects. |
| `notify` or `apply`; check due | Revalidate recorded actor/key, host registration and target identity; execute a bounded fresh check through the current Direct/Edge transport. |
| Fresh candidate found | Persist candidate status. For notify, create one candidate entry/outbox link in the same transaction as its unique dedup identity. Apply uses the existing stack runtime operation. |
| Saved inputs have undeployed drift | Auto-apply skips with `update.config-drift`; no snapshot, pull, file write or deploy. |
| Baseline missing or unprovable | Auto-apply refuses visibly with `update.baseline-unavailable`; a successful normal deploy must establish proof. |
| Runtime target or host image lease busy | Record a visible typed skip; no spin or automatic retry of the effect. The next ordinary schedule is a new trigger. |
| Hub restarts with a child checkpoint | Reconcile the same runtime child receipt. Never dispatch another effect or resume a destructive phase from job metadata. |
| Actor/key loses permission before child admission | Refuse the new child with a neutral permission outcome. Revalidation happens for every child, never through a system actor. |
| Policy revision changes before admission | Refuse stale intake with `update.policy-changed`. |
| Policy revision changes after admission | The admitted revision remains frozen in the child intent. Later edits govern later admissions; explicit cancellation uses existing durable runtime controls. |
| Notification already recorded; check/restart/policy toggle | Reuse the existing candidate entry. A changed immutable candidate identity may create one new entry. Delivery failure never creates a replacement entry. |
| Agent-local inherited job/watcher fires | Refuse/skip with a typed hub-ownership reason, without independent checks or workload effects. |

## Ownership and effects

Each registered host has two hub projections: `image-polling` and `auto-update`.
The existing scheduler coordinator owns occurrences and run history; the same
six-field cron parser and hub IANA zone drive execution and next-run projection.
Stored actor/key attribution is revalidated at fire time and before every child.
For remote hosts the registration ID is the environment ID; deleting and
recreating an environment gives it a new binding. Agent replacement within an
existing environment is fenced by target identity and executor generation. The
local host binds its persistent instance ID. Missed occurrences do not create a reconnect catch-up burst.

Apply children are serial within a host. Before admitting a child, persist its
identity and frozen policy revision; once admitted, existing runtime admission,
parent exclusion, snapshot/pins, literal edits and post-deploy verification remain
the only effect path. Recovery follows recorded child evidence and cannot select
a newer candidate under the old run identity.

Notification identity includes host registration, stack identity, repository,
platform, release/tag and immutable candidate digest. A transaction commits that
identity with its durable entry before existing channels are dispatched. Badge
data remains visible without channels; an unconfigured or failed delivery is
reported truthfully. A durable `sending` claim means delivery is in flight or its
outcome is unknown after interruption; observers never label it failed or resend
it. Sent entry history remains visible after switching from notify to apply.
This guarantees one entry, not exactly-once network delivery.

System hints are explicit reads of the effective agent/tools references. The hub
selects only an observed stable release at or below its running version and never
proposes a downgrade. Unknown references, development versions and missing
published release evidence remain unknown. Tools overrides remain pinned; this
binary only asserts its own release's tools mapping, so older-agent compatibility
stays unknown. Hints cannot pull or replace containers. The existing release-feed
source limitation is tracked separately in issue #235.

Queued wrappers from an earlier hub process only reconcile receipts. An active or
unknown admitted child also holds later batches for that host. An undeclared
executor result code becomes `runtime.outcome-unknown` and requires receipt
retry/reconciliation without replaying update effects; another host can
continue. A known terminal receipt clears the checkpoint. An administrator's
acknowledged runtime release clears it in the same transaction, without changing
unknown effect knowledge into a successful result. Public version responses and
version streams omit system image references; authenticated hub reads retain the
metadata used by the protected hint endpoint.

A host's default cron values are readable before configuration. Its jobs stay
disabled until a schedule save records a current actor/key; no system actor is
installed as default authority. Policy edits do not silently replace that schedule
owner. The dependent UI must configure the host schedules as well as stack modes.

Policy `code` and `checkedAt` describe checks only; `stale` depends on that check
state. Apply attempts have separate `applyCode` and `applyCheckedAt` fields, so a
poll cannot erase a drift refusal and an apply job cannot stale a notify check.
Manual checks return their successful observations even if optional policy
bookkeeping fails. A separate `policyCode` on the response and persisted
`bookkeepingCode`/`bookkeepingAt` report that failure without changing check state;
if persistence itself fails, the response still reports the bookkeeping reason.

Off and notify policies require project read, image-update read and check rights.
Apply additionally requires jobs manage, project update/deploy, image pull and
activity read. Schedule owners independently require jobs manage at fire time.
Permission refusals consistently fail the affected run/target with
`update.permission-denied`; an unconfigured schedule reports
`update.schedule-owner-unset` and remains disabled.

## Scoped update capabilities and restore-point protection (#229 addendum)

This additive design closes DG-01/DG-03 for the dependent update UI. Guard citations
refer to base `5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8`; paths below are relative
to `backend/internal/`. M2 still requires a point for every update/restore and a
baseline before update; M5 is stacks only, M6 protects restore targets and the
healthy baseline with one failed slot, and D2 freezes policy revision at admission.

### Descriptor contract

Descriptor IDs are adapter camelCase IDs, distinct from HTTP operation IDs.
`S` means `kind=stack,id=E~P`; `H` means `kind=host,id=E`. `E` is the actual
environment ID (`0` locally), `P` a nonempty raw project ID; reject extra `~`,
empty components and mismatched kinds. Every grant below uses `Allows(grant,E)`.
No hub-wide `AllowsAny` fallback or grants combined across environments apply.
Point-specific calls use the owning stack descriptor; host-admin calls use H,
including orphan points. Descriptors gate operations, not individual point holds.

Grant abbreviations (conjunction, never alternatives): `R` = `projects:read` AND
`image-updates:read`; `W` = `projects:deploy` AND `projects:update` AND
`images:pull` AND `image-updates:check` AND `activities:read`; `N` = R AND
`image-updates:check`; `A` = N AND W AND `jobs:manage`. `G` = `IsGlobalAdmin()`.

| Descriptor ID | Ref | HTTP operation ID / exact grant conjunction and guard source |
| --- | --- | --- |
| `checkUpdates` | S | `check-project-updates`: `image-updates:check` only; `updater/module.go:70`, `updater/handler.go:94`, `updater/policy_execution.go:443`. Policy bookkeeping revalidates its recorded actor separately. |
| `installStackUpdates` | S | `update-project-images`: W; `activity/runtime_image_routes.go:38`, `activity/runtime_http_service.go:87`. |
| `restoreRestorePoint` | S | `restore-update`: W; `activity/runtime_image_routes.go:45`, `activity/runtime_http_service.go:87`. |
| `getUpdateBaseline` (new) | S | `get-update-baseline`: R; `updater/lifecycle_routes.go:110`, `updater/lifecycle_routes.go:47`. |
| `listRestorePoints` (new) | S | `list-restore-points`: R; `updater/lifecycle_routes.go:92`, `updater/lifecycle_routes.go:47`. |
| `deleteRestorePoint` (new) | S | `delete-restore-point`: W; `updater/lifecycle_routes.go:98`, `updater/lifecycle_routes.go:49`. |
| `listHostRestorePoints` (new) | H | `list-host-restore-points`: G AND `image-updates:read`; `updater/lifecycle_routes.go:104`, `updater/lifecycle_routes.go:51`. |
| `deleteHostRestorePoint` (new) | H | `delete-host-restore-point`: G AND `image-updates:read` (host branch replaces W); `updater/lifecycle_routes.go:107`, `updater/lifecycle_routes.go:51`. |
| `getAutoUpdatePolicy` (new) | S | `get-auto-update-policy`: R; `updater/policy_routes.go:32`, `updater/policy_routes.go:45`. |
| `setAutoUpdate` | S | `set-auto-update-policy`, modes `off`/`notify`: N; `updater/policy_routes.go:32`, `updater/policy_routes.go:52`, `updater/policy_service.go:349`. |
| `setAutoUpdateApply` (new) | S | `set-auto-update-policy`, mode `apply`: A; same guards. The UI must select this descriptor for apply; the off/notify descriptor cannot authorize it. |
| `listSchedule` | H | `get-update-schedules`: `jobs:manage`; `job/image_policy_routes.go:30`. Covers the two M10 schedules only. |
| `saveUpdateSchedule` (new) | H | `set-update-schedules`: `jobs:manage`; `job/image_policy_routes.go:34`, actor revalidation at `updater/policy_service.go:250`. |
| `listSystemComponents` | H | `get-system-image-hints`: `image-updates:read`; `updater/system_hints.go:124`. This is the protected agent/tools version-hint read, with no rollout action. |

Use exactly the route's full grant conjunction, including nested admission guards;
never a weaker set. `capability.unavailable` covers invalid/scoped-denied refs.
Container update/apply and host-wide or unscoped check requests stay unavailable.
Availability describes discovery; every call independently rechecks
current rights, target identity/revision, point ownership/holds and runtime gates.
The two policy descriptors preserve mode-specific rights without requiring apply
authority to disable a policy. No route permission is changed by this addendum.

Descriptor IDs/rights are identical for hub-local, Direct and Edge host refs.
Capabilities are read from the hub for the caller's E, never from agent admin
credentials. Existing executor commands are `updater.project.check`,
`updater.project.baseline`, `updater.restore.{list,delete,host-list,host-delete}`
(`backend/pkg/libarcane/edge/commands.go:18`). Update/restore admission and policy,
schedule and hint reads remain hub-owned; executor work uses runtime commands.
No new agent route or Edge fallback is introduced.

### Authoritative point projection

Add required `RestorePoint.protectionReason: RestorePointProtectionReason`, enum
`none|restore-target|healthy-baseline|failed-slot`, and required
`RestorePoint.holdState: RestorePointHoldState`, enum `none|active|unknown`, in
`types/updater/`. Declare constants and schema enum tags; generate OpenAPI/DTO,
agent v7 fingerprint/fixture and provenance through `pnpm run contract:generate`.
Use EN/DE keys `updates.restorePoints.protection.{none,restore-target,healthy-baseline,failed-slot}`
and `updates.restorePoints.hold.{none,active,unknown}` for every enum value.

Extract one pure retention plan in `project/children/lifecycle/retention.go`;
both `RetentionDeletions` and point-list projection consume its decisions. Inputs
are the same available points, newest first (`created_at DESC,id DESC`), grouped
by target ID exactly as `project/lifecycle_pins.go:281`, plus shared hold states.
Host lists evaluate each target separately; unavailable points have reason none
but retain their hold state. Do not classify from rendered list order or history.
The plan preserves existing M6 selection: latest settled baseline, latest
`LastRestoredAt` target, one failed partial-source slot, then oldest-other eviction.
One field uses precedence restore-target, healthy-baseline, failed-slot, none;
the planner retains all overlapping internal anchors. A failed-slot label means
the retained replacement slot, not exemption from the five-point count limit.
Holds always override eviction; limits may be exceeded when anchors/holds require.

Replace the store's boolean hold calculation with one shared state query, keeping
`Held` as `state != none` for retention and transactional deletion. Inspect both
unsettled lifecycle owners and selected-point intents/receipts currently read at
`project/children/lifecycle/store.go:247`. Active means positively known live
ownership; unknown means uncertain receipt outcome or missing/inconsistent owner
evidence. Unknown dominates active when owners overlap; neither permits deletion.
Database read failures fail the list/retention/delete operation, never imply none.
Restart and pruned activity history cannot clear durable ownership. No migration.
Retention reasons explain automatic eviction only; explicit deletion may remove
an unheld anchor and still rechecks holds atomically (`store.go:263`).
Agent-derived enum values must be runtime-validated before trusted consumption;
missing/undeclared values cannot become none or enable delete. The dependent UI
owns its HTTP decoder; it consumes generated DTOs and these declared enums.

### Break map, ownership and verification

| Break boundary | Planned test | Red mutation (assertion failure required) |
| --- | --- | --- |
| Scoped deny per host/environment | `TestUpdateCapabilitiesScopedGrants`: every descriptor/grant removed in turn, cross-host split grants, explicit denies, malformed refs and non-admin host lists/deletes; off/notify versus apply. | Replace E with another allowed environment, omit each conjunct, or drop G; affected descriptor incorrectly becomes available. |
| Grant removed after capability read | `TestUpdateCapabilityReadThenRevokedCall`: read available, revoke each route grant, invoke the real registered read/write route, assert 403 and zero executor/storage effects. | Bypass the corresponding route/admission grant check; call reaches an effect/read spy. |
| Direct/Edge descriptor and agent parity | `TestUpdateCapabilitiesTransportParity`: identical IDs/results for local/Direct/Edge refs; exercise existing baseline/point commands through actual transport, hub-only routes never forwarded. | Remove one descriptor/Edge mapping or bypass one agent guard; ID/result/dispatch assertion fails. |
| Enum contract and translations | `TestRestorePointProjectionContract`: Go enum sets equal OpenAPI, generated DTO and v7 fixture/fingerprint; all EN/DE keys present with equal placeholders; malformed/missing wire enums rejected at actual consumer. | Remove one schema enum, fixture entry or catalogue key, or accept unknown wire values; targeted assertion/contract drift check fails. |
| Projection equals M6 retention | `TestRestorePointProjectionRetentionParity`: oldest restore target, baseline beyond five held points, one failed slot, five failed restores, failed target owns slot, active/unknown holds, overlaps, separate targets, unavailable points and restart. Assert plan reasons, holds, exact deletion IDs and explicit-delete semantics. | Remove each anchor/hold or retain all failed attempts; reason/deletion/effect assertions fail together. |

Phase 2 owns `activity/capabilities*.go`, `project/lifecycle_{points,pins}*.go`,
`project/children/lifecycle/{retention,store}*.go`, `types/updater/` enums/tests,
generated contract/DTO/fingerprint/fixture/provenance, and EN/DE message keys only.
New cross-route tests live in `activity/capabilities*_test.go`; existing updater,
job, middleware and Edge code/tests are read-only references. Other web source
belongs to #231; migrations stay untouched. Run focused Go/contract/catalogue
checks and behavioural mutations, then full root lint/test under the load rule.
Phase 1 changes this rationale only and uses the repository's text-only checks.
