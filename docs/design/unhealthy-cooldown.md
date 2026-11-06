<!--
Source: docklet-next/issues/352
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original container unhealthy notice, Auto Heal cooldown and validity hint rationale.
-->

# "Container unhealthy" notice, Auto Heal cooldown and validity hint

Decision for #352, #353, #300 and #299 (M19, maintainer kickoff 2026-11-06). Docklet is a homelab tool
with normally one administrator. Extends [Auto Heal](auto-heal.md) and
[runtime history](runtime-history.md).

## Accepted decisions (do not reopen)

- "Container unhealthy" covers Docker healthcheck failures and restart loops.
- A notice is sent only after the container has been unhealthy continuously for 10 minutes, and at
  most once per container per hour. Both are constants, not settings.
- The notice is independent of Auto Heal settings and opt-in per target.
- Auto Heal cooldown: 1–60 min, default 10, per host next to the window limit (Auto Heal stays per
  host).
- The agent contract moves to v15 (protocol break, announced in the closing PR).

## Starting point (main b5068aff)

- Auto Heal runs on each executing host (`pkg/scheduler/auto_heal_job.go`, default every 5 min). It
  re-inspects each unhealthy candidate, checks exclusions and ownership, then `Store.Reserve`
  (`internal/autoheal/store.go:61-101`) counts durable `auto_heal_restarts` rows inside
  `WindowMinutes` and refuses at `MaxRestarts` with one `limit-reached` marker per window. No cooldown
  exists.
- `AutohealSettings{enabled, maxRestarts 1-10, windowMinutes 5-1440, excludedStacks,
  excludedContainers, managedByEnvironment}` (`types/autoheal/settings.go`) is pinned in agent
  contract v14 (`autoHeal.settings`, `autoHeal.update`; Edge commands `auto_heal.settings.get|update`).
  PUT rejects unknown members, so a v14 agent refuses any new field.
- Out-of-contract effective values (environment-managed hosts) are skipped and logged; the GET returns
  the raw values and the web parser throws, so the host row shows "Unavailable".
- The hub samples every host each minute (`history_job.go`; `history.sample` on agents) and
  `projectTransitions` (`internal/history/projection.go:45-83`) writes
  `container.condition.unhealthy|down|restart-loop|running-again` events. The previous condition per
  container lives in memory only (`hostState.conditions`), so a hub restart re-emits the condition
  events of every currently unhealthy container. Nothing reaches the outbox; `sustained_load`
  (`projection.go:87-149`) is the existing history → outbox model.
