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

This closes DG-01/DG-03. Guard citations use base `5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8`;
Unqualified backend paths use `backend/internal/`; `types/` paths are always repo-root paths. M2 requires points for update/restore and a baseline before update.
M5 is stacks only; M6 protects restore targets/baseline with one failed slot; D2 freezes policy revision at admission.

### Descriptor contract

Descriptor IDs are adapter camelCase IDs, distinct from HTTP operation IDs.
`S` means `kind=stack,id=E~P`; `H` means `kind=host,id=E`. E is the environment ID (`0` locally),
P a nonempty raw project ID; reject extra `~`, empty components and mismatched kinds. Grants use `Allows(grant,E)`.
No hub-wide `AllowsAny` fallback or grants combined across environments apply.
Point calls use the owning stack descriptor; host-admin calls use H, including orphan points. Holds are checked separately.

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

Use the full route/nested grant conjunction, never weaker; invalid/scoped-denied refs use `capability.unavailable`.
Container update/apply and host-wide/unscoped checks stay unavailable; calls recheck current rights and identity.
Remote baseline/point guards are `middleware/environment.go:186` through :203, plus matcher :204/:325; agents use sudo (:180).
IDs/grants match local/Direct/Edge; capabilities come from the hub for caller E. Policy descriptors preserve off/notify rights.
Existing commands: `updater.project.{check,baseline}`, `updater.restore.{list,delete,host-list,host-delete}` (`backend/pkg/libarcane/edge/commands.go:18`).
Admission/policy/schedules/hints stay hub-owned (`middleware/environment.go:556`); no new agent route, command, fallback or route grant.

### Point projection, evidence and safety

`RestorePointProtectionReason` contains only `restore-target|healthy-baseline`; required nullable `protectionReason` uses explicit null for no anchor.
Required `holdState: RestorePointHoldState` has enum `none|active|unknown`; required `failedAttempt: bool` is a neutral kind, never a protection by itself.
One pure planner in `project/children/lifecycle/retention.go` supplies retention deletion IDs and projection reasons from stored `available = true` rows,
per target in `created_at DESC,id DESC` order (`project/lifecycle_pins.go:281`). Display `available`/`code` are independent: registration mismatch or
failed snapshot verification never filters planner inputs or erases reasons. Host lists plan each target separately. M6 keeps latest settled baseline,
latest `LastRestoredAt` target and one failed slot, then evicts oldest others. Reason precedence is restore-target then healthy-baseline; anchors may overlap.
Holds override eviction, including above five; unheld anchors remain explicitly deletable. Failed slots remain subject to replacement/count eviction.

Executor active evidence is `image_lifecycle_runs.settled = false` with matching `point_id` or `selected_point_id` (`project/children/lifecycle/store.go:247`).
Hub evidence joins `runtime_image_intents.run_id` to `runtime_receipts.run_id`: payload `request.pointId`, receipt `environment_id`, `kind = receipt`,
`settled_at IS NULL`. Do not filter by current registration: old-registration non-terminal/unknown receipts selecting the same point UUID still hold.
Executor states pending/accepted/executing mean hub active; unknown with non-null `unknown_reason` means uncertain hub outcome. Dispatch
pending/dispatching/accepted all hold; pending with null `dispatched_at` already holds. Unknown dominates active, then none when neither holds.
Current declarations: `backend/resources/migrations/sqlite/007_image_lifecycle.sql:10` and :25. Executor holds survive hub settlement/release and restart.
The hub overlays successful forwarded target/host lists for Direct and Edge: executor active OR hub active/unknown. This holdState is display only.
Before forwarding either delete, an advisory, lock-free hub read refuses a visible non-terminal hold with declared `restore.in-use` (409).
No admission/delete fence or new lock is added. Never hold `runningMu` or a DB transaction around remote calls; the advisory check may race.
Inject an evidence-read port via `middleware/environment.go:842` (`EnvironmentProxySecurity`), wired at `bootstrap/router_bootstrap.go:309`; middleware never imports activity.
Bound agent point-list bodies to 8 MiB before buffering (Direct reads and Edge assembly), and to 4,096 points. Validate the list envelope and required fields.
Malformed/non-list/oversized/missing/undeclared agent values fail the whole list with declared fallback `update.check-unavailable` (503), never passthrough/defaults.
Declared typed agent errors preserve their code and HTTP status; malformed/undeclared errors use the fallback.
Direct/Edge transport or tunnel failures use existing `agent.unavailable` (502); oversized evidence still uses the validation fallback.
Hub evidence DB failures use `update.persistence-failed` (500), including deletes before forwarding. These existing codes are in `types/image/reason.go:18`.

Safety invariant: a restore never acts on a deleted or unavailable point. Only executor conditional selection plus executor `Held` enforce this invariant.
`project/children/lifecycle/store.go:189` through :191 checks id, target, identity, registration and available=true; RowsAffected must equal one, before effects.
Selection and creation of the executor run/hold share a transaction; delete rechecks `Held` transactionally (:264). Preserve `restore.unavailable` (409)
through Prepare's outer error mapping (:209) and runtime result projection. Hub pre-check/overlay are advisory, never authority for proceeding with effects.

