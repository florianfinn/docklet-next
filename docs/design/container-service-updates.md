<!--
Source: docklet-next/issues/342
Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
Retrieved: 2026-11-04
License: Apache-2.0
Deviations: Original container and single service updates rationale.
-->

# Container and single service updates

Decision for #342 and #343 (M18, maintainer kickoff 2026-11-04). Docklet is a homelab tool with
normally one administrator; nothing here is secured twice.

Builds on [container origin and recreate](container-origin-recreate.md) (M14) and the stack
lifecycle in [images and updates](images-updates.md).

## Accepted decisions (do not reopen)

- B1: standalone containers get check, install and a policy per container through recreate, next to
  stack updates. Unraid and uncertain origin stay locked (M14).
- A20: one service of a stack can be updated; the whole-stack update stays.
- core-adoption: show failures and offer the previous image/configuration for manual recovery; no
  automatic rollback; no promise to restore application data.

## Starting point (main 89f69980)

- `recreate-container` / `redeploy-container` are durable runtime operations (admin rule
  `access/rules.go:283`) with the origin guard at admission (`GuardRuntimeOrigin`,
  `container/origin.go:146-162`) and again in the executor (`effectOriginGuard`). The recreate
  journal renames the old container, creates and starts the new one and removes the old one; on
  failure it compensates (remove new, rename old back, start old) and reports
  `container.recreate-recovered` / `container.recreate-ambiguous`; startup recovers interrupted
  journals (`container/recreate_journal.go:67-243,345`).
- `RedeployContainer` re-pulls the same tag; `EditContainer` pulls only when the image is missing.
  Neither pins by digest nor keeps the previous image. The old `update-container` route returns
  `update.path-retired` (`updater/handler.go:89-91`).
- Container observations come from `GetUpdateInfoByContainers` (`container/service_reads.go:251-287`).
- Stacks: `LifecycleRequest` has no service selection (`types/updater/lifecycle.go:18-22`); the update
  preflight checks all services (`lifecycle_preflight.go:143`); `deployLifecycle` deploys the whole
  model with `PullPolicy:never, ForceRecreate:true` and `Up(model, nil, …)` (all services)
  (`lifecycle_workflow.go:280-393`, `pkg/projects/coordinator.go:83`).

## Design: container update

1. **Operation** `update-container`: `POST /api/environments/{id}/containers/{containerId}/update`
   (replaces the retired route), runtime operation `RuntimeUpdateContainer`, durable admission like
   recreate (idempotency key, `Prefer: respond-async`), origin guard at admission and executor.
   Rights: the existing update grant set W on the container (admin; M15 assigned-application rights
   map a standalone container like today's recreate, which is admin-only).
2. **Candidate.** The executor runs a fresh check of the container's image reference (same code as
   `check-image-update`). No candidate → `job.skipped` (no effect). Version candidate → new reference
   with the new tag; rebuild candidate → same reference.
3. **Pull.** Pull the target reference (tag) with the container's platform, the same way
   `RedeployContainer` does today; compute the expected local image ID from the candidate with the
   existing native identity helper (`imageupdate/children/identity/types.go:101-112`, classic
   store: config digest, containerd store: manifest digest; as the stack preflight does,
   `lifecycle_preflight.go:175-202`) and require the pulled tag to resolve to it, otherwise
   `update.target-changed` before any container effect. No retag and
   no tag compensation: like today's redeploy, the tag stays on the pulled image even if the
   recreate later fails (the restored old container keeps running its own image ID).
