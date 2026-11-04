<!--
Source: docklet-next/issues/346
Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
Retrieved: 2026-11-04
License: Apache-2.0
Deviations: Original one schedule rule, hub time zone and manual scans rationale.
-->

# One schedule rule, hub time zone and manual scans

Decision for #346 and #319 (M18, maintainer kickoff 2026-11-04). Docklet is a homelab tool with
normally one administrator; nothing here is secured twice.

Replaces the deferred M14 time-zone plan and keeps its accepted answers: remote hosts keep their
own zone for volume backups, an occurrence that a zone change moves into the past is not caught
up, and hub-local volume backups follow the hub zone.

## Accepted decisions (do not reopen)

- B9 (2026-10-30): a wall-clock time in the spring gap runs one hour later; the autumn fold runs once.
  One rule for scans, backups and updates.
- A10 / #319: the zone is selectable in Settings › General; `TZ` stays the initial value until a
  choice is saved; schedules follow the stored zone without restart.
- A13: scan schedule modes manual / daily / weekly.

## Starting point (main 89f69980)

- Every scheduled job goes through robfig/cron v3.0.1 `SpecSchedule.Next` with the six-field parser
  `pkg/scheduler/schedule/cron.go:35`. Install computes `parsed.Next(now.In(js.location))`
  (`pkg/scheduler/scheduler.go:168-186`); the durable coordinator re-parses on every alarm and
  overrides `spec.Location = a.q.location` (`pkg/scheduler/runs/actors.go:282-321`).
- robfig behaviour: a fixed time inside the spring gap never matches (that day is skipped); in the
  autumn fold minute stepping can match the repeated hour again (fires twice). The live UI says so
  (`settings.liveScans.zone`). The web `zonedTimeToInstant` already moves gap times forward
  (`web/src/domain/schedule.ts:79-95`), so web projection and backend execution disagree today.
- The zone is `TZ` only (`config/config.go:106,415-427`), copied at construction into the coordinator
  (`di/providers.go:141`), JobScheduler (`bootstrap/jobs_bootstrap.go:66`), JobService
  (`job/service.go:99`), SystemService (`system/service.go:119`) and VolumeService
  (`volume/service.go:116`); only `vulnerability/children/query/risk.go:220` reads it per use.
- `Coordinator.Checkpoint` recomputes `NextRun` only when the schedule text changes or at first
  checkpoint (`runs/coordinator.go:173-205`).
- Scans: `vulnerabilityScanEnabled` (default false) plus `vulnerabilityScanSchedule` JSON
  `{mode: daily|weekly, day, time}` (`types/settings/vulnerability_schedule.go:18-44`). The web
  domain already knows `manual` (`web/src/domain/scan-schedule.ts`), the backend does not.
- Hub backup plans store six-field cron (`system/children/backup/scheduling.go`); volume backup plans
  store six-field cron and run in the owning process (hub or agent) (`jobs_bootstrap.go:410-417`).

## Design

1. **One parse entry point.** `schedule.Parse(expr string, loc *time.Location) (cron.Schedule, error)`
   in `pkg/scheduler/schedule` is the only place that turns schedule text into a `cron.Schedule`.
   Scheduler install, coordinator alarm, JobService next-run display, volume backup overview and
   hub backup scheduling and the libarcane settings validator (`pkg/libarcane/settings.go:70-85`)
   call it. A structural test fails on any other call of robfig `Parser.Parse`, `ParseStandard`,
   `NewParser(...).Parse` or `cron.New` in `backend/` outside that package and its tests (closed
   guard criterion: these call forms on any import of `github.com/robfig/cron/v3`, whatever the
   local package name).
