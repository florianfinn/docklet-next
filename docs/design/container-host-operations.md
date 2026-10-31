# Container and host operations: shared foundation

M14 completes the container and host actions of the approved React reference on
hub-local, Direct and Edge hosts and adds the container shell. Five area notes
hold the behaviour; this note holds what they share: migration 014, agent
contract v11 and their ownership.

| Area | Note |
| --- | --- |
| Container shell | [container-shell.md](container-shell.md) |
| Container origin, recreate and edit | [container-origin-recreate.md](container-origin-recreate.md) |
| Stack stop mode, stopped stacks, single service restart | [stack-stop-service.md](stack-stop-service.md) |
| Host maintenance and installation files | [host-maintenance.md](host-maintenance.md) |
| Truthful host availability | [host-availability.md](host-availability.md) |

The selectable hub time zone moved to M18 together with the schedule rework.
Users and resource rights (M15), container updates (M18), bulk actions (M19),
adopting external Compose projects and Swarm are outside M14.

## Ownership

One step owns migration 014, agent contract v11, the OpenAPI export and the
generated DTOs, including the shapes that the shell, host maintenance and
availability steps implement later. Those steps add behaviour against the merged
contract and never extend it on their own. The integrated v11 fixture and
fingerprint are verified when the foundation lands and again on the final
integrated head.

## Migration 014

Forward-only; the same file runs on hub and agent databases. Files up to 013
stay unchanged.

1. `runtime_receipts` is rebuilt, because SQLite cannot alter a CHECK
   constraint. The rebuild follows migration 007: a temporary copy of
   `runtime_key_bindings`, a new receipts table with the extended CHECK,
   `INSERT … SELECT` of every row, rename, recreation of every index of 007
   (`idx_runtime_receipts_environment_run`, `_active_scope`, `_active_parent`,
   `_retention`) and restoration of the bindings with
   `idx_runtime_key_bindings_run`. New operations: `recreate-container` and
   `redeploy-container` (container targets only) and `stop-project` (stack
   targets only). The CHECK keeps rejecting invalid target/operation pairs. A
   nullable `result` column stores the compact typed run result (for example
   the recreate replacement identity); it survives activity history pruning.
2. `container_recreate_journal` on the executing host
   ([recovery](container-origin-recreate.md#recovery)).
3. `container_origin_overrides` on the hub; present but unused in agent databases.
4. `environments.credential_revision` (integer, default 0, incremented with every
   token, key or endpoint change) and `environments.color` (nullable lowercase
   `#rrggbb`, enforced by CHECK).
5. `agent_restart_requests` on the hub
   ([restart](host-maintenance.md#restart-agent)).
6. Schema registry, integrity manifest and migration hashes are updated.

## Agent contract v11

`AgentHubVersion` and `MinimumAgentVersion` become 11 with a new fingerprint.
Version 10 agents are update-required: the existing gate forwards no ordinary
request to them, only the documented recovery and upgrade routes. The contract
contains the new runtime operations with their option and result shapes, the
Edge allowlist entry `system.agent.restart`, `ws_start` validation and frame
protocol v1 with close-reason transport for the shell, typed agent refusal
problems and the `/health` `instanceId`.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
| --- | --- | --- | --- | --- |
| FND1 | 014 rebuild preserves receipts, bindings and indexes and admits the new operations | migration resources → initializer → admission and store | schema-013 hub and agent databases seeded with live, unknown and terminal rows → migrate → reopen → admit each new operation; mutation: keep the old CHECK or drop a binding |
| FND2 | one v11 boundary, no v10 fallback for ordinary requests | contract fixture → compatibility gate → public request on Direct and Edge | v11 passes; v10 and a mismatched fingerprint are update-required with zero forwarded ordinary requests; mutation: omit one minimum check |
