# M20 pre-release hardening acceptance — 2026-11-08

The accepted package is main `08980fad696300611d4d8816a3f0c8d6cdf2247b`, the
squash merge of the [M20 closing PR #427](https://github.com/florianfinn/docklet-next/pull/427).
Its tree equals the merged feature head `d6c64d47`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:

- Codex GPT-6.1 Sol workers implemented the module removal and contract step, the
  first web hardening pass, the HTTP/1.1 host-read isolation and the stale
  snapshot recovery. Claude Opus or Sol reviewed them.
- The Claude main thread wrote the design notes, the runtime, proxy and test
  steps, the Dashboard and terminal fixes and every fix round. Codex Sol
  reviewed them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #358, #419 | S1 removal of webhooks and the remote CLI, migration 020, agent contract 16 | R1 failed with 1 blocker; the fix verify passed. |
| #367, #368, #375, #415 | S2 runtime: recreate recovery, restart refusal while backups run (`host.busy`), shell frame limit, Edge stream back-pressure | The combined S2/S3 R1 failed with 4 blockers, 3 in runtime recovery; the fix verify passed. |
| #264, #274 | S3 route identity for encoded paths and the reverse-proxy page | Route identity failed in the initial design review and its delta check. The combined S2/S3 implementation R1 then found a shared-listener gRPC bypass; the rebuilt entry-point coverage and fix verify passed. |
| #359, #24, #175, #177, #383, #421, #271 | S4 web: coming-soon pages, localisation, loaded-data navigation, log errors, password labels, bounded host reads | R1 failed with 1 blocker: the #383 root cause was not established. The fix was verified in the closing review. |
| #271 | D4 HTTP/1.1 host-read admission and result-local recovery | R1 failed with 1 blocker: a JavaScript stream reader made Chromium record completed reads as aborted. Native body methods fixed it. |
| #271, #383 | C1 fair admission and target-scoped stack detail | R1 failed with 1 blocker: a paused agent kept five reads after the hub answered it unavailable. Fixed. |
| #271 | C2 recovery of retained stale snapshots | R1 passed with 1 test finding, applied. |
| #360 | Dashboard timeline render isolation | R1 failed with 2 blockers (prepended events, locale); R2 passed. |
| #360 | Terminal readiness | R1 failed with 1 blocker (paste and IME bypass); R2 passed. |
| #159, #171, #173, #273, #304, #396, #422 | S5 load-independent tests and the CI timeout budget | Checked in the closing review, including a real quadratic mutation of the publication scan. |
| #360 | Closing integration (#427) | Sol failed on the two hub-local REG acceptance failures and verified every other correction. The terminal race was fixed and re-checked. The update refusal was decided by the hub origin. The documentation finding was fixed. |

## Defects found before acceptance

An early smoke, a candidate run, four delta runs and four diagnostic traces on
the integrated feature head found these defects. All were fixed before the
merge:

- the encoded-path refusal text was hidden on stack and container commands;
- with one Direct agent paused, HTTP/1.1 pages starved the healthy hosts' reads
  and never recovered the paused host without a reload (#271). The fix has three
  parts: fair admission of at most five host reads, one read for a host the hub
  answers unavailable, and a 15-second recovery that also covers the hub's
  retained snapshots;
- the stack page settled 9 to 16 seconds after a lifecycle run on a host with
  hundreds of containers, because it read the whole container inventory (#383).
  It now reads only the project's containers;
- the Dashboard timeline rebuilt one tooltip marker per event of the last 24
  hours on every metrics update. With about 11,000 markers, completed responses
  reached the page tens of seconds late and inventory reads hit their 30-second
  budget;
- the apps widget threw and caught one error per stack without an address on
  every render;
- the terminal reported Connected before the shell's first output, so input typed
  at once could interleave with the shell's cursor-position query.

The first candidate runs met a sandbox with 220 to 340 containers per host,
mostly leftover fixtures of earlier milestones. The maintainer had the sandbox
cleaned to homelab density, about 15 to 40 containers per host; dense fleets are
tracked in #430.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The sandbox had the
hub-local host, one Direct agent and one Edge agent, and a local OCI registry for
fixtures. Package checks on each run:

- migration ledger at 020;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract 16.

The matrix covered:

- the upgrade from the M19 package;
- the removed modules and their refusals;
- encoded-path refusals on every host type;
- HTTP/1.1 host-read isolation with a paused Direct agent, over HTTP/1.1 and
  HTTP/2;
- terminal event delivery to an open Dashboard and stack page, and cold
  Dashboard completeness;
- the busy-restart refusal and its durable reason;
- production surfaces, localisation and the first-run state;
- a regression of stack operations, update, backup, terminal and Dashboard;
- the named tests of the CI run of the exact package.

Through the production UI it used Playwright Chromium in German and English at
1440 and 390 px. API, Docker and sqlite3 calls prepared fixtures and verified
the results.

The maintainer decided these acceptance readings:

- the hub origin decides terminal event latency and the hub-local update
  refusal: the sandbox's shared proxy route buffers the event stream;
- only the restart pair counts for the busy-restart cell;
- the restart panel's reason after a reload is a follow-up (#429);
- the runs on exact main repeat only the cells the later commits affect.

## Result

The runs on the integrated feature head were a candidate run and four delta
runs; each delta repeated only the cells its fixes could affect. The run on
exact main repeated the upgrade smoke, the Dashboard isolation cell, terminal
event delivery on the hub origin and the regression on the hub-local host, and
reused the other cells' evidence.

| Package | Result | Cause and fix |
| --- | --- | --- |
| `f19d713e` (candidate) | Not accepted | Healthy hosts stayed incomplete on a cold HTTP/1.1 Dashboard and stack pages settled late on dense hosts. Fixed by fair admission and the target-scoped stack detail. |
| `f9d0aaa5` (delta 1) | Not accepted | Healthy-host reads and Dashboard settlement still failed on the dense fleet after the held-host cap fix. The maintainer approved cleanup to homelab density and a separate dense-fleet follow-up. |
| `cd7a7d1b` (delta 2) | Not accepted | After cleanup, cold Dashboard and stack-detail checks passed, but containers and Dashboard retained unavailable snapshots after recovery, and Dashboard event settlement still failed. Fixed by stale snapshot recovery and the later render isolation. |
| `d4a55e15` (delta 3) | Not accepted | The Dashboard timeline starved the page, so inventory reads hit their budget. Fixed by render isolation. |
| `c76bab82` (delta 4) | Not accepted | Isolation and event delivery passed. The hub-local regression failed: an update refused as busy through the buffering proxy route (decided by the hub origin) and the terminal input race, fixed by terminal readiness. |
| `08980fad` (exact main) | **Accepted** | All 29 applicable host cells and the CI cells T1 and T2 passed. With Direct paused, the Dashboard kept the healthy hosts complete and recovered Direct 8.5 s after unpause without a reload. Five Dashboard restarts on the hub origin settled in 1.9 to 5.2 s without query errors. The terminal marker arrived through both routes. The update passed on the hub origin; through the proxy route it was refused again while the terminal event was 21 s late, as accepted. |

The exact-main full test passed in 28 minutes.

## Limits

- Several paused agents can still slow healthy hosts over HTTP/1.1 until their
  reads reach the 30-second budget; HTTP/2 at the reverse proxy is recommended.
- Through a reverse proxy that buffers the event stream, live updates arrive
  late and a finished run can still lock its target in the browser until its
  event arrives (#431).
- A shell cursor query split across two output frames can still let input typed
  right after Connected reach the shell first.
- Dense fleets of hundreds of containers per host are outside the tested range
  (#430).
- Direct and Edge agents ran on separate Docker daemons on one shared kernel.
- Chromium only; the shared sandbox proxy route was HTTP/1.1 with event
  buffering, and the hub origin was used for the decisive timing cells.

## Open follow-ups

#428, #429, #430, #431.