2. **Wall-clock rule.** Classification uses the parsed spec, not its spelling: a
   `*cron.SpecSchedule` whose hour bit set covers all 24 hours (`*`, `*/1`, `0-23`, `@hourly`, …)
   is an elapsed-time schedule and keeps robfig's behaviour (availability, delivery, history
   sampling: no fold hour is dropped). Every other `SpecSchedule` (fixed hours, lists, ranges,
   steps such as `*/6`, `@daily`, `@weekly`, `@monthly`, `@yearly`) is a wall-clock schedule.
   `ConstantDelaySchedule` (`@every`) is unchanged. `Next(t)` of a wall-clock schedule:
   - floating wall time: robfig runs on wall-clock values expressed in UTC (no DST there);
   - the search starts at `wall(t) − 3h` (longer than any DST shift), so a gap time whose floating
     value is before `wall(t)` but whose shifted instant is still after `t` is found (
     02:30 at 03:10 CEST on the spring day → 03:30 CEST);
   - each matching wall time `W` maps to one instant: exists once → that instant; in the spring
     gap → `W` read with the offset before the transition (= `W + gap`); in the autumn fold → the
     earlier instant only;
   - mapped instants are **not** monotonic in `W` (a half-hour transition such as
     Australia/Lord_Howe maps 02:15 after 02:30), so the search evaluates every `W` from the
     start until 3 h of floating time past the first `W` whose mapped instant is after `t`, and
     returns the **earliest** mapped instant strictly after `t` (equal instants once); robfig's zero
     time ends the search with zero.
   So the fold never fires a second time, also after a restart inside the fold.
3. **Zone in the coordinator record, not in the text.** The coordinator `QueueRecord` gains
   `Zone string` (the IANA name its `NextRun` was computed in). `Checkpoint(jobID, schedule, zone,
   next)` treats a changed zone like a changed schedule text: generation/sequence bump and new
   `NextRun`; alarms of the old generation are ignored (existing fence `actors.go:269`). The alarm
   computes the following `NextRun` with `record.Zone` instead of `a.q.location`. The schedule
   text stays unprefixed, so JobService's comparison of runtime state with `job.Schedule`
   (`job/service.go:202-208,290-301`) and the text projections are unchanged. An old
   record without `Zone` counts as "zone differs" once at the first checkpoint after upgrade.
4. **Zone setting.** Hub setting `timeZone`: empty = use `TZ`. Accepted: IANA names, `UTC` and the
   current effective value (alias or `Local`); validated with `time.LoadLocation`; invalid → 400
   `settings.invalid-time-zone`. A provider `ZoneProvider.Location()` replaces the five
   construction-time copies, including their nested consumers (prune service, volume backup
   dependencies and their policy children, risk clock). Saving a new value calls
   `JobScheduler.Reinstall(ctx)`, which re-installs every registered hub job with the new zone
   (one pass, serialized with the existing scheduler mutex), and publishes the settings change
   event. Startup installs with the stored zone; because the record keeps its zone, a crash
   between the settings commit and the re-install is repaired at the next start.
5. **Due semantics (accepted in M14).** Queued or running runs continue. Future occurrences use the new
   zone. An occurrence that moves into the past because of the change is not caught up
   (`Checkpoint` with `first=false` sets `NextRun` without a recovery run). Manual runs unaffected.
6. **Ownership.** Hub-owned schedules (update check and windows, scans, hub backups, prune, risk
   snapshot) and hub-local volume backups follow the hub zone. Agents have no `timeZone` setting:
   remote volume backups keep the agent's `TZ` (M12); the backups UI shows the host zone and a hint
   when it differs from the hub zone. Agents receive the wall-clock rule with the shared package.
7. **Manual scans.** `VulnerabilitySchedule.mode` gains `manual` (no day/time). The separate
   `vulnerabilityScanEnabled` switch is removed; migration 018 rewrites the settings rows:
   enabled=false or empty schedule → `{"mode":"manual"}`, enabled=true with a valid schedule → that
   schedule. Manual installs no scan job; "Scan now" stays available.
