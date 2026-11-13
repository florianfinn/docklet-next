# M24 scheduled update recovery acceptance — 2026-11-13

The accepted package is main `6e7a179dbad0bf4855b27f4c486242f85151c846`, the
squash merge of the [M24 closing PR #464](https://github.com/florianfinn/docklet-next/pull/464).
Its tree equals the merged feature head `22d7a0c3`, which the last delta run
accepted. The packages were built locally from the exact source by the sandbox
kit; no registry image was used.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head. The Claude main thread planned the milestone, wrote the
design notes, implemented S0 and S3 and made every review correction. Codex
workers built S1, S2, S4, S5, S6 and S7. Each step was reviewed by the other
provider.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #407 #406 #410 #409 | S0 cron validation, typed nested COMPOSE_FILE refusal, time-zone projections, prefixed-tag evidence | Self-check and CI; the early sandbox smoke found two defects (cron 500, a missed update route), fixed before the delta smoke passed. |
| #440 #441 | S1 recovery evidence foundation (migration 003, evidence grammar, wire codes) | R1–R3 failed (evidence bounds, zero-effect refusals, one shape table); round 4 passed. |
| #440 | S2 stability observer | R1 passed; its eight non-blockers were fixed in one correction. |
| #440 #409 | S3 observed container update, compensation, journal recovery, previous-image release | R1 failed (four blockers); round 2 passed. |
| #440 #408 | S4 observed stack update and own-point recovery | R1 passed; two non-blockers fixed, one report count corrected. |
| #440 #441 | S5 recovery notices, failed-candidate record, skip and clear | R1 failed (volume sentence after zero-effect failures); round 2 passed. |
| #440 #411 | S6 web: "Update with check", recovery display, settings and dashboard follow-ups, design note | R1 passed; most non-blockers fixed, the rest in #469. |
| #412 | S7 per-reference digest observations | R1 failed (stopped containers left out), round 2 failed (stored state wording), round 3 passed. |
| #456 | Closing integration (#464) | Round 1 failed (a held run found active again was returned early); round 2 passed. |

## Defects found by acceptance

The candidate run failed three cells; delta run 1 found the cause of the third:

- **A held scheduled run did not converge (AR5).** When the agent was killed during
  the observation window and restarted, the hub kept the run held as transport
  unknown until someone refreshed it, and the window job had already ended with
  `runtime.outcome-unknown`. The scheduled window now re-queries a held run every
  30 s for up to 10 minutes and never resends the update; a run found still
  active is followed to its end. Delta 1 measured 11.4 s from the agent's first
  healthy probe to the hub terminal and 23.9 s from the executor terminal to the
  notice delivery; the target was `update.interrupted`.
- **Run output times used the browser clock (TZ1).** Run output lines showed the
  browser's local time, so a restricted user in another zone saw UTC instead of
  the hub zone. They now use the hub zone that every other formatter uses.
- **Matrix corrections by the maintainer.** TZ1 originally asked for "next check"
  and backup times that restricted users do not have; it now checks a time they
  do see. TAG1's absolute Dashboard count included an unrelated fixture stack
  from an earlier run; the runner archived it reversibly for the count.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
Scheduled cells enabled real policies and triggered their hub-owned window jobs;
failing candidates were private fixture images. Outbound notices were read from a
local webhook sink; UI assertions ran in English and German with Chromium.

| Cell | Behaviour |
| --- | --- |
| AR1 | Failing scheduled stack candidate: whole previous point running again, one notice, next window not blocked (D, E) |
| AR2 | Unhealthy scheduled container candidate: original resumes with the same ID, replacement removed; plain manual update unchanged (D, E) |
| AR3a | Recovery fails after the restore: `update.auto-restore-failed`, no second recovery (D stack, E container) |
| AR3b | Saved files edited during the update: `unavailable` (`restore.files-changed`), edits kept, no recovery write (D, E) |
| AR4 | Healthy scheduled update: full 120 s window, no early success, no notice (D, E) |
| AR5 | Agent process killed during the window: journal safety tail, `update.interrupted`, one notice, no resend (D) |
| MAN1 | "Update with check" from the keyboard menu: container and stack restored, today's manual notice (D, E) |
| SUP1 | A failed candidate is skipped across three windows and a hub restart; another digest proceeds; a manual update clears (D) |
| TAG1 | Two references to one image each get their own result and action; Dashboard counts 2, then 1 (D) |
| TZ1 | Restricted user sees times in the saved zone, not UTC; reset label names the process TZ (H) |
| UI1 | Settings › Updates table, Dashboard schedule rows, "Check all now" toast, disabled recovery reason (H, D) |
| KIT/REG | Kit build, deploy, smoke and regression walk on all three hosts |

## Result

| Package | Result | Notes |
| --- | --- | --- |
| `9112b87d` (candidate) | Failed, 19/22 | AR5, TAG1, TZ1 |
| `9a1e1f33` (delta 1) | Failed, 5/6 | AR5, TAG1 and REG passed; TZ1 failed |
| `22d7a0c3` (delta 2) | **Accepted**, 3/3 | TZ1, kit, REG on H |
| `6e7a179d` (exact main) | **Accepted**, 5/5 | Tree equals `22d7a0c3`; kit, REG on H/D/E and one healthy observed container update |

The first package's other cells are carried: later changes touched only per-reference
checks, the held-run wait and run output times, each rerun by its own cell.

## Limits

- Only process loss of the agent was exercised in the sandbox; hub process loss
  during the window is covered by tests.
- Beyond the 10-minute hold limit a scheduled target stays `runtime.outcome-unknown`;
  refreshing the run and retrying the window job reconciles it.
- Volume data is never restored; the notices say so when a run changed the workload.
- Chromium only; host load was not controlled.

## Open follow-ups

#465 (a nested COMPOSE_FILE stack blocks target resolution for its host), #466
(workspace `.env` read), #468 (CI scanner flake), #469 (update UI polish), #470
(converge manual remote runs when their host returns), #475 (admin backup and
check times in the hub zone).
