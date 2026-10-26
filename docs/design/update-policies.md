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
paths below are relative to `backend/internal/`. M2 requires points for update/restore and a baseline before update.
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

Use the full route/nested grant conjunction, never weaker. `capability.unavailable` covers invalid/scoped-denied refs.
Container update/apply and host-wide/unscoped check requests stay unavailable.
Availability describes discovery; calls recheck rights, identity/revision and holds.
Policy descriptors preserve off/notify rights. For remote baseline and point reads/deletes, hub guards are
`middleware/environment.go:186` through :203, plus the matcher at :204 and :325.
Agents have sudo rights (:180); revocation tests mutate the hub proxy conjunction.
IDs/grants match local/Direct/Edge. Read capabilities from the hub for caller E.
Existing commands are `updater.project.{check,baseline}` and
`updater.restore.{list,delete,host-list,host-delete}` (`backend/pkg/libarcane/edge/commands.go:18`).
Admission, policy, schedules and hints stay hub-owned (`middleware/environment.go:556`).
No new agent route, Edge fallback or route grant is introduced.

### Authoritative point projection

Declare `RestorePointProtectionReason` with only `restore-target|healthy-baseline`.
Required nullable `RestorePoint.protectionReason` uses null for no anchor; missing is invalid. `holdState: RestorePointHoldState` has required enum
`none|active|unknown`. Required `failedAttempt: bool` exposes the stored flag as
a neutral point kind; failure alone never confers protection or a protected label.
There is no failed-slot protection value; replacement/count eviction still apply.

One pure planner in `project/children/lifecycle/retention.go` supplies reasons and
deletion IDs to retention and projection. Both use stored `available = true` rows
per target, ordered `created_at DESC,id DESC` (`project/lifecycle_pins.go:281`).
Display `available`/`code` stay independent: registration mismatch or failed
snapshot verification must not remove a planner input or erase its reason.
Host lists plan each target separately. M6 keeps latest settled baseline, latest
`LastRestoredAt` target, one failed-attempt slot, then evicts oldest other points.
Reason precedence is restore-target then healthy-baseline; internal anchors may overlap. Holds override eviction, even above five. Unheld anchors remain explicitly
deletable with the transactional recheck (`project/children/lifecycle/store.go:263`).

Executor evidence is `image_lifecycle_runs.settled = false` with matching `point_id`
or `selected_point_id` (`project/children/lifecycle/store.go:247`): it means active.
Hub evidence joins `runtime_image_intents.run_id` to `runtime_receipts.run_id`;
match payload `request.pointId`, receipt `environment_id`/`registration_id`,
`kind = receipt` and `settled_at IS NULL`. Executor states pending/accepted/executing
mean active; unknown with non-null `unknown_reason` means an uncertain hub receipt.
Dispatch pending/dispatching/accepted all hold; pending with null `dispatched_at`
already holds before send. Unknown dominates active, then none when neither holds.
Columns obey `backend/resources/migrations/sqlite/003_runtime_receipts.sql:8`; unsettled executor ownership still holds after hub settlement/release. Invalid
evidence or DB reads fail closed; restart/history cleanup cannot release a hold.

The hub overlays its evidence on successful forwarded target/host point lists for
both Direct and Edge, merging executor active with hub active/unknown. The agent
cannot infer hub uncertainty. Before forwarding either delete route, the hub
refuses any non-terminal admission hold with existing declared `restore.in-use`
(409, `types/image/reason.go:18`); agent-local transactional guards remain.
Serialize point selection/intent commit with hub delete check/forward under the same environment/point admission fence, without a DB transaction across remote I/O. If delete
wins, admission revalidates point existence; if admission wins, delete never sends.
This closes the pre-dispatch race, rather than merely reflecting it in the list.

