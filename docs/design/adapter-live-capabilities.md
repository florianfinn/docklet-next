# HTTP adapter, live channel and capabilities

## Boundary and reuse

The production web app currently selects `createDisconnectedAdapter` in
`web/src/runtime.ts`; development selects `createDemoAdapter`. The existing
`DockletAdapter` port in `web/src/data/adapter.ts` and `api.ts` defines calls and
`Topic` names. `live.ts` shares action, metric and log subscriptions;
`AdapterProvider.tsx` maps `subscribeChanges` topics to query invalidation;
`run-gate.ts` deduplicates local starts. The disconnected adapter rejects calls
and returns no invented data. The HTTP adapter belongs in `web/src/data/http/`
and keeps these boundaries rather than making features import wire DTOs.

Arcane v2.15.1 already has the multiplexed `GET /stream` SSE operation
`streamClient` for `activities` and `events`. Activity envelopes carry run
snapshots or messages; event envelopes signal a changed event query. The
adopted `backend/api/handlers/stream.go` and `types/stream/` provide this path.
Arcane's `backend/api/ws/` and `backend/pkg/libarcane/ws/` instead serve
per-resource WebSocket logs and stats. Reuse those WebSocket routes when their
adapter methods gain live domain mappings; do not duplicate them for M3 run
invalidation. Their Echo routes are absent from OpenAPI.

## HTTP calls and outcomes

`api/openapi.json` and `web/src/api/generated/schema.ts` are the checked-in
operation and DTO contract. There is no generated runtime client yet. A small
HTTP caller in the adapter uses generated `paths`/`operations` types, then maps
wire DTOs to the existing view models. Route presence alone does not make an
adapter operation available: its mapping and server capability must both exist.

| Adapter call or family | Current OpenAPI operation | M3 handling |
| --- | --- | --- |
| `listActions`, run refresh | `list-activities`, `get-activity` per environment | Map recorded activity, message and step DTOs; enable the full `CodedActionRun` projection only where required fields have wire sources. |
| `listEvents` | `listEvents` or `getEventsByEnvironment` for the selected scope | Map `EventEvent` to `CodedEvent`; use recorded DTOs. |
| `subscribeActions`, `subscribeChanges` | `streamClient` (`activities`, `events`) | Parse envelopes and re-read changed snapshots/queries. |
| `listHosts`, `listStacks`, `getStack`, `listContainers` | `listEnvironments`, `list-projects`, `get-project`, `list-containers` | Expose unavailable until M5 owns the view-model mappings. |
| `getSecurity`, `listSchedule`, `getSettings` | `get-environment-vulnerability-overview`, `get-job-schedules`, `get-settings` | Expose unavailable until M11, M10 and M4 respectively own their mappings. |
| `runStackCommand`, `runContainerCommand` | `deploy-project`/`down-project`/`restart-project`/`redeploy-project`; `start-container`/`stop-container`/`restart-container` | Expose unavailable until M6 supplies guarded run identities and mappings. |
| Other `DockletAdapter` reads, writes and subscriptions | Existing operation where listed, otherwise no matching operation | Expose unavailable until the owning milestone supplies the complete mapping. Existing log/stats WebSockets remain the later stream transport. |

The HTTP boundary classifies an operation into these outcomes. Existing
`Promise<T>` port methods return the `ok` value or reject with the typed
non-`ok` outcome; the UI can use the same shape for error rendering.

| Outcome | Required fields | Meaning |
| --- | --- | --- |
| `ok` | `kind`, `value` | Confirmed server result, including `run.joined` and `run.fresh` when returned by admission. |
| `not-available` | `kind`, `operationId`, `reasonCode`, `resourceRef` | No request is executed; UI renders the action disabled with a localized reason. |
| `conflict` | `kind`, `code: "run.busy" \| "run.cancelling"`, `activeRunId` | Server refused admission; keep the active run visible. |
| `error` | `kind`, `code`, `status` when HTTP responded | Request or mapping failed; never turn it into an empty collection or successful action. |