4. **Previous image slot**, written **before** the recreate: in one transaction the
   executor replaces the container name's row in `container_previous_images` (executor DB, schema
   migration 018) with the image ID and reference the container runs now, plus run ID, time and
   `owner_container_id` = the current container ID, and
   moves the guard pin (new protection reason `container-previous`, added to the guard's Protect
   allowlist and to the delete/prune/UI reason projection) from the old slot image to this
   one. If the update then fails and the old container is restored, the slot equals the current
   image and the UI hides "Restore previous image"; nothing is lost. A crash at any point leaves a
   consistent slot. **Slot ownership:** when the update's recreate finishes (normal or
   startup recovery with the verified image), the slot's `owner_container_id` advances from the old
   ID to the replacement ID conditionally (`WHERE run_id = ? AND owner_container_id = old`). The
   slot is offered and restorable only while the name resolves to `owner_container_id`; any other
   instance (outside replacement, or another Docklet edit/redeploy) makes it stale: hidden in the
   UI, restore refused with `update.target-changed`, zero effects.
5. **Recreate.** Existing durable recreate with the unchanged configuration except `Config.Image`
   = target reference (M14 edit path). The journal row stores the expected image ID (new nullable
   column, migration 018). The journal's compensation and startup recovery apply unchanged;
   before a normal or recovered update run settles as success, the replacement's image ID is
   compared with the stored expected ID; mismatch (tag moved between pull and create) →
   `update.target-changed`, the replacement stays as it is, no rollback, the slot owner does not
   advance.
6. **Failure.** Pull failure or digest mismatch → no container effect. Recreate failure → the
   journal restores the old container (`container.recreate-recovered`) and the run fails with
   `update.deploy-failed`; ambiguous journal outcomes keep `container.recreate-ambiguous` and the
   runtime exclusion as today.
7. **Manual recovery.** `restore-container-image` (`POST …/containers/{id}/previous-image/restore`,
   runtime operation, same rights and guards, instance proof as for update): if the slot reference
   no longer resolves to the slot image ID, tag the slot image with that reference; then recreate
   with `Config.Image` = slot reference. A crash between tag and recreate leaves the tag on the old
   image and the container on the new one; the next check shows an update, which is consistent.
   The container page shows "Restore previous image" with reference and date while the slot image
   differs from the running image. Volumes/binds are untouched (stated in the dialog).
8. **Policy.** Container policy rows are keyed by name (update-policy note); the window job calls
   the same runtime operation with the policy actor.
9. **Excluded.** Compose-managed containers keep the stack path (container detail links to the
   stack/service update). Unraid, uncertain and the protected self container: no update action, no
   policy, observation only.

## Design: single service update

1. `LifecycleRequest.services []string` (optional; empty = whole stack), frozen in the intent.
   Preflight validates every name against the current model (`update.invalid-target`, 400) and
   checks only the selected services.
2. **Shared source.** The version rewrite is per service (`lifecycle_inputs.go:201-212`), so other
   services with the same literal reference are not changed. If the selected service's image field
   is a YAML alias/anchor shared with another service, the update is refused with
   `update.image-binding-unsupported` (narrow, rare; whole-stack update remains).
3. **Point.** The restore point remains a whole-stack snapshot (unchanged code path).
4. **Deploy scope.** The deploy model is reduced to the selected services (like `deployLifecycle`
   already drops absent services and their `depends_on` entries), deployed with force recreate and
   orphan removal forced off at the real option boundary for this path (the ComposeUp merge
   `removeOrphans || envOpts.RemoveOrphans`, `pkg/projects/cmds.go:164-170`, must not turn it on
   from `COMPOSE_REMOVE_ORPHANS`; ordinary deploys keep the merge), so compose neither
   starts, stops, removes nor recreates unselected services. The
   pre-Up stop signals (`lifecycleStopSignals`, `lifecycle_workflow.go:318-334`), image selection
   and verification use the same selection.
5. **Coupled services.** If a selected service and an unselected one are linked by
   `network_mode: service:`, `ipc`/`pid: service:` or `volumes_from` (either direction), the single
   service update is refused before admission with `update.service-coupled`; the dialog suggests
   the whole-stack update.
6. **Verification** checks image IDs of the selected services; the baseline is saved for the whole
   stack after success (fresh observation of all services, existing code).
