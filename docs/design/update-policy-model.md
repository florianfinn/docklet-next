<!--
Source: docklet-next/issues/341
Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
Retrieved: 2026-11-04
License: Apache-2.0
Deviations: Original update policy model rationale. M24 S6: observed recovery and update UI follow-ups.
-->

# Update policy model

Decision for #341, #345 (backend) and #242 (M18, maintainer kickoff 2026-11-04). Docklet is a homelab tool with
normally one administrator; nothing here is secured twice.

Replaces the stack-only, host-cron model of [update policies](update-policies.md) where this
note says so; everything not named here
(admission, frozen revision, child checkpoints, notification identity, restore-point protection)
stays as accepted.

## Accepted decisions (do not reopen)

- A1: check only by default, every 12 h, for all stacks and standalone containers.
- A2: one global check interval 6/12/24 h; install windows daily or weekly per stack and per
  container (A14: no "once"). A3: the `autoUpdate` master switch and the per-host cron jobs go.
- A4/A5: "Check all now" and the approved Settings › Updates table (stacks and containers, mode
  check | scheduled, window).
- B1: standalone containers get check, install and a policy through recreate; Unraid and uncertain
  origin stay excluded ("Docklet does not manage their updates").
- core-adoption: never install automatically without activation; plain manual updates require manual recovery; [observed updates](scheduled-update-recovery.md) can restore workloads.

## Starting point (main 89f69980)

- `stack_update_policies` (009) holds mode off/notify/apply per stack; `image_update_schedules`
  (009) holds per-host polling/apply cron, enable flags and the schedule owner actor. Missing policy
  row = `off`; missing schedule row = owner unset → `update.schedule-owner-unset`
  (`updater/children/policies/store.go:32,67-77`).
- Hub job pair per host `image-policy:<env>:image-polling|auto-update`
  (`pkg/scheduler/image_policy_job.go:46-67`); runs only if setting `pollingEnabled` and (apply)
  `autoUpdate` are true and the host row is enabled with an owner (`policy_service.go:288-292`).
- `RunImagePolicyHost` visits only existing policy rows (`policy_execution.go:85-87`), so nothing
  is checked by default. The policy route accepts kind `project` only (`policy_routes.go:26`).
- Agents still register inert `auto-update` / `image-polling` jobs that return
  `update.agent-local-job-disabled` (`pkg/scheduler/auto_update_job.go`,
  `image_update_watcher.go`; wired in `jobs_bootstrap.go`).
- #242: `image_policy_children`, `image_update_candidates`, `image_update_notifications` have no
  pruning and no foreign keys.

## Design

### Settings and storage

- Hub settings `updateCheckInterval` ∈ {`6h`,`12h`,`24h`} (default `12h`; no "never", ) and
  `updateNotify` (bool, default **off**, ). Removed settings and their readers: `autoUpdate`, `autoUpdateInterval`,
  `pollingEnabled`, `pollingInterval`. `autoUpdateExcludedContainers` stays (it feeds the existing
  `autoExcluded` refusal).
- Migration 018 creates `update_policies` (table rebuild of `stack_update_policies`) **keeping
  each row's `id`** (children, candidates, frozen `PolicyBinding.PolicyID` and reconciliation join
  on it): `id, environment_id, registration_id, target_kind (stack|container), target_id, target_identity,
  mode (check|scheduled), window_kind (daily|weekly|NULL), window_day (0-6|NULL), window_time
  (HH:MM|NULL), revision, actor_id, actor_key_id, checked_at, code, apply_checked_at, apply_code,
  bookkeeping_at, bookkeeping_code, candidates, missing_since`, unique `(environment_id, target_kind, target_id)`,
  CHECK constraints for mode/window consistency. It copies stack rows and drops
  `stack_update_policies` and `image_update_schedules`. The settings rows for the removed keys are
  deleted in the same migration.
