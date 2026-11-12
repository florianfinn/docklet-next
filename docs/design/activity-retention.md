<!--
Source: docklet-next/issues/355
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original configurable activity retention and event order rationale.
-->

# Configurable activity retention and event order

Decision for #355 and #303 (M19, maintainer kickoff 2026-11-06). Docklet is a homelab tool with
normally one administrator. Reverses the removal of configurable limits in [run gate](run-gate.md).

## Accepted decisions (do not reopen)

- Event and finished-run limits are configurable and hub-wide (one value for all hosts).
- Agents receive the limits when they are saved and again when an agent comes online.
- Active (queued or running) runs are never counted or pruned.

## Starting point (main b5068aff)

- One job prunes both on every node, agents included (`pkg/scheduler/event_cleanup_job.go:46-73`,
  registered without an agent guard in `bootstrap/jobs_bootstrap.go:310-321`), every 6 h by default:
  - events: `PruneEvents(ctx, 500)` keeps the newest 500 across the hub (`event/retention.go:35-49`),
    although the settings note says "per environment" (`settings/messages/en.ts:454`);
  - finished runs: `PruneHistory(ctx, 100)` keeps 100 per environment and ranks only terminal
    statuses (`activity/service_projection.go:301-337`).
- Agents forward their events to the hub (the hub copy is pruned by the hub) but keep their own runs;
  the hub reads remote run lists live from the agent (`service_projection.go:565-600`), so an agent's
  own run limit decides what the hub can show for that host.
- The web domain has the limits (`domain/activity-history.ts:10-13`: events 50–5000, default 500;
  finished runs 10–1000, default 100) and the click dummy has two number fields in Settings › System ›
  Activity; the live section shows only a note.
- Only timeout keys are pushed to agents, on change, best effort
  (`syncTimeoutSettingsToAgentsInternal`, `jobs_bootstrap.go:559-640`).
- `ListEventsPaginated` sets no order and pagination appends `id ASC`; IDs are random UUIDs, so the
  default API order looks random (#303). The web already sends `sort=timestamp&order=desc`, but the
  tie-breaker stays `id ASC`.

## Design

### Retention

- Settings keys `activityEventLimit` (50–5000, default 500) and `activityRunLimit` (10–1000, default
  100), typed validation in `setting_validation.go`.
- Meaning as implemented: the event limit counts all hub events together; the run limit counts
  finished runs per host. The settings text says so.
- The cleanup job reads both keys on each run. A change of either key triggers one prune run on that
  node after the save (settings subscription), so "keeps at most N" holds at once. Subscribers can
  finish after the save response (`settings/service_updates.go:82`), so the prune is an effect that
  tests wait for, not the HTTP 200.
- The prune statement stays one statement (`event/retention.go:39`), so rows inserted after its
  snapshot survive.
- Agents: both keys join the agent settings sync (the timeout sync renamed; same worker, best-effort
  semantics). The synced set is `AgentSyncedSettingKeys()` = the keys of
  `libarcane.TimeoutSettingKeys()` (timeouts and Trivy resources) + the two retention keys; the
  hub-only vulnerability keys are asserted absent. The current synced keys are also sent when a remote
  host comes online again (the existing host-availability transition to online), so a limit saved
  while an agent was offline converges without another edit.
- Environment overrides follow the existing settings loader; a hub with UI configuration disabled
  shows the values read-only. An environment override on an agent wins on that agent.
- Web: two number fields in Settings › System › Activity with range hints and the existing
  `validateHistoryLimit`; the Activity Centre note reads the configured values.

### Event order (#303)

Event list queries without an explicit sort order by `timestamp DESC, id DESC`; with `sort=timestamp`
the tie-breaker follows the requested direction. This applies to `ListEventsPaginated` and
`GetEventsByEnvironmentPaginated` (`event/service.go:284,315`); the shared pagination helper keeps its
behaviour for other lists.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| Active runs under a low limit | never pruned | repository test: 15 finished + 3 running, limit 10 → 10 + 3 remain; red proof: count all statuses → red |
| Limit change applies at once | prune after save on that node | settings subscription test waiting for the prune effect; red proof: no trigger → red |
| Range validation | typed 400 for out-of-range or non-integer | validation table test |
| Agent sync key set | retention keys in, hub-only vulnerability keys out | key-set test (shared with the notification note's #270 item) |
| Agent offline during save | resent when the host comes online | agent offline during save, online transition → effective value updated and its prune ran; red proof: no resend → red |
| Agent override | environment override wins on the agent | separate case; other cases use values not masked by local overrides |
| Real transport | sync PUT through the real Direct proxy and Edge path | httptest agent + Edge event path test |
| Event order | IDs not in time order → newest first | repository test with crafted IDs, no-sort and equal-timestamp pages in both directions, both list functions; red proof: drop ORDER → red |
| Concurrent prune vs new events | rows inserted after the snapshot survive | existing retention tests extended with an insert in a competing transaction before and after the prune commits; no hook inside the statement |

## Contract

- Settings keys `activityEventLimit` and `activityRunLimit` (generic settings route; key/value rows,
  no migration, no agent contract change).
- Code `settings.activity-limit-invalid` (400, with the field name), EN/DE text.
- [Run gate](run-gate.md) and [adapter, live channel and capabilities](adapter-live-capabilities.md)
  state this rule.
