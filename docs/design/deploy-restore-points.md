<!--
Source: docklet-next/issues/344
Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
Retrieved: 2026-11-04
License: Apache-2.0
Deviations: Original restore points before deploy and redeploy rationale.
-->

# Restore points before deploy and redeploy

Decision for #344, #252 and #250 (M18, maintainer kickoff 2026-11-04). Docklet is a homelab tool with
normally one administrator; nothing here is secured twice.

Extends [images and updates](images-updates.md) and the restore-point protection addendum in
[update policies](update-policies.md); their safety invariants stay binding.

## Accepted decisions (do not reopen)

- B2: restore points also before deploy and redeploy; after a failure the point is offered for
  recovery. core-adoption: no automatic rollback; no promise to restore application data
  (volume/bind data is never saved).
- Five retained points per target with the restore-target and healthy-baseline anchors and one
  failed-attempt slot (M6, confirmed 2026-10-30 "five retained restore points").

## Starting point (main 89f69980)

- Points are created only inside `ExecuteImageLifecycle` for update and restore
  (`project/lifecycle_workflow.go:28`, `activity/runtime_executor.go:145-151`). A point is the
  encrypted `lifecycle.Snapshot` (compose file, env/config files, compose model, per-service image
  ID/digest/platform) plus image pins (`lifecycle_pins.go:96-118`) in the executor's own DB (hub for
  environment 0, the agent DB for Direct/Edge).
- A normal deploy marks the baseline unavailable first (`project/service.go:731`,
  `Store.Invalidate` sets `available=false`, payload kept), and after success saves the new
  baseline from the deployed inputs (`captureDeployedBaseline`, `lifecycle_baseline.go:15-30`).
  So the baseline payload is the **last recorded baseline**: normally the last successful Docklet
  deploy, but successful updates and restores also write it, and a deploy whose baseline capture
  failed leaves the older one.
- Deploy runs already record a `LifecycleResult` (baseline code) into activity metadata; the hub
  validates it with `checkedActivityLifecycle` (`activity/runtime_image_boundary.go`), and
  `LifecycleResult.RestorePointID` exists (`types/updater/lifecycle.go:96`).