- Mapping (what was really active stays active): `off`, `notify` → `check`. `apply` → `scheduled` only if it was effective at
  migration time, i.e. the full current gate of `ImagePolicyEnabled` (`policy_service.go:278-292`):
  `pollingEnabled=true` **and** `autoUpdate=true` (stored value or default), host row
  `auto_update_enabled=1` with a non-empty actor; otherwise `check`. Whether the host is
  currently reachable does not matter; a disabled environment stays disabled by its own flag. Window from the host's `auto_update_interval`: `0 M H * * *` → daily HH:MM,
  `0 M H * * D` (single day) → weekly; anything else → weekly Sunday 04:00 (approved default).
  `updateNotify` = true only if notifications were really produced: a `notify`/`apply` row exists on a
  host whose `image-polling` gate held (`pollingEnabled` stored or default true, host row
  `polling_enabled=1`, non-empty actor, host enabled); otherwise false (the new default).
- A row exists only when a target has been checked or has a non-default policy. A missing row means
  `check` with no observation yet; the API projects it, never an error.

### Target identity

- Stack: `target_id` = project ID, `target_identity` as today.
- Container: `target_id` = container **name** (recreate changes the ID; the name survives). The
  name is the policy identity (a same-name container created outside
  Docklet inherits the policy, like a stack keeps its policy across outside `compose up`; the
  origin guard still decides eligibility). Instance proof for effects: an install admission freezes the container ID
  it resolved for that name; the executor refuses with `update.target-changed` when the name now
  resolves to another ID, and a Docklet recreate is the only path that advances the frozen ID. Only containers whose effective origin is confirmed standalone are targets; Unraid,
  uncertain, compose-managed and the protected self container never are (origin guard
  `container/origin.go:146-162`, including remote origin overlays and manual overrides).
- Notification dedup identity (`store.go:151`) gains target kind and target ID, so two containers
  with the same image on one host get separate entries.

### Jobs

- **One hub job `update-check`** with cron from the interval (`0 0 */6 * * *`, `0 0 */12 * * *`,
  `0 0 0 * * *`; hub zone and wall-clock rule from the schedule note). It visits every enabled
  registered host serially; per host it reads the stack list and the container list once, checks
  every stack through the existing check path (local `CheckProjectUpdates`, Direct/Edge proxy) and
  every standalone container through `check-image-update` for its image reference on that host,
  records `checked_at/code/candidates` per target and, if `updateNotify`, the existing deduplicated
  notification entry per candidate. Lists are read completely (`start=0&limit=-1`, envelope
  validated). A host failure records `agent.unavailable` for that host's targets it could not
  read and continues with the next host.
- **Observation-only checks.** Checks write observation records and activities on the
  executor but never deliver notifications there: the executor-side `notifyImageUpdateInternal`
  call (`imageupdate/service.go:359,378-395`) and its batch flush are removed; delivery is owned by
  the hub (`updateNotify` + dedup) for every check source, manual or scheduled.
- **Authority.** `update-check` and the fresh check inside a window run through a trusted
  system observation path that does not go through `imagePolicyActor`/role lookup and carries no
  apply authority. Installs always use the policy actor, revalidated at fire time. New and removed
  operation IDs are added to `access.Rules` and `TestM15RuleCompleteness` stays green.
- **"Check all now"**: `POST /api/updater/check-all` (`check-all-updates`, admin) triggers a manual
  run of `update-check` through the existing `run-job` path and returns its run ID.
- **Window jobs** `update-window:<env>:<kind>:<target>` exist only for `scheduled` rows; cron from
  the window (`0 MM HH * * *` or `0 MM HH * * D`). `RefreshUpdateWindows` (replacing
  `RefreshImagePolicyJobs`) adds/removes them on policy save, host add/remove and startup. At fire
  time: revalidate the recorded actor's apply grants and the target identity (existing
  `update.permission-denied`, `update.target-changed`), fresh check of that one target, and if a
  candidate exists admit the install: stacks through the existing `AdmitPolicyUpdate` child path
  (whole stack), containers through the new `update-container` runtime operation (container note).
  The existing child checkpoint/reconcile semantics, frozen revision and `update.policy-changed`
  apply unchanged. Missed windows (host offline, hub down) are not caught up.
