# Container origin, recreate and edit

Docklet detects where a container comes from, lets an administrator assign an
unclear origin, and recreates or edits standalone containers through runtime
admission. Shared contract and migration items are in
[container and host operations](container-host-operations.md).

## Decisions

- [Core adoption](core-adoption.md): automatic detection and manual assignment
  per container; uncertain origin is restricted; Unraid permits status, logs,
  shell, usage and start/stop/restart but not editor or recreate. Standalone
  containers get Arcane management, editor and recreate with a warning.
- Unraid is its own origin with badge and filter. Its dashboard ring segment is
  off by default and can be shown per personal preference.
- After an external recreate or name reuse, a manual origin needs confirmation
  again.
- Unknown containers with partial Compose labels can be assigned Unraid or stay
  unknown; standalone is not offered and editing stays locked with an
  explanation. Compose sets complete labels, so partial labels never come from
  Compose itself. Reversible: narrowing the backend Compose veto for manually
  assigned containers would allow standalone assignment.
- A redeploy of a Compose child is refused; the web offers stack redeploy or a
  single service restart instead. The synchronous edit and redeploy routes are
  retired in favour of the runtime intakes.

## Baseline

- Origin exists only in the web (`web/src/data/http/inventory-mapping.ts`):
  `compose`; `unknown` for partial Compose labels or a truthy
  `net.unraid.docker.managed` value; `standalone` otherwise. Locks are UI-only.
  The backend knows only `RedeployDisabled` for the hub's own container; edit
  refuses any nonempty Compose project label, and runtime resolution treats a
  project label as managed.
- `edit-container` and `redeploy-container` run as synchronous handler
  activities without admission or idempotency. `update-container` is already
  retired. The shared core `recreateContainerInternal` renames the old
  container, stops it, creates and starts the replacement and removes the old
  one, with rollback branches and an AutoRemove special case.
- Webhooks call `ContainerService.RedeployContainer`, which delegates Compose
  children to `UpdateProjectServices`.
- Remote container JSON is assembled on the agent and forwarded by the
  environment middleware; the inventory middleware captures and retains remote
  list responses.

## Origin

1. **Detection** on the reading host (agent or hub-local) fills a new `origin`
   field on container summary and detail DTOs: complete Compose project and
   service labels → `compose`; otherwise the label key
   `net.unraid.docker.managed` present with any value → `unraid`; otherwise
   partial Compose labels or other manager hints → `unknown`; otherwise
   `standalone`. `originSource` is `detected`.
2. **Overrides** live in the hub table `container_origin_overrides`
   (`registration_id`, `container_name`, `docker_id`, `origin`, `assigned_by`,
   `assigned_at`; unique per registration and name).
   `PUT /api/environments/{id}/containers/{containerId}/origin` with
   `{origin: "unraid"|"standalone", dockerId}` assigns or confirms; `dockerId`
   must equal the container ID read fresh from the host. `DELETE …/origin`
   returns to detection. Permission: `containers:edit` (administrators until
   M15). Assignment is allowed only when the detected origin is `unknown`, and
   standalone only without a Compose project label.
3. **Overlay**: one hub function sets `origin` and `originSource` (`manual`, or
   `unconfirmed` when the stored ID or name differs) on hub-local list and
   detail responses and, through the inventory middleware, on proxied Direct
   and Edge list and detail responses. Retained snapshots are invalidated on
   assign, reset, recreate completion and host removal.
4. **Admission never uses retained snapshots**: the origin guard reads the
   override row and a fresh inspect (labels, ID, name) from the host.

## Guards

5. Hub admission for `recreate-container` and `redeploy-container` requires an
   effective origin of `standalone` (detected, or manual and confirmed) and no
   protected-self container; otherwise 409 `container.origin-locked` with
   `{origin}`.
6. The executing host enforces the same rule at the effect boundary,
   independent of the caller: `RedeployContainer`, `EditContainer` and the
   recreate core refuse a complete Compose identity or Compose project label, an
   Unraid label or the protected-self container. This covers the retained
   webhook caller; webhooks are removed in M20 without a further redesign.