- `image_lifecycle_runs` rows settle only for update/restore receipts
  (`activity/runtime_store.go:430`); `ReconcileImageLifecycle` deletes points whose run settled
  without effect (`lifecycle_pins.go:232-241`) and runs only after runtime settlement, never at
  startup (#252). Multi-file compose stacks cannot be snapshotted (`lifecycle_inputs.go:76-78`).
- Web: the restore-point list keeps `holdState=active` after the run settles until a re-read (#250);
  terminal activities do not emit the `stacks` topic (`web/src/data/http/http-adapter.ts:545-576`).

## Design

1. **Point source.** Before `DeployProject` and before `RedeployProject` (so before its pull), the
   executor (`RuntimeServiceAdapter.ExecuteRuntime`, deploy/redeploy cases) calls
   `ProjectService.PrepareDeployPoint(ctx, projectID, runID, registration)`. It reads the baseline
   row **without** requiring `available` (payload = last recorded baseline) and requires matching
   target identity and registration. It does not snapshot the current files: for deploy they are
   already the new, unproven configuration. The UI calls it "last recorded deployment".
2. **No reuse.** Every deploy/redeploy with a usable baseline creates its own point (simple
   ownership: one run, one new point). Repeated failing deploys add points of the same state; the
   healthy-baseline anchor and the five-point retention bound them.
3. **Creation.** Under the image lease, `store.Prepare(snapshot, run, "", "settled")` creates the
   point and its `image_lifecycle_runs` row in one transaction and `lifecyclePinPoint` pins the
   baseline image IDs. If any image of a service that the baseline records as present is no longer
   available locally, no point is created (`PointCode = deploy.point-unavailable`); workload
   presence (`Present`) is never changed to express image availability. The run row's
   `had_effect` is set immediately before the compose deploy starts.
4. **No point is never a refusal**. No baseline (first deploy, deploys only from outside
   Docklet), identity/registration mismatch, multi-file stack, or a point/pin persistence error →
   the deploy proceeds; `LifecycleResult.PointCode` carries the reason (`deploy.point-unavailable`,
   `update.image-binding-unsupported`, `update.persistence-failed`) and the run detail shows a
   neutral note "no restore point".
5. **Evidence, durable.** The point ID is persisted before any effect in the run's
   `image_lifecycle_runs` row (`id = run ID`, `point_id`), and `PointCode` with it (new nullable
   column `point_code`, migration 018). Every settlement of a deploy/redeploy receipt —
   normal, interrupted at startup, or queried later — derives `RestorePointID`/`PointCode` from
   that row and merges them into the `LifecycleResult`; the baseline capture result merges its
   baseline fields into the same result instead of replacing it. `checkedLifecycleResult`
   validates `PointCode` with the declared reason enum (#254). The executor never marks a deploy
   point `failed_attempt`: it holds the last recorded good state and is the recovery target.
6. **Settlement and holds.** `runtime_store.go:430` settles `image_lifecycle_runs` for
   deploy/redeploy receipts too (only rows that exist). While the run is unsettled the executor
   `Held` check protects the point from delete; restore of the same stack is excluded by the
   existing runtime target exclusion.
7. **Recovery offer.** A failed (or interrupted) deploy/redeploy run whose evidence carries a
   restore point shows "Restore previous deployment" in the run detail and in the stack's
   deployments tab; it opens the existing restore dialog with that point preselected (explicit
   configuration-replacement acknowledgment as today). Nothing restores automatically.
8. **Retention.** Deploy points join the per-target retention unchanged (five, anchors, holds).
9. **#252 startup cleanup.** Each executor process (hub and agent) runs `ReconcileImageLifecycle`
   once after `InitializeLocalStartup` and before `SetReady(true)` (`di/runtime.go:151-166`).
10. **#250 web.** A terminal `project_update`, `project_restore`, `project_deploy` or
    `project_redeploy` activity emits the `stacks` change topic, which invalidates
    `["stack", id, "update-points"]`.
11. Webhook deploys (removed in M20) bypass the runtime executor and get no point (documented limit).

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
|---|---|---|---|---|
| RP1 | point content | deploy point = last recorded baseline payload, not the new files; baseline image missing → no point, presence never altered; partial pin failure → no usable point | `PrepareDeployPoint` | test: edit compose, deploy; point files/images equal the previous baseline; pruned baseline image → `deploy.point-unavailable` and a restore of an older point still has the service (mutations: snapshot current inputs; set Present=false) |
| RP2 | failure and recovery | failed deploy keeps the point, not failed_attempt; run evidence carries its ID; restore returns previous files and image IDs and the stack runs | executor → receipt → hub boundary → web | Go test with failing compose up + restore; sandbox H/D/E (mutation: mark failed_attempt / drop RestorePointID) |
| RP3 | no point | first deploy, no baseline, mismatch, multi-file, persistence error → deploy proceeds with `PointCode` | `PrepareDeployPoint` | table test (mutation: return error → deploy refused) |
| RP4 | interrupted evidence | crash after `had_effect` before executor return → startup settlement carries the exact `RestorePointID`; a successful deploy keeps point and baseline fields together | run row → settlement (normal/startup/query) → hub → web | restart test on separate hub/agent lifetimes via Direct/Edge query (mutations: in-memory-only evidence; baseline result overwrites point fields) |
| RP5 | crash/restart | executor restart before `had_effect` → point removed at startup reconcile; after `had_effect` → point kept, run settled, no hold left | startup reconcile (#252) | restart test on hub and agent executor (mutation: remove startup call, or set had_effect after deploy) |
| RP6 | holds | delete of the point refused while the deploy run is unsettled; allowed after settlement | executor `Held` + settlement | test (mutation: no settlement for deploy receipts → hold never ends) |
| RP7 | redeploy order | point before pull; tags moved by the pull do not change pinned IDs | executor redeploy case | test with a tag moved by pull (mutation: point after `PullProjectImages`) |
| RP8 | retention | deploy points evict oldest unanchored points; anchors and holds kept | `PlanRetention` | retention test with mixed kinds (mutation: count deploy points separately) |
| RP9 | Direct/Edge evidence | `RestorePointID`/`PointCode` survive the real runtime transport and hub validation; undeclared `PointCode` → `runtime.outcome-unknown` | `checkedLifecycleResult` | transport test (mutation: skip PointCode validation → raw value persisted) |
| RP10 | web invalidation | restore action enabled after the run settles without reload | http adapter stream | vitest with real adapter stream (mutation: no topic for terminal runs) |
| RP11 | prune protection | image prune keeps images pinned by deploy points | image guard | test (mutation: no pin for deploy points) |

## Contract

`LifecycleResult.pointCode` (new, enum = declared reasons) and the deploy/redeploy settlement are
part of agent contract v14 (owned by the update policy model change). New reason `deploy.point-unavailable` in
`types/image/reason.go` with EN/DE text; the activity web model reads `restorePointId` and
`pointCode` for deploy runs.

## Kickoff decisions (2026-11-04)

- A deploy for which no restore point can be made proceeds; the run carries the reason.
- Deploy points share the five retained points per stack.