`run.started`, `run.joined` and `run.fresh` are admission results. A cancelling
run remains `running` until completion. The HTTP adapter never exposes the
server's executable work context. The client-side `run-gate.ts` remains a UI
deduplication aid; server admission decides cross-tab conflicts and freshness.

## Capability descriptor

The server is the source of truth for each operation's descriptor:
`{ operationId, available, disabledReasonCode?, resourceRef: { kind, id } }`.
`operationId` names the adapter operation, and `resourceRef` identifies its hub
or target resource without defining rights. An unavailable descriptor requires
a reason code; an available one has no disabled reason. The current OpenAPI has no descriptor operation,
so #38 must add it and regenerate the contract. The HTTP adapter exposes the
descriptor to the UI, merges it with its implemented mapping, and treats missing
or unreadable entries as unavailable with a generic reason code. UI action controls read that result to set
`disabled` and show a translated reason; direct calls still return
`not-available`. Resource-right evaluation and policy remain M15's decision.

## Live channel and recovery

M3 subscribes only to `activities` (runs) and `events` (query invalidation)
through Arcane's existing `streamClient` SSE transport. A start, progress,
completion or completion message means “snapshot changed”, not guaranteed
delivery. `subscribeActions` re-reads the affected run/list via
`list-activities`/`get-activity`; `subscribeChanges` reports affected existing
`Topic` values, at least `events`, to `AdapterProvider.tsx`. No event is treated
as authoritative state or a replay log.

On first connect, reconnect, malformed envelope or detected event loss, re-read
the subscribed run and event topics, including the active run detail. This
recovers a committed completion whose event was lost in a hub crash. Where an
envelope names a narrower affected topic, invalidate that topic; otherwise
refresh the subscribed M3 topics. Preserve WebSocket logs and stats as separate
later streams. The deployment has one hub process per database, so no
multi-hub fan-out protocol is needed.

## #27 scope and history setting

M3 replaces the production `createDisconnectedAdapter` selection with the HTTP
adapter, which implements run/event reads and the live feed in this step. It removes the empty production value of
`activity.output.simulated`; real run output has no simulation badge. Other
unsupported actions remain visibly disabled with their capability reason.
Login and setup gates and their empty text fallbacks belong to M4; onboarding,
host installer and host text fallbacks belong to M5. Preview and design-lab
routes remain development-only. Later domain text fallbacks follow their
owning milestones in `api-contract.md`.

Remove the editable history limits from the web settings UI: retention is fixed
at 500 events and 100 finished runs per environment; running runs are retained.

## Contract test matrix for #38

Each numbered case runs against the demo adapter and HTTP adapter with the same
scenario; HTTP responses and streams use recorded DTOs, while the demo supplies
the equivalent view-model state. The last column names a mutation that must
make the case fail.

| # | Required observation | Red mutation |
| --- | --- | --- |
| 1 | Recorded activity and event DTOs produce the same coded run/event fields in both adapters, including steps, affected services and message codes. | Drop `affectedServices` or a message code in the HTTP mapper. |
| 2 | An unsupported operation is disabled with the server reason and direct invocation returns `not-available` without a request or run. | Return a synthetic action ID or omit the disabled reason. |
| 3 | A descriptor missing an entry or failing to load leaves that operation unavailable. | Treat missing capability as available. |
| 4 | `run.busy` and `run.cancelling` preserve their code and active run ID; neither starts another action. | Convert a conflict into `ok` or erase `activeRunId`. |
| 5 | A transport or DTO mapping error renders as error, with no empty or successful fallback. | Catch the error and return `[]` or a fake ID. |
| 6 | Start, progress, terminal and completion-message notifications re-read the affected run; event invalidation refreshes the event query. | Trust the stream payload as final state or drop the `events` invalidation. |
| 7 | Reconnect and detected event loss re-read subscribed runs and events, revealing a committed completion that had no delivered event. | Remove reconnect refresh or omit active run detail. |
| 8 | The production route uses the HTTP adapter for M3 data and offers no simulated success or empty simulation label. | Select the demo adapter or restore the empty `activity.output.simulated` value. |
| 9 | Settings expose no editable history limit; the displayed retention is 500 events and 100 finished runs, preserving active runs. | Restore the numeric fields or prune a running run. |