## Recreate and redeploy through runtime admission

7. `POST /api/environments/{id}/containers/{containerId}/recreate` with body
   `{edit: <containertypes.Edit>}` and `…/redeploy` without a body, both with
   `Idempotency-Key` and `Prefer: respond-async`. Strict JSON decoding (unknown
   fields rejected, at most 64 KiB) at the hub intake and again at the receiver.
8. `RuntimeOptions.Recreate` holds the edit; the receiver takes an owned deep
   copy. The idempotency digest uses a canonical encoding: sorted map keys,
   absent pointer sections as null, present-empty sections as their empty
   value, array order preserved. The same key with the same canonical edit
   returns the same run; with a different edit it is 409
   `runtime.idempotency-mismatch` without effect.
9. The result `{oldId, newId, oldName, newName}` is stored in the compact
   receipt `result` column on the executor and in the hub projection, so it
   survives cleared activity history. Run query and retry return it; the web
   navigates to the new ID. A retry after success resolves the retained key
   before inspecting the removed old ID.
10. Activity metadata never contains environment values, credentials or the raw
    edit, only the names of changed fields.
11. The synchronous `edit-container` and `redeploy-container` routes return 409
    `update.path-retired` once the web uses the runtime intakes.
12. The hub terminal projection transaction moves an override conditionally:
    `UPDATE … SET docker_id=newId, container_name=newName WHERE
    registration_id=? AND docker_id=oldId AND container_name=oldName`. Zero rows
    change nothing, so a reset, removal or re-registration in between wins.
    Hub-local completion uses the same function.
13. Recreate and redeploy are container-scoped runs. Admission excludes a
    parallel run on the same container, a run of its parent stack and a policy
    run on the same target, using the existing scope rules.

## Recovery

14. The executing host keeps `container_recreate_journal`: `run_id`, `old_id`,
    `old_name`, `backup_name`, `new_id`, `new_name`, `was_running`,
    `auto_remove`, `phase` and `updated_at`. Phases: `intent-rename`, `renamed`,
    `intent-create`, `created`, `intent-start`, `started`, `intent-remove-old`,
    `done`, `rolled-back`, `ambiguous`, plus the compensation intents below.
    Every Docker effect is preceded by an intent write and followed by an
    observed write. The replacement is labelled `io.docklet.run=<run_id>` at
    create.
15. Recovery runs at startup before runtime readiness and before admission
    opens. The run stays non-terminal and keeps its exclusion until recovery
    has committed. Observations use exact IDs; "restore old" means renaming
    `old_id` to `old_name` and starting it only if `was_running`.

    | Last phase | Observation | Action | Outcome |
    | --- | --- | --- | --- |
    | intent-rename | `old_id` named `old_name` | none | failed `container.recreate-recovered` |
    | intent-rename or renamed | `old_id` named `backup_name`, no `new_id` | restore old | failed `container.recreate-recovered` |
    | intent-create or created | `new_id` unknown | find the container named `new_name`, created after `updated_at`, labelled with the run; found → use it as `new_id`; none → restore old | failed `container.recreate-recovered` |
    | created or intent-start | `new_id` not running | remove exactly `new_id`, restore old | failed `container.recreate-recovered` |
    | intent-start or started | `new_id` running under `new_name` | continue as started | next row |
    | started or intent-remove-old | `old_id` exists | remove `old_id` | succeeded |
    | intent-remove-old | `old_id` gone | none | succeeded |
    | any | `was_running=false` and the replacement created but deliberately not started | keep it stopped, remove old | succeeded |
    | any | AutoRemove original gone, ID or name mismatch, a foreign container on a name, or both IDs missing | touch nothing | `ambiguous`, failed `container.recreate-ambiguous`, backup shown with a marker |

    Compensation steps have their own intents (`comp-remove-new`,
    `comp-rename-old`, `comp-start-old`) and are re-evaluated idempotently by ID
    after an interruption. A compensation effect that fails moves the journal to
    `ambiguous` and the run settles failed; the exclusion is released only with
    that settlement. Recovery never repeats a pull or create. Journals stay
    until resolved; resolved rows are pruned after 7 days. AutoRemove containers
    interrupted mid-recreate end `ambiguous` and need manual cleanup (accepted
    limit).