- `config.Load()` is called per stack terminal transaction (`activity/runtime_notifications.go:21`),
  per Auto Heal candidate under an errgroup (`settings/autoheal.go:24`) and in
  `settings/service_loading.go:27-36`; it rewrites the `utils.FilePerm/DirPerm` globals (#299).

## Design

### Notice (#352)

- Producer: the hub's history projection, for every host (hub-local, Direct, Edge) through the
  existing sampler.
- Covered reasons are those of the history classifier: `Reason=health` (Docker healthcheck) and
  `Reason=restart-loop` (restarting state or ≥ 3 restarts in 10 min).
- Minimum duration: the notice is sent once `now - unhealthy_since ≥ 10 min`. `unhealthy_since` is
  the earliest `Since` of the unbroken unhealthy period, persisted, so it survives hub restarts; a
  change between the two reasons keeps it, a healthy sample clears it. Recovery before 10 minutes
  sends nothing.
- Quiet period: at most one notice per container per 60 minutes (`unhealthy_notified_at`, not cleared
  by healthy samples).
- The check runs in the projection on every successful sample, not only on transitions, so the notice
  fires in the first sample after the 10 minutes elapse. Outbox event `container_unhealthy`, dedup key
  `unhealthy:<host>:<container>:<unhealthy_since unix>`, payload host, container, stack, reason and
  duration facts. `unhealthy_notified_at` is written in the same statement batch as the outbox insert.
- The event is opt-in per target (in the opt-in list next to `sustained_load`), so existing targets do
  not start receiving it after the upgrade. Texts are in
  [notification targets](notification-targets.md).

### Persisted condition state (migration 019)

```
history_conditions(host_id TEXT, kind TEXT CHECK (kind IN ('container','stack')), entity_id TEXT,
  revision TEXT NOT NULL, condition TEXT NOT NULL,        -- JSON of dto.Condition
  restarts TEXT NOT NULL DEFAULT '[]',                    -- JSON [{minute,count}], 10-min window
  unhealthy_since INTEGER NULL, unhealthy_notified_at INTEGER NULL,
  PRIMARY KEY (host_id, kind, entity_id)) WITHOUT ROWID
```

- The persisted state carries the classifier's inputs, because `observations` derives `Since` from
  `old.conditions` and the restart window from `old.restarts`. On `commit`, when the in-memory
  `hostState` is missing (process start), the hub loads the host's rows whose `revision` equals the
  current host revision into `old.conditions`, `old.stacks` and `old.restarts` before `observations`
  runs. Seeding also stops the duplicate `container.condition.*` events after a restart, and a
  counter-derived `restart-loop` stays `restart-loop` (no false `running-again`).
- A host revision change keeps today's reset semantics: rows of another revision are ignored for
  conditions and restarts (and rewritten); only `unhealthy_notified_at` survives, by
  `(host_id, container_id)`.
- The full `dto.Condition` (kind, container ID, name, project ID, condition, reason, exit code,
  restarts10m, since) is stored, so stack association and display facts survive an inspection gap.
  Rows that fail to decode count as no prior state.
- Writes happen in the same transaction as events and outbox rows, only for rows whose condition,
  since or restart evidence changed. Rows in `sample.Uninspected` are kept untouched; rows absent from
  both the inspected containers and `Uninspected` of a successful sample are deleted, as are stack
  rows absent from `sample.Projects`. A failed fetch writes nothing. Rows of removed hosts go with
  the host (environment delete path) and, for unknown host IDs, in the history retention job.
- Migration 019 also rebuilds the outbox with `container_unhealthy` in its CHECK list (as 016 did).
  Migrations run on agent databases too; there the table and rebuild are unused. `types/notification`
  `EventType` and `EventMap`, the web `EVENT_KEYS` and `OPT_IN_KEYS` and the EN/DE labels follow.

### Auto Heal cooldown (#353)

- Per-host setting `autoHealCooldownMinutes`, 1–60, default 10; environment name
  `AUTO_HEAL_COOLDOWN_MINUTES` with the existing managed-by-environment rules. Generic settings writes
  reject it together with the existing Auto Heal keys (`settings/handler.go:315`,
  `settings/autoheal.go:20,35`).
- `Store.Reserve` first reads the newest `auto_heal_restarts` row of the container ID with outcome
  `attempting|restarted|restart-failed|interrupted`. If its `started_at` is inside the cooldown, the
  candidate is skipped: no row, no marker, no notice, one debug log. Then the window count applies as
  before. Cooldown and count run in the same `BEGIN IMMEDIATE` transaction, measured from the last
  attempt's start. A recreated container (new ID) is a new identity: no cooldown, window as before.
- DTO: `AutohealSettings.cooldownMinutes` (required in GET) and `AutohealUpdate.cooldownMinutes`
  (optional in PUT: an omitted value keeps the stored one).
- Web: slider "Pause after restart" 1–60 min in the Auto Heal dialog; the host row summary shows it.

### Validity hint (#300)

- GET adds `invalidFields: ("maxRestarts"|"windowMinutes"|"cooldownMinutes"|"excludedStacks"|
  "excludedContainers")[]`, computed by the same validator the job uses (one function for both).
- The web parser accepts out-of-range values only for fields listed there and shows a read-only hint
  on the host row: "Auto Heal is paused on this host: {fields} outside the allowed range" (plus "set
  by environment" when managed). Direct and Edge hosts use the same proxied GET. Invalid values keep
  pausing healing; the hint never permits a restart.

### #299

`*config.Config` is injected into the activity runtime notifier, the settings service (Auto Heal and
loading paths) and the backup service. No `config.Load()` remains on these paths (grep guard in a
test); `go test -race` with concurrent Auto Heal candidates.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| Hub restart while a container is unhealthy | seed from `history_conditions` before classification; no second notice, no duplicate condition event | persist a stack-associated unhealthy container; new Service; first sample lists it only in `Uninspected`; later inspected → same Since, project and name, nothing new; truly absent → row deleted; red proof: seed only `projectTransitions` or drop uninspected rows → red |
| Counter-derived restart-loop | restart points persisted | three restarts while running, restart Service, unchanged Docker state → no `running-again`, Since kept; red proof: reset restart points → red |
| Minimum duration | 9 min → no notice, 10 min → notice | injected clock; red proof: notify on transition → red |
| Clock across restart and reason switch | `unhealthy_since` persisted, kept across a reason change | restart at minute 7 → notice at minute 10 from the persisted value; healthy then unhealthy again restarts the clock; red proof: clock from process start → red |
| Quiet period | second notice within 60 min suppressed; after 60 min allowed; survives healthy samples and restarts | notice, healthy, restart, unhealthy within 60 min → none; red proof: clear on healthy upsert → red |
| Restart-loop reason | own text | restart-loop notice case |
| Host revision change or re-pair | state reseeded, no duplicate | test with revision change |
| Shared read results | projection reads the sample decoded for this host; condition maps copied before mutation | `-race` test with concurrent samplers on the real `Producer` |
| Real Direct/Edge path | sampler → projection → `history_conditions` → outbox → receiver, then service restart | extension of the real harness (`internal/history/transport_test.go`) with an Edge-shaped sample; red proof: skip the outbox insert → red |
| Migration 019 | forward-only, outbox rebuild keeps rows, both node roles | migration tests with M18-schema fixtures for hub and agent |
| Opt-in | existing targets without the key do not receive it | subscription test; red proof: not in opt-in list → red |
| Cooldown race (scheduled + recovery run) | count and cooldown in one transaction | concurrent Reserve test; red proof: cooldown read outside the transaction → red |
| Recreated container | cooldown not applied | test documents it |
| v14 agent with v15 hub | refused by the agent-access envelope (`agent.update-required`) | contract test on the minimum version |
| v15 settings through both transports | `cooldownMinutes` and `invalidFields` survive proxy and Edge allowlist; web renders the host row from the response | round-trip test through the real Direct proxy and Edge `auto_heal.settings.*` path; red proof: omit the field in the Edge mapping → red |
| Validity hint | invalid field listed, hint instead of "Unavailable" | producer fixture + parser tests on Direct and Edge shapes |
| #299 | no `config.Load()` on the paths | `-race` test + grep assertion |

## Contract

- `AutohealSettings.cooldownMinutes`, `AutohealUpdate.cooldownMinutes`, `invalidFields` change the
  pinned `autoHeal` schemas: agent contract v15, minimum agent version 15, fixture
  `agent-hub-v15.json` and fingerprint. Hub and agents share a version.
- Migration 019: `history_conditions` and the outbox CHECK rebuild (shared with
  [notification targets](notification-targets.md)).
- Outbox event `container_unhealthy`; EN/DE labels and codes through the wire-code guard.
- Setting `autoHealCooldownMinutes` / `AUTO_HEAL_COOLDOWN_MINUTES`.
