<!--
Source: docklet-next/issues/349
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original bulk actions from the stack and container lists rationale.
-->

# Bulk actions from the stack and container lists

Decision for #349 (M19, maintainer kickoff 2026-11-06). Docklet is a homelab tool with normally one
administrator. Bulk stays a client-side fan-out over the existing single-target commands: every item
is admitted by the hub on its own, so conflict scopes, rights, origin locks, Direct/Edge dispatch and
idempotency are those of the single action.

## Accepted decisions (do not reopen)

- The approved interface applies: selection in the Advanced view and a docked action bar with
  "N selected", Clear and Esc.
- Stacks: Start, Stop, Restart, Redeploy, Updates. Containers: Start, Stop, Restart.
- No bulk backend endpoint; no lifecycle actions beyond these.

## Starting point (main b5068aff)

- `web/src/shared/free/bulk-action-bar.tsx` is live; the stack actions are wired in
  `features/stacks/overview/StacksPage.tsx:266-287`, the container actions in
  `features/containers/ContainersPage.tsx:265-276`.
- The handler fires one single command per selected ID and clears the selection
  (`StacksPage.tsx:119-122`). Each call goes through the adapter command, the client run gate and the
  hub's runtime admission (one admitted run per target, [runtime control](runtime-control.md)).
- Defects:
  1. Bulk "Updates" in HTTP mode dispatches a window event per stack; the single update dialog keeps
     only the last one (`use-stack-commands.ts:61`, `stack-update-dialog.tsx:62`).
  2. The hub-ref capability descriptor carries no assigned project, so user bulk controls are
     disabled today; the per-stack `runStackCommand` descriptor is decided as `deploy-project` for
     every command and cannot tell Restricted from Full for Redeploy.
  3. Failures are one toast each, mixed with successes; there is no per-item result, and busy targets
     are swallowed by the gate (`use-stack-commands.ts:26`).
- Arcane's `system/containers/start-all|stop-all` are not admitted runs, Admin-only and unused; they
  stay untouched.

## Design

### Capability per action

- The capability operation list gains `redeployStack` in the producer (`activity/capabilities.go`).
  Users: `access.Decide("redeploy-project")` in `userCapabilityOperations`; administrators: the
  `runStackCommand` grant rule. `installStackUpdates` per stack already maps to
  `update-project-images` (Full).
- The single Redeploy button and the bulk preflight read `redeployStack`; Start, Stop and Restart
  keep `runStackCommand`. Preflight runs per selected item with `useCapability(op, {kind, id})`
  (queries cached by key).
- An unavailable item is `skipped` with the descriptor reason (`access.full-required`,
  `container.origin-locked`, `inventory.stale`, host offline); no request is sent for it. A button is
  enabled when at least one selected item is available, and its title says how many are skipped.

### Fan-out

- Hook `useBulkRun` (`web/src/shared/hooks/use-bulk-run.ts`) takes the selection snapshot (IDs with
  host and display name captured at click time), the action and a per-item sender. At most 4 items
  are in flight; each adapter promise is awaited until the run is admitted or refused. One result per
  item: `started`, `joined`, `skipped` (reason code) or `failed` (code).
- Admission is the boundary: the hook does not wait for runs to finish. Progress stays in the normal
  toasts, action dialog queue and activity centre.
- `runtime-commands.ts` `command()` distinguishes 202 `run.started` from 200 `run.joined` but returns
  only the run ID. An internal `admit(target, command, {quiet})` → `{outcome: "started" | "joined",
  runId}` serves the bulk sender, exposed as one optional adapter method `admitRuntimeCommand` (HTTP
  and demo). The public methods keep returning the run ID. A local gate join returns `joined`;
  `quiet` routes gate busy and join notices to the summary instead of toasts. A busy target is
  `failed: run.busy`.
- The selection is cleared when the fan-out starts; the summary keeps its own copy, so navigation or
  a list refetch cannot relabel results.
- Only the browser identity lifetime stops pending items (`failed: session.ended`). Host credential
  rotation is a per-host refusal for the affected items.

### Bulk update for stacks

- "Updates" opens one `BulkUpdateDialog` instead of the single dialog.
- Eligibility comes from the stack's update policy read (`getAutoUpdatePolicy`: `candidates.length >
  0`, not stale, no `code`), not from `getUpdateBaseline.available`, which only says that the
  deployed baseline matches the saved configuration. The baseline is read (at most 4 in parallel)
  only for `savedRevision` and must be available.
- Results: no candidates → skipped "up to date"; stale or code → skipped "not checked"; baseline
  unavailable → skipped with its code (for example deploy first). "Install" sends
  `installStackUpdates(stackId, {expectedRevision: baseline.savedRevision})` per listed stack through
  the fan-out. A refusal at admission (revision changed, typed update conflict) is `failed` for that
  stack only. Whole-stack updates only; single-service updates stay on the stack page.

### Summary, rights and containers

- One toast per bulk action ("Start: 3 started, 1 skipped, 1 failed") with "Details" opening a dialog
  that lists item, host and result text from the catalogue. Partial failure never retries
  automatically; the user re-selects and repeats.
- Restricted users: Start, Stop and Restart on assigned stacks; Redeploy and Updates are skipped with
  `access.full-required`. The container list is admin-only, so container bulk actions never appear
  for users. Start, Stop and Restart are allowed for Unraid and uncertain origins, as single actions.
- Container Restart keeps the single-action confirmation for containers in a Compose stack
  (`useContainerRestart`); in bulk it is asked once for all such containers and the restart goes
  through the explicit bulk sender so outcomes are collected.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| Partial failure: one item 409 busy, one 503 host offline, one ok | each item its own result | hook test with a fake adapter honouring per-item promises; red proof: `Promise.all` short-circuit → red |
| Late responses after clearing or refetch | results keyed by the captured snapshot | refetch with a renamed stack before the slow item settles; summary keeps the captured name |
| Identity change during fan-out | pending items `failed: session.ended` | test with the `identityLifetime` signal; red proof: ignore the signal → red |
| Restricted user | preflight skip, no request | adapter call count = 0 for skipped items; rights test with a Restricted principal |
| Per-action capability | `redeployStack` decided by `redeploy-project` for users | backend capability test: Restricted stack → Start available, Redeploy `access.full-required`; Full → both; red proof: map to `deploy-project` → red; web: zero Redeploy requests for the Restricted item |
| Admission outcome | 202 → started, 200 → joined, local join → joined, busy → failed; one summary, no gate toast | real `runtime-commands` against a fetch double with the real response shapes; red proof: collapse joined to started or let quiet notices toast → red |
| Bulk update eligibility | policy candidates decide, baseline only gives the revision | stacks current, with candidate, unreadable policy, plus one revision refusal; red proof: select by `baseline.available` → red |
| Concurrency cap | ≤ 4 in flight | blocking adapter records max concurrency; red proof: unlimited → red |
| Direct/Edge parity | unchanged single-target path | practical acceptance with stacks of all host kinds in one selection |
| Catalogue | summary labels in EN/DE | catalogue test through the wire-code guard |

## Contract

- No new route, migration or agent contract change. The hub's capability operation list gains
  `redeployStack`; the adapter gains the optional `admitRuntimeCommand`.
- Summary codes (`run.busy`, `session.ended`, skip reasons) are existing codes or web-local labels
  with EN/DE catalogue text; nothing new on the wire.