## Web

16. Unraid badge and filter, ring segment toggle, assignment dialog with the
    allowed targets, confirm action for `unconfirmed`, reset, and a recreate and
    edit dialog with the standalone warning. Disabled actions show the lock
    reason from the capability descriptor.

Codes and shapes: `container.origin-locked`, `container.recreate-recovered`,
`container.recreate-ambiguous`, `runtime.idempotency-mismatch`, the origin
enums, recreate options and result.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
| --- | --- | --- | --- | --- |
| OR1 | Unraid detection | key presence including an empty value; complete Compose wins | backend mapper | empty-value label → unraid; complete Compose plus Unraid label → compose (mutation: truthiness or order) |
| OR2 | migration 014 | see FND1 | migration | FND1 |
| OR3 | identity change outside Docklet | `unconfirmed` plus detected origin | overlay on hub-local lists and proxied Direct/Edge list and detail | real proxied response through the inventory middleware with a new ID (mutation: name-only match) |
| OR4 | Docklet recreate keeps the override | conditional update in the terminal projection | remote result → hub projection transaction | real Direct and Edge round trip including a rename (mutation: skip the update) |
| OR5 | admission lock | 409 from fresh inspect plus override | public intake on all transports | per origin (mutation: retained snapshot or UI-only lock) |
| OR6 | idempotency | canonical edit digest | intake and receiver | map-order equivalence, absent versus empty, changed edit (mutation: digest without edit) |
| OR7 | crash recovery | decision table before readiness, exclusion held until settlement | executor startup | separate process lifetimes, one case per table row including a crash during compensation and a failing compensation, each asserting the exact Docker state, outcome code and exclusion (mutation: name-pattern cleanup, release before recovery, or treat intent-start with a running replacement as failed) |
| OR8 | Direct/Edge parity | runtime sender and receiver with body and result | remote runtime path | real Direct and Edge tests (mutation: drop the result) |
| OR9 | edit bounds | strict, at most 64 KiB, both intakes | hub intake and receiver | unknown field, oversize, nested invalid → refused at the receiver too (mutation: receiver skips validation) |
| OR10 | effect-boundary guard | Compose, Unraid and self refused at the service | `RedeployContainer` through the webhook producer | webhook-triggered Compose-child redeploy refused without effect (mutation: remove the service guard) |
| OR11 | codes and shapes | listed above | types, OpenAPI, v11 fixture, EN/DE | catalogue and fixture tests |
| OR12 | host removal and re-registration | overrides deleted; a late result cannot resurrect one | delete and projection | remove, re-register the same name, deliver the old result → no override (mutation: unconditional upsert) |
| OR13 | replacement result retention | compact receipt result, query, retry, navigation | executor receipt → Direct/Edge query → hub projection | lost terminal response, activity history cleared on executor and hub, store reopened, then real query and retry on the removed old ID: same run, all four identity fields, navigation target, no second effect (mutation: result only in activity metadata, or omitted from the compact projection) |
| OR14 | no replay of the original effect | recovery compensates only | startup | effect counters show no second pull or create |
| OR15 | partial-label unknown | standalone not offered, edit locked | assign API and guard | assigning standalone → 409 (mutation: allow) |
| OR16 | concurrent assign, reset and recreate | conditional writes | assign/reset API and projection | race tests; the final row matches the last committed intent |
| OR17 | recreate concurrency | container-scoped run excludes parallel container, parent stack and policy runs | real admission → receiver with effects held | hold a recreate effect; start a manual restart, a parent stack run and a policy update on the same container → each refused or queued per the existing scope rules, no second effect (mutation: skip the exclusion for the new operations) |