- Agent-local `auto-update` / `image-polling` jobs, `image-policy:*` jobs, `get/set-update-schedules`
  and `update.schedule-owner-unset` / `update.agent-local-job-disabled` are removed (codes stay
  declared only if a stored run still carries them; otherwise deleted from types/ and catalogues).

### API

- `GET/PUT /api/environments/{id}/updater/policies/{kind}/{targetId}` with kind `stack|container`
  (hub-owned route, the web is its only client, so `project` is not kept as an alias). PUT body `{mode, window?, expectedRevision}`. `scheduled` requires the apply grant set A
  (administrators only, includes `jobs:manage`; full-access application users get 403 `update.permission-denied` and still install by hand); `check` requires N.
- `GET /api/environments/{id}/updater/policies` (`list-update-policies`, host descriptor): every
  stack and standalone container of the host with projected mode/window/status, built from the
  inventory read plus rows; one call per host for the settings table.
- Capability descriptors: `setAutoUpdate` (check), `setAutoUpdateApply` (scheduled) gain the
  container ref kind `container` (`env~name`); `installContainerUpdate`, `setContainerAutoUpdate`
  get real rules (today `capability.mapping-unavailable`); `listSchedule` / `saveUpdateSchedule`
  are removed; `checkAllUpdates` is new (host-independent, admin).

### Pruning (#242), kept simple

- Environment deletion deletes its `update_policies`, children, candidates and notifications in the
  deletion transaction.
- Stack destroy through Docklet deletes the stack's row and dependants unless it has an unsettled
  child.
- Absent targets: after a host's lists were read completely and successfully, a row whose target
  is absent is deleted only if it is mode `check`, has no unsettled or unknown child, and its
  target has been absent for 7 days (`missing_since` column set on the first complete read without
  the target, cleared when it reappears). A recreate's temporary rename never reaches 7 days; a
  partial or failed read never sets or advances `missing_since`. Scheduled rows are never pruned
  automatically; the settings table shows them as "target missing" with a remove action. Remove sets the
  policy to `check`; the row then follows the check-mode pruning rule.
- Late observations commit conditionally: `UPDATE … WHERE id = ? AND registration_id = ? AND
  revision = ?`; a row is created only inside a transaction that sees the environment row. So a
  slow check cannot resurrect a deleted environment's rows or overwrite a newer policy.
