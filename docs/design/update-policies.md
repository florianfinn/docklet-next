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
