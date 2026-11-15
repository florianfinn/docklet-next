# M25 host operations and Edge correctness acceptance — 2026-11-13

The accepted package is main `9139d8a2a90b5b90f0e11edf8761862688f3c9e2`. It is
the [M25 closing PR #478](https://github.com/florianfinn/docklet-next/pull/478)
(main `8a712aa1`, tree equal to the delta-accepted feature head `8c298b22`) plus
the [acceptance fix PR #481](https://github.com/florianfinn/docklet-next/pull/481).
The sandbox kit built the packages locally from the exact source; no registry
image was used.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head. The Claude main thread planned the milestone and wrote the
design notes. It implemented S1, S2 and S6, integrated every step and made the
corrections after S1, S2, S5 and S6. Codex workers built S3, S4 and S5. Each step
was reviewed by the provider that did not implement it. The acceptance fix #481
was implemented by the main thread and reviewed by Codex.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #372 | S1 host verdict rule (migration 004, one owned status write path, registration fence) | R1 failed (registration revision fence, proof boundaries); the verify round passed. |
| #370 #429 | S2 latest agent restart after a reload, including a busy agent's refusal | R1 failed (three blockers: stale known run, older run masking a newer one, followed seed taken as history); the verify round failed once (reopen after the follow budget). The fix was checked by the closing review. |
| #366 #369 | S3 replacement exclusion and Docklet protected-self | R1 passed; N3 was checked by integrated lint, and N1, N2, N4 and N5 went to #479. |
| #416 | S4 ordered Edge tunnel delivery | R1 passed; one non-blocker went to #479, one was accepted, and an Activity fixture race went to #480. |
| #379 | S5 Edge container log download | R1 passed; all three non-blockers were corrected and checked by the closing review. |
| #477 | S6 container page by name and tab | R1 failed (same-name recreate lost the address); the verify round failed once (encoded names). The fix was checked by the closing review. |
| #457 | Closing integration (#478) | Passed with no blockers and no new non-blockers, including independent red proof for the four corrections no earlier round had verified. |
| #370 | Acceptance fix #481: initial activity history after an early restart read | R1 failed (two blockers), R2 failed (one blocker), R3 passed; the carried non-blocker is the timing self-test in #468. |

## Defects found by acceptance

- **A reloaded Agent panel lost a Direct agent restart (RL1-D).** After a reload
  during a Direct agent restart, the panel showed no restart and made no latest
  read. The browser's per-host HTTP/1.1 read admission queued the hub-owned
  restart read behind that host's hanging agent reads until its budget ran out.
  The admission predates M25. Restart reads now have their own admission key,
  still within the common five-slot budget. The delta run passed RL1-D and RL1-E
  with the latest read dispatched while the Direct agent was still unavailable.
- **Older failed activities toasted after a reload during a restart (RL1-D on
  exact main).** The restart read published the active run before the first
  activity history arrived, because that history waits for the restarting host.
  The toast consumer took this restart-only publication as its one-time initial
  history, so 50 older failed activities appeared as new toasts, in English and
  German. The delta run had counted only restart toasts. Fix #481 marks targeted
  publications before the first history so they cannot seed it. A single host's
  refresh waits for that first history, and the first history clears the mark,
  even when empty. Its independent review took three rounds. Round 1 failed with
  two blockers: a scoped snapshot still seeded history, and an empty first
  history silenced a later new failure. Round 2 failed with one blocker: a
  waiting refresh was lost when another source timed out. Round 3 passed.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
UI assertions ran in English and German in Chromium. Latency cells were measured
on the hub origin. After an inventory read exceeded its budget in the early
smoke, the candidate run first ran the ownership-checked retention and fixture
prune. It found no eligible objects to remove; image counts were unchanged, and
REG-H passed.

| Cell | Behaviour |
| --- | --- |
| RL1 | Reload during an agent restart: the first latest read is unsettled, the panel follows it to connected and notifies once (D, E) |
| RL2 | Restart refused while a volume backup runs: `host.busy` stays visible with its time after a reload; the backup and a later restart succeed (D) |
| RC1 | Stop on the replacement during an "Update with check" window is refused as busy; the replacement is unchanged and the update succeeds (H, D, E) |
| OS1 | A container with a Docklet system-role label cannot be recreated: 409 `container.origin-locked`, ID and start time unchanged (H, D, E) |
| LD1 | Log download: exact bytes and filename for the administrator and an assigned restricted user; 404 without log data for a foreign container (D, E) |
| LD2 | A log over 64 MiB answers JSON 413 `container.logs-too-large` before any file bytes; a container list during the download stays fast (E) |
| TS1 | A stalled Edge log stream does not slow other requests: 60 list requests within the bound, E online, the stream closed by the delivery bound (E) |
| MG1 | Previous-main data at schema 3 migrates to 4 with statuses, column default and the three triggers intact (H) |
| CL1 | `/containers/<host>~<name>?tab=terminal` opens the terminal; ID links unchanged; the same name after a recreate; an encoded name; a name only on another host (D, E) |
| REG | Kit build, deploy, smoke and the standard regression walk |
| T1 | Exact-head selectors for every break-map row, with sampled assertion-red mutations |

## Result

| Package | Result | Notes |
| --- | --- | --- |
| `1cdada20` (candidate) | Failed, 10/11 | RL1-D |
| `8c298b22` (delta) | **Accepted**, 11/11 | RL1-D and RL1-E; LD1-E and LD2-E responses and warning lines; shared error bodies without `$schema`; REG on D |
| `8a712aa1` (exact main) | Failed, 2/3 | Tree equals `8c298b22`; deploy and REG on H/D/E passed; RL1-D replayed 50 older failed activities |
| `9139d8a2` (exact main with #481) | **Accepted**, 4/4 | Deploy and smoke on H/D/E, REG on D, RL1-D and RL1-E in English and German; one restart toast and no toast for any of the 50 older failed activities, observed until 10 s after settlement |

Unchanged candidate cells are carried forward. The delta run reran only the cells
that the two later commits could affect: the admission key, and the log-download
schema and logging correction. The exact-main runs repeated the deploy and the
cells that main and #481 could affect; the second counted every action toast,
not only restart toasts.

Measured in the candidate run on the hub origin:

- TS1: during a 120 s stalled Edge log stream, all 60 container list requests
  answered with p95 37.5 ms and maximum 39.6 ms (bound 1000 ms). All 24 status
  samples showed E online.
- LD2: the delta run refused 71,680,019 retained bytes with 413, and a list probe
  during the download took 47 ms.

## Limits

- Only the agent process was restarted; hub process loss during a restart is
  covered by tests.
- An Edge log download is capped at 64 MiB and buffered in hub memory; Direct
  streams without a cap.
- Interactive Edge streams that fall far behind a slow browser are closed
  instead of throttled.
- Chromium only; host load was not controlled.

## Open follow-ups

#479 (review follow-ups: refusal headline wording, legacy-agent label proof,
refused payload flag, Edge delivery diagnostics), #480 (Activity fixture race and
package runtime), #468 (CI scanner timing flake, hit twice during M25).