Go constants/schema enum tags own the vocabulary. `scripts/agent-contract.mjs:65`
derives `retentionProtections` from the protection enum and `holdStates` from the
hold enum through OpenAPI; remove the old latest-/active-or-unknown aliases.
`failedAttemptPoints`/`failedAttemptScope` describe replacement only. Generate OpenAPI/DTO/v7 fingerprint/fixture/provenance via `pnpm run contract:generate`;
the drift check enforces Go, manifest and wire enum equality and nullability.
Catalogue owner #229 uses `web/src/shared/messages/{en,de}.ts`, under
`common.restorePoints`: `protection.{restore-target,healthy-baseline}`, `unprotected`,
`hold.{none,active,unknown}`, `kind.{point,failedAttempt}`. #231 treats this subtree
as read-only; no keys go into its updates/settings catalogues. Existing translated
`images.errors.restore_in-use` supplies the hub delete refusal. Both hub overlay
and #231's real web decoder validate agent fields; invalid/missing values fail
loading, never default to null/none or enable delete. Keep the Go contract test.

### Break map, ownership and verification

| Break boundary | Planned test | Red mutation (assertion failure required) |
| --- | --- | --- |
| Scoped deny per host/environment | `TestUpdateCapabilitiesScopedGrants`: every descriptor/grant removed in turn, cross-host split grants, explicit denies, malformed refs and non-admin host lists/deletes; off/notify versus apply. | Replace E with another allowed environment, omit each conjunct, or drop G; affected descriptor incorrectly becomes available. |
| Grant removed after capability read | `TestUpdateCapabilityReadThenRevokedCall`: local and remote E; revoke every grant, call real route, assert 403 and zero reads/effects/forwarding. | Bypass local admission or the hub proxy conjunction/matcher; forbidden call reaches spy. |
| Descriptor/transport parity | `TestUpdateCapabilitiesTransportParity`: identical IDs/grants local/Direct/Edge; actual baseline/point commands; hub-only routes never forwarded. | Remove descriptor/Edge mapping or bypass hub proxy grant; parity/dispatch assertion fails. |
| Direct holds and pre-dispatch delete | `TestDirectPointHoldOverlay`: barrier after restore intent/receipt commit, before send; target/host lists show active, then unknown after receipt uncertainty; both deletes return 409 restore.in-use and send zero requests. | Skip Direct overlay or hub delete guard; hold/effect assertion fails. |
| Edge holds and pre-dispatch delete | `TestEdgePointHoldOverlay`: same barrier/states/target and host routes through actual Edge tunnel; retain agent local guard and restart evidence. | Skip Edge overlay or delete guard; wrong hold or a delete command is observed. |
| Enum contract and translations | `TestRestorePointProjectionContract`: Go/OpenAPI/DTO/manifest/v7 fixture/fingerprint equality, required nullable reason, every EN/DE key/placeholder. | Restore old manifest alias, add failed-slot, drop enum/nullability/key; drift/assertion fails. |
| Real web consumer | #231 `point-shapes`: malformed/missing protectionReason/holdState and neutral failedAttempt through real HTTP decoder; load error, zero delete writes; valid null accepted. | Default invalid values or skip decoder validation; load/control assertion fails. |
| Projection equals M6 retention | `TestRestorePointProjectionRetentionParity`: registration mismatch and failed snapshot check preserve reasons; target/baseline/overlaps/holds, five failed restores, failed slot evicted by count, separate targets/restart; assert exact deletion IDs and neutral failure kind. | Filter planner by display availability, remove anchor/hold, or protect failed slot; reason/deletion assertions fail. |
| Concurrent actor delete/admission/trim | `TestPointDeleteAdmissionRace`: deferred barriers force both orderings of actor B delete versus actor A selection/intent commit, and trim after target reclassification; admitted held point gets restore.in-use, deletion winner prevents restore effects, anchors survive trim. | Drop hub admission fence or transactional hold/existence recheck; effect or deletion assertion fails. |

Phase 2 owns `activity/capabilities*.go`, scoped `activity/runtime_image*.go`
hold/fence helpers, `middleware/environment*.go` overlay/delete boundary and tests,
`project/lifecycle_{points,pins}*.go`, `project/children/lifecycle/{retention,store}*.go`,
`types/updater/` enums/tests, `scripts/agent-contract.mjs`, generated contracts and the named shared message subtree. Other web code/tests belong to #231; updater,
job and Edge producers are read-only, migrations untouched. Focused checks and red
mutations precede full lint/test under the load rule. Phase 1 uses text checks only.