| Order: restore R on point P versus delete D | Executor result / data | Required outcome |
| --- | --- | --- |
| D before R admission | P deleted; no restore effects | R refused at admission or executor select with restore.unavailable. |
| R admitted, hub intent committed, not dispatched; D | Hub pre-check sees intent; P unchanged | D refused with restore.in-use, zero forwarded deletes. |
| R admitted; pre-check misses, or unknown-outcome D lands late | Executor deletes P before R selects it | Dispatch R; conditional selection refuses with restore.unavailable, zero effects; run settles refused. |
| R running on executor; D | Executor Held refuses D; P unchanged | D refused with restore.in-use. |
| D times out at hub | D may have landed or may land later | UI shows delete outcome unknown and re-reads list; no restore effect on a deleted point. |

`TestPointDeleteOrders` runs all five orders with deferred barriers on hub-local and executor fakes through actual Direct and Edge transports.
Mutations: M1 remove conditional selection/RowsAffected check (late-delete safety red); M2 remove hub pre-check (visible-intent refusal red, safety green);
M3 remove executor Held check (running-delete refusal red). Assert errors/effect counts/settlement; never count compiler errors or timeouts as red proof.

Go constants/schema enum tags own the vocabulary; `scripts/agent-contract.mjs:65` derives retentionProtections/holdStates through OpenAPI, removing old aliases.
failedAttemptPoints/failedAttemptScope describe replacement only. `pnpm run contract:generate` updates OpenAPI/DTO/v7 fingerprint/fixture/provenance;
drift checks enforce enum equality, nullable reason, and required protectionReason/holdState/failedAttempt in Go, wire, DTO and fixture.
#229 owns `web/src/shared/messages/{en,de}.ts` subtree `common.restorePoints`: `protection.{restore-target,healthy-baseline}`, `noRetentionAnchor`,
`hold.{none,active,unknown}`, `kind.{point,failedAttempt}`; #231 treats it read-only. Render hold label first when active/unknown; null anchor never says unprotected.
Existing `images.errors.restore_in-use`, `images.errors.update_check-unavailable` and `images.errors.update_persistence-failed` cover refusals/fallbacks.
#231's real HTTP decoder also rejects missing/malformed fields and unknown enums; list load fails, no delete writes, no invented null/none.

### Break map and ownership

| Boundary | Planned test | Red mutation (assertion failure required) |
| --- | --- | --- |
| Scoped rights/revocation | `TestUpdateCapabilitiesScopedGrants` and `TestUpdateCapabilityReadThenRevokedCall`: each grant, host/admin scope, split grants, refs/modes, local/remote E; real calls return 403 with zero forbidden reads/effects. | Omit conjunct/admin check; change E; bypass hub proxy conjunction/matcher. |
| Descriptor/transport parity | `TestUpdateCapabilitiesTransportParity`: identical IDs/grants locally/Direct/Edge; actual baseline/point commands; hub-only routes never forwarded. | Remove descriptor/command mapping or bypass proxy grant. |
| Advisory overlay and delete orders | `TestDirectPointHoldOverlay`, `TestEdgePointHoldOverlay` and `TestPointDeleteOrders`: target/host lists and both deletes, active/unknown, all five orders above. | Skip overlay; apply M1/M2/M3 independently with their distinct outcomes above. |
| Hub ingress | `TestPointOverlayValidation` for Direct/Edge: malformed envelope, non-list, missing/invalid fields, undeclared enums, exactly/over 8 MiB or 4,096 points, DB failure; typed 503/500, no partial output or forwarded delete. | Pass through/default bad values; remove byte/count bound or DB-error refusal. |
| Re-registration | `TestPointOverlayRegistrationChange`: old-registration unknown receipt selects same point after re-registration; hold stays visible and advisory delete refuses. | Filter by current registration only. |
| Contract/catalogues | `TestRestorePointProjectionContract`: Go/OpenAPI/DTO/manifest/v7 fixture/fingerprint enum equality, reason nullability and all three fields required; complete EN/DE keys/placeholders. | Restore alias/failed-slot; omit required fields or add omitempty, including failedAttempt; drop key. |
| Web consumer | #231 `point-shapes`: validate real decoder; valid null, invalid/missing enums/failedAttempt; null reason with active/unknown shows hold label; unknown delete reconciles list. | Default invalid values, skip validation, show unprotected for null+hold or turn unknown delete into success. |
| M6/concurrent trim | `TestRestorePointProjectionRetentionParity`: registration mismatch/snapshot failure preserve reasons; targets/baselines/overlaps/holds, five failed restores, count-evicted failed slot, separate targets/restart; deferred trim after reclassification preserves anchors. | Filter by display availability; remove anchor/hold; protect failed slot; drop transactional recheck. |

Phase 2 owns `activity/capabilities*.go`, evidence helpers/tests in `activity/runtime_image*.go`, `middleware/environment*.go` and its injected port;
`bootstrap/router_bootstrap.go` wiring; `project/lifecycle_{points,pins}*.go`, `project/children/lifecycle/{retention,store}*.go`, `types/updater/` enums/tests;
`backend/pkg/libarcane/edge/command_client*.go` only for bounded point-list response assembly/tests; `scripts/agent-contract.mjs`, generated contracts and named message subtree.
No admission lock change or runtime_http_service.go edit is needed. Other web code/tests belong to #231; migrations and other producers stay read-only.
Focused checks/red mutations precede full lint/test under the load rule. Phase 1 changes this design only and uses text checks.