7. **Gestures.** Services card, update radar and the update dialog offer per-service update;
   "Update stack" remains. Scheduled windows always update the whole stack.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
|---|---|---|---|---|
| CU1 | origin locks | Unraid/uncertain/compose/self refused 409 `container.origin-locked` at admission and executor | admission guard, executor guard | table test incl. origin change between admission and execution (mutation: drop executor guard) |
| CU2 | no candidate | fresh check finds nothing → `job.skipped`, zero pulls/recreates | executor | test (mutation: recreate anyway) |
| CU3 | partial failure | digest mismatch → zero container effects; create/start fails → compensation restores old container; run `update.deploy-failed`; slot equals running image, restore hidden; existing older slot replaced consistently | executor + recreate journal | Go test starting with an existing slot (mutations: write slot after recreate; skip digest check) |
| CU4 | crash mid-update | crash before slot write, after slot write, after rename, after start, after done → slot consistent, startup journal recovery, no second pull/recreate, receipt settles once; tag moved after pull check + crash before normal verification → recovered run settles `update.target-changed`, not success | slot store + startup `RecoverRecreates` + runtime settlement | crash-cut table test (mutations: replay the update intent at startup; journal result alone implies success) |
| CU8 | slot ownership | outside same-name replacement or another Docklet recreate → slot hidden, restore refused with zero retag/create; successful update (normal and recovered) advances the owner | slot store + restore admission/executor | test (mutation: check only the current admission ID, ignoring `owner_container_id`) |
| CU5 | previous image slot | pinned by the real guard (new reason accepted, truthful prune/delete reason); restore recreates with the previous image, also for a same-tag rebuild (tag moved back) | slot store + guard + restore op | test with the real guard (mutations: no pin → prune removes it; no retag → restore uses new image) |
| CU6 | Direct/Edge parity | update and restore go through `runtime.execute` on the agent with identical results | real runtime transport | transport test with httptest agent and Edge path (mutation: missing operation in agent allowlist) |
| CU7 | identity | container ID changes; run evidence and policy refer to the name; a late result for the old ID never updates a new container with another name | executor target check | test (mutation: resolve by ID only after recreate) |
| SV1 | selection validation | unknown service 400; empty = whole stack | preflight | test (mutation: ignore unknown names) |
| SV2 | shared source / coupling | alias-shared image → `update.image-binding-unsupported`; namespace or volumes_from link across the selection → `update.service-coupled`; both before admission | preflight | fixtures with `network_mode: service:`, `pid`, `volumes_from`, anchors, a stopped dependent (mutation: skip coupling analysis) |
| SV3 | deploy scope | only selected services recreated; unselected services (incl. a running one with a custom stop signal and a stopped dependency) keep ID and state on success and failure | real `ExecuteImageLifecycle` → coordinator → Docker double + sandbox H/D/E | Go test (mutations: whole-source `lifecycleStopSignals`; full model to `Up`; keep the env OR merge with `COMPOSE_REMOVE_ORPHANS=true` and unselected running and stopped containers) |
| SV4 | failure | failed service update offers the whole-stack point; untouched services keep running | executor | test (mutation: stop all services on failure) |
| SV5 | exclusion | service update vs whole-stack update vs window job on the same stack → existing runtime target exclusion (`run.busy`) | runtime admission | existing exclusion test extended (mutation: per-service target key) |

## Contract

Agent contract v14 (owned by the update policy model change): runtime operations `update-container`,
`restore-container-image`, `LifecycleRequest.services`, container policy kind; new reason codes
declared in `types/image/reason.go`; EN/DE texts.

## Kickoff decisions (2026-11-04)

- One previous-image slot per standalone container, with "Restore previous image".
- A single-service update of a coupled service (namespace sharing, `volumes_from`) is refused with
  `update.service-coupled` and a hint to update the whole stack.
- A container policy follows the container name ([update policy model](update-policy-model.md)).
