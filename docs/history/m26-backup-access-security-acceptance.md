# M26 backup, access and security follow-ups acceptance — 2026-11-14

The accepted package is main `fb081f5b38f88941a32050a71f7b7e946fcdfd66`, the
[M26 closing PR #485](https://github.com/florianfinn/docklet-next/pull/485). Its
tree equals the feature head `01832df0`, which passed the final delta run. That
head differs from the candidate `c8bdfd51` by one transport test, so the
candidate's production build was the build under test for both delta runs. The
sandbox kit built the packages locally from the exact source; no registry image
was used.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head.
- The Claude main thread planned the milestone, wrote the design notes,
  implemented S1 and S5, integrated every step and made every correction after the
  reviews.
- Codex workers built S2, S3, S4 and S6.
- Each step was reviewed by the provider that did not implement it.
- Corrections by the main thread were verified by Codex.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #394 #378 (rows) #387 (code) | S1 contract foundation: volume collection routes under `_collection`, 16 access rows, new `BackupProblem` code, agent contract 18 with a new fingerprint | R1 passed with one non-blocker (provenance text), fixed. |
| #387 #388 #390 | S2 backup follow-ups | R1 passed with one non-blocker; its regression test and documentation sentence were added. |
| #378 #447 | S3 access guard and auth/activity follow-ups | R1 passed with three non-blockers, all fixed. |
| #445 | S4 vulnerability follow-ups | R1 passed with four non-blockers; three were fixed, and one cosmetic item went to #488. |
| #386 | S5 streamed snapshot download | R1 failed with four blockers: an exec stream accepted before `tar` ended, fixture races, an archive proof that bypassed the producer, and missing transport proofs. R2 failed with one blocker: zero-filled file content passed the archive-end check. R3 passed with one non-blocker (cancellation fixture), fixed. |
| #449 #462 | S6 Security header help and shell counters | R1 failed with one blocker (focus forced back to the help button after an outside press) and four non-blockers. The verify round passed and carried one cosmetic item to #488. |
| #458 | Closing integration (#485) | Passed with no blockers and no new findings; the two cosmetic carryovers are in #488. |

## Findings during acceptance

No product defect was found in the sandbox.
- **V4 producer stop, attempt 1.** Chromium's CDP page throttling did not slow
  native browser downloads. The archive therefore finished on the server before
  the helper stop took effect, and the download correctly completed. A sandbox
  diagnosis then applied real TCP backpressure: the stop ended `tar` with 137,
  recorded a failed activity and left an incomplete archive. The delta runs moved
  the throttle to a rate-limited loopback proxy in the browser's path (decision
  AUTO-1 on #458). The behaviour under test and the pass criteria were unchanged.
- **A2 History screens, attempt 1.** The matrix asked for History tabs that
  simple users never get by the accepted design (`docs/design/simple-users.md`
  §4). All API checks of the cell passed. The maintainer removed the screen part
  (decision Q-A2-SCREENS on #458), and delta 1 passed A2 on its API checks.
- **V4 producer stop, delta 1.** `tar` was witnessed running and then ended with
  137, and the activity was recorded as failed. The browser connection, however,
  stayed open until the test proxy timed out. Goroutine dumps showed the hub
  waiting for the agent body. The cause was a sandbox-only TCP relay between hub
  and Direct agent that did not pass the agent's end of the connection on to the
  hub. Delta 2 replaced it with the same relay plus a half-close (decision AUTO-2
  on #458). A new test proves the product behaviour through the real agent stack
  and the hub's Direct forwarding: `TestStreamedDownloadAbortReachesBackpressuredClient`.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
- UI assertions ran in Chromium, in English and German where a cell names a locale.
- Latency was measured at the hub origin unless a cell names the loopback proxy.
  For V3 that proxy has a 60 s read timeout with buffering off. V4's delta runs
  used a separate rate-limited loopback proxy with a 120 s idle timeout.
- Unit and transport cells were checked against the candidate head's CI run and
  the red-proof ledger.

| Cell | Behaviour |
| --- | --- |
| V1, V2 | Volumes named `binds`, `counts`, `sizes`, `prune` open on H, D and E; bind rows, sizes and prune preview unchanged |
| V3 | A snapshot whose preparation exceeded 90 s on the baseline downloads through the proxy with early bytes; extracted tree matches the source |
| V4 | Stopping the producer mid-transfer fails the browser download and the activity; cancelling fails the activity; cleanup and new admission work |
| V5 | Edge download regression: small archive matches, over-cap download answers 413 |
| B1 | Draft target probe on an agent without its hub identity answers `backup.target.identity-unavailable` (EN, DE) and succeeds after delivery |
| B2 | Retired target paths stay protected; snapshot browse and download keep working |
| A1, A2, A3 | Administrator-only Echo endpoints refuse simple users; access rows give the agreed API results; sign-in, refresh and sign-out |
| S1-SEC, S2-SEC, S3-SEC | Candidate result reads cache raw registry observations for 60 s; standalone containers make no stack baseline read; duplicate advisory rows are shown and selected together |
| T1 | Touch opens and closes the Security header help in condensed and card views |
| N1 | Shell badges use one active-summaries and one volume-count read per host with unchanged values, including a partial Edge host |
| REG | Standard regression on H, D and E in English and German |

Edge streaming of long preparations was excluded by the kickoff decision and is
tracked in #486.

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `c8bdfd51` (candidate) | Failed, 26/28 | V4 (throttle method), A2 (screen part) |
| `01832df0` (delta 1) | Failed, 27/28 | A2 passed under the corrected matrix; V4 stalled behind the sandbox relay |
| `01832df0` (delta 2) | **Accepted**, 28/28 | Relay fixed; the hub ended the incomplete download 1.8 s after `tar` exited with 137 |
| `fb081f5b` (exact main) | **Accepted**, 28/28 | Tree equals `01832df0`; deploy, REG on H/D/E in English and German (36/36 operations), a D volume named `counts` and a D snapshot download; logs clear |

Unchanged passing cells carry their evidence forward, so each denominator counts
all 28 cells, not 28 newly executed ones. Delta 1 ran V4 part (i), re-evaluated
A2 from its unchanged API evidence and verified the added transport test. Delta 2
repeated V4 part (i). The exact-main run rebuilt and deployed main and repeated
REG and the listed Direct checks.

Measured in the candidate run:

- V3: 3 GiB snapshots, first body byte at the origin 45.5 ms (H) and 29.8 ms (D);
  through the proxy 47.2 ms (H) and 27.9 ms (D). Before M26, preparation took
  125.0 s (H) and 117.4 s (D) before the first byte, and the proxy answered 504
  at 60 s.
- S1-SEC: registry calls per candidate result read on D and E were 3 cold, 0
  within 60 s and 3 after 61 s, for three consumers of one image (before: 3 per
  consumer on every read).
- N1: 206 Volumes rows equalled the badge (185 named volumes plus 21 bind
  sources); each host made exactly one active-summaries and one count read.

## Limits

- Edge snapshot downloads stay fully prepared with a 512 MiB cap (#486).
- One measurement per V3 path; host load was not controlled.
- Chromium only.
- No upgrade from the M25 package was exercised: the sandbox kit has no
  upgrade cell. M26 changes the agent contract fingerprint, so hub and agents
  upgrade together.
- The sandbox relay fix lives in the sandbox kit; product hubs reach agents
  directly or through proxies that pass a connection end on.

## Open follow-ups

#486 (Edge streaming downloads), #487 (publication scanner timing guard),
#488 (review follow-ups: registry header layout, shell inventory provenance
text).