- Superseded candidates (not in the row's current `candidates`) and their terminal notifications
  older than 30 days are deleted; the current candidate's dedup identity is never deleted, so a
  restart never re-notifies. Settled/refused children older than 30 days are deleted; admitted or
  unknown children never.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
|---|---|---|---|---|
| UP1 | forward migration | rows off/notify/apply × pollingEnabled × autoUpdate (stored and default) × host-enabled/owner × cron shapes, with prepared/admitted/unknown children, candidates and pending/sending notices (IDs kept, same receipt reconciled after reopen without redispatch) → mode/window/notify as above; drops old tables; settings rows removed; reopen + integrity | migration 018 | `TestUpdatePolicyForwardMigrationFrom017` (mutations: omit pollingEnabled from the gate; notify mapping ignores the host polling gate; regenerate policy IDs) |
| UP2 | default check | fresh hub, no rows: one `update-check` run checks every stack and standalone container on H/D/E | job → host loop → real Direct/Edge transport | test with httptest agent and Edge path (mutation: visit only existing rows) |
| UP3 | install only when scheduled | window fire installs only `scheduled` targets; `check` never admits a child | window job → admission | test (mutation: admit for check rows) |
| UP4 | actor revalidation | actor loses apply grant before fire → `update.permission-denied`, no child | window job | test (mutation: skip revalidation) |
| UP5 | revision race | policy changed between fire and admission → `update.policy-changed` | existing admission | existing test extended to container kind |
| UP6 | container identity | update by recreate changes the ID; policy by name still applies; renamed container → new name defaults to check, old row pruned after 7 days (check mode) or shown as target missing (scheduled) | target key | test (mutation: key by container ID) |
| UP7 | origin exclusion | Unraid/uncertain/compose-managed/self never listed as targets, policy PUT refused 409 `container.origin-locked` | list builder + PUT + admission guard | test per origin (mutation: list by inventory without origin filter) |
| UP8 | host failure isolation | offline host → its targets `agent.unavailable`, other hosts checked; no pruning of the failed host | host loop | test (mutation: return on first host error / prune on partial read) |
| UP9 | restart mid-run | hub restart with an admitted window child → reconcile receipt, no second effect | existing child checkpoint | existing restart test extended to window jobs and container kind |
| UP10 | notification dedup | same candidate across runs, restart and notify off/on → one entry; two containers with the same image on one host → two entries; notify off → badge only, no entry | `RecordCandidate` | test (mutation: drop unique identity) |
| UP11 | pruning | env delete, stack destroy, absent target only after 7 days of complete reads in check mode without unsettled child, superseded candidates; current candidate kept; held recreate rename survives a full discovery; late result after env delete/policy save does not resurrect/overwrite | deletion paths, job | `TestUpdatePolicyPruning` (mutations: prune current candidate → re-notify; unconditional absent delete; unguarded upsert) |
| UP12 | window jobs refresh | save scheduled → job exists with cron from window; switch to check → job removed; host removed → jobs removed; startup rebuilds | `RefreshUpdateWindows` | test (mutation: no removal on check) |
| UP13 | removal | settings keys, per-host jobs, agent-local jobs, schedule routes gone; v14 fixture; web has no schedule/master UI | contract + structural | contract test + `TestNoLegacyUpdateJobs` (mutation: keep agent-local job registration) |
| UP14 | Direct/Edge parity | container check and policy listing work identically on Direct and Edge through the real agent transport | proxy / Edge command path | transport test (mutation: drop container check forwarding on Edge) |
| UP15 | shared read results | hub-local container lists coalesce in Docker `ListContainers` (`docker/service.go:357-372`); the target builder only reads names, origins and image refs and never edits label maps or slices in place | target builder on the real producer | `-race` test with concurrent `update-check` and container-list callers on the real producer (mutation: sort/label edit in place) |
| UP16 | executor notifications | checks from any source never deliver on the executor; hub delivers once if `updateNotify` | real check producer → Direct/Edge → hub outbox | send spies on executor and hub, notify off/on, repeated checks, restart (mutation: keep `notifyImageUpdateInternal`) |
| UP17 | instance proof | name resolves to a different container ID than frozen at admission → `update.target-changed`, zero effects; Docklet recreate advances the ID | executor | test with external same-name replacement between admission and execution (mutation: resolve by name only) |

## Migration and contract gates

Real goose upgrade 017 → 018 with reopen, `PRAGMA integrity_check`/`foreign_key_check`; fresh
schema equals the GORM model catalog (`schema/schema_integration_test.go:93`) for every 018 table
(`update_policies`, `container_previous_images`, and the dropped tables gone); migration manifest
entry for 018 only (001–017 hashes unchanged); `LifecycleRequest.services` compared semantically
(sorted set) where the frozen intent is compared today (`activity/runtime_image_intent.go:45`), the
admitted slice owned by the intent (caller mutation cannot change it), and part of the idempotency
digest. One foundation change owns every schema, types/, OpenAPI, DTO, fingerprint and v14 fixture
change of M18; later changes consume them and never edit migration 018.

## Contract

Agent contract **v14** (minimum agent 14, announced as a protocol break): removed agent-local jobs,
policy kinds `stack|container`, the container update runtime operation and `LifecycleRequest`
service selection (container note), deploy restore-point fields (restore-point note), #254 enums.
One owner regenerates OpenAPI, DTOs, fingerprint and the v14 fixture.

## Kickoff decisions (2026-11-04)

- Upgrade principle: what was really active stays active. Only `apply` policies that were
  effective at migration time become `scheduled`; notifications stay on only where they were really
  produced.
- "Notify when updates are found" is off by default.
- Only administrators set a scheduled install; full-access application users check and install
  by hand.
- Checks cannot be switched off; the interval is 6, 12 or 24 hours.
- A container policy follows the container name.