8. **Web.** General section gets a zone picker (`Intl.supportedValuesOf('timeZone')` + UTC + the
   current value + "Use TZ (<value>)"). `use-hub-time-zone` reads the effective zone from settings
   (never the browser zone). `nextOccurrence` follows the same rule (gap forward, fold once) and both
   sides test the same vectors (`types/settings/testdata/schedule-rule-vectors.json`). The zone hint
   text changes to the new rule in EN/DE wherever it appears.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
|---|---|---|---|---|
| SR1 | spring gap | 02:30 daily, Europe/Berlin 2027-03-28 → 03:30 CEST once; also from t = 03:10 CEST (still ahead), exact boundary t = 03:30, multiple hours/minutes in the gap, exhaustion; Australia/Lord_Howe 2027-10-03 `0 15,30 2 * * *` from 01:59 +10:30 → 02:30 +11 first | `schedule.Parse` and real coordinator alarm | vector test Go+web with exact instants (mutations: robfig `Next` unchanged; search starting at `wall(t)`; first mapped candidate instead of earliest) |
| SR2 | autumn fold | 02:30 on 2026-10-25 runs once at the first pass; restart at 02:15 CET after it ran → next is 2026-10-26 | `schedule.Parse` and real coordinator alarm | Go test through `Coordinator` + actor alarm with injected clock (mutation: drop the "not after t" loop → second run) |
| SR3 | elapsed-time specs | all-hours coverage (`*`, `*/1`, `0-23`, `@hourly`) keeps robfig semantics; minute job still fires in the fold hour; `*/6` and `@daily` follow the wall rule | `schedule.Parse` | equivalent-spelling vectors (mutation: classify by literal `hour != "*"`) |
| SR4 | one entry point | every scheduler uses `schedule.Parse` | structural test over `backend/` | guard test with the closed criterion above (mutation: add a direct `Parser().Parse` call in a job → red) |
| SR5 | zone change, unchanged cron | zone field changes, generation bump, new NextRun in new zone; old alarm ignored; running run continues; no catch-up; an ordinary job-schedule settings save still succeeds afterwards | settings save → `Reinstall` → `Checkpoint` → alarm → handler; JobService `UpdateJobSchedules` | real scheduler+coordinator test with barrier on a running run + settings-save test (mutations: alarm keeps `a.q.location`; Checkpoint ignores zone) |
| SR6 | crash after settings commit | startup install uses stored zone | startup | test: commit zone, skip reinstall, restart scheduler → NextRun in new zone (mutation: install from `TZ`) |
| SR7 | zone input | IANA/UTC/current alias accepted; `Mars/Base` 400; empty → TZ | settings validation | handler test (mutation: silent UTC fallback) |
| SR8 | remote zones | agents keep `TZ`; hub-local volume backups follow hub zone | volume policy registration | H/D/E test: hub zone change moves H next run only (mutation: agents read a hub zone) |
| SR9 | display parity | DTO `timeZone`/`nextRun` come from the same provider and `Parse` as execution, after a live zone save | JobService, volume overview and nested policies, prune cutoff, risk clock | test per consumer after a live save (mutation: cached construction-time location) |
| SR10 | manual scans + migration | enabled=false (with or without stored schedule) and enabled+empty → manual; enabled+valid → schedule; switching to manual removes the installed job | migration 018, settings parser, scheduler | migration test from 017 with each case + job install/remove test (mutation: map enabled=false to daily; keep job on manual) |
| SR11 | web/backend parity | same vectors with known expected instants (incl. SR1/SR3 cases) pass in Go and Vitest | shared JSON vectors | both suites (mutation: web keeps fold twice) |

## Contract

`timeZone` setting and scan `mode=manual` are hub settings (no agent route). Agent contract v14
(owned by the update-policy note) carries the shared package change; no new agent wire field.
New error code `settings.invalid-time-zone` is declared in types/ with EN/DE text.

## Open questions

None; all product decisions are accepted.
