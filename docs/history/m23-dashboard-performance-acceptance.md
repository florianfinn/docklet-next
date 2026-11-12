# M23 dashboard performance and request hygiene acceptance — 2026-11-12

The accepted package is main `6e41f905b90b1da2010709954c8724d545e1d20c`, the
squash merge of the [M23 closing PR #461](https://github.com/florianfinn/docklet-next/pull/461).
Its tree equals the merged feature head `b559dca0`, which the last delta run
accepted. The packages were built locally from the exact source by the sandbox
kit; no registry image was used.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head. The Claude main thread wrote the design notes and
implemented S2, S3a, S4 and every correction; Codex workers built S1a, S1b and
S3b. Each step was reviewed by the other provider.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #435 #428 #425 #401 | S1a bounded event reads (`since`), session presence hint, disk-path fallback | R1 passed; two non-blockers fixed, one settled with query-plan evidence. |
| #435 | S1b agent route for active vulnerability counts (agent contract 18) | R1 passed; two non-blockers fixed. |
| #391 #389 #398 | S2 hub maintenance holds and skipped reads of unavailable hosts | R1 and verify rounds 1–4 failed (hold lifecycle, consumer mismatch, guard coverage); after a rebuild from a root-cause table, verify round 5 passed. |
| #435 | S3a Dashboard events window, readiness markers, active-count consumer | R1 failed; round 2 passed. |
| #428 | S3b inventory scope retention and metrics lifetime | R1 and round 2 failed; round 3 passed with one accepted deferral (R1E-7). |
| #401 #400 #431 #430 | S4 capacity source and time axis, run gate confirmation, density statement | R1 failed (unknown-outcome confirmation); round 2 passed. |
| #391 | Candidate corrections: refresh without aborting reads | Correction rounds 1–7 failed, round 8 passed. |
| #391 | Update-policy list without a baseline projection per default stack | Passed without findings. |
| #455 | Closing integration (#461) | The closing review passed with two minor non-blockers: PR-text precision (fixed in the PR text) and trailing whitespace in one test file (fixed with this record). |

## Defects found by acceptance

The candidate run failed three cells, and the delta runs found further causes:

- **Change notifications cancelled reads (MNT1, OFF1).** A stream change aborted
  reads in flight, which showed as aborts during hub maintenance and as repeated
  reads after a host returned. Refreshes now never cancel a read: a superseded
  refetch of loaded data repeats (at most three times) without publishing its
  outdated value. Readers mark their attempt current where they read their
  changeable state, the internal host catalogue is versioned per change, moves
  between skipped host statuses refresh only the host list, and automatic backup
  overview reads of a host share one read for 2 s. Eight review rounds closed
  the class, each with reviewer reproductions adopted as regression tests.
- **Default update policies slowed the Dashboard (DASH1).** The hub's policy
  list projected a live baseline (Compose inputs and a Docker observation) for
  every never-saved stack policy, serially. On the hub host with nine default
  stacks the list took 1.3–5 s, and the schedule widget waited on it. The list
  now leaves the default stack identity to the single policy read. The candidate
  had passed DASH1 only because its fleet happened to have saved policies.
- **DASH3 attribution.** The always-visible summary row reads capacity and
  active counts; by maintainer decision it counts as a consumer, so these reads
  are not hidden-widget reads.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
with a labelled fleet of about 50 containers per host. Timings and requests were
measured at the hub origin over HTTP/1.1 with Chromium and the DevTools
protocol.

| Cell | Behaviour |
| --- | --- |
| DASH1 | Cold first usable ≤ 2.5 s, cold settled ≤ 5 s, warm return ≤ 1 s (medians of five); one event request per cold load, none on a warm return |
| DASH2 | With ≥ 600 events in 24 h: one event request, "500+" with a link, the Events page lists the full retained history |
| DASH3 | Hidden optional widgets read none of their sources |
| UP1 | Fresh sign-in and first load: no 401, no aborted request, no request console error |
| MNT1 | Hub archive with Dashboard and Containers open: hint within 1 s, reads held (no maintenance 503), no error UI, reads resume |
| OFF1 | Direct, then Edge agent stopped with Dashboard, Security and Backup targets open: no reads of the offline host, quiet recovery, at most one refresh per query |
| TGT1 | Backup target edited while the Edge agent is stopped: pending, no repository read while offline, delivered after restart |
| GATE1 | An update issued 200 ms after the preceding run turned terminal is admitted |
| CAP1 | Missing disk path on D: meter and trend from one source, path shown |
| DENS1 | 100 containers per host: Dashboard, Containers and Stacks load without error |
| KIT/REG | Kit build, deploy, smoke and regression walk on all three hosts |

Maintainer decisions during acceptance refined how cells are counted, and in
one case what was fixed:

- M-1: the summary row counts as a consumer.
- M-2 and M-3: a Dashboard sample whose window contains event-log changes is not
  quiescent and is replaced (at most five replacements).
- M-4: offline recovery is counted per query; an attempt after a recorded later
  change on that host is a new refresh.
- M-5: fix the default-policy list in M23 and measure DASH1 with default
  policies.

## Result

| Package | Result | Notes |
| --- | --- | --- |
| `a331ec9d` (candidate) | Failed, 8/11 | DASH3, MNT1, OFF1 |
| `f2a84118` (delta 1) | Failed, 8/11 | DASH1, MNT1, OFF1 |
| `acecb815` (delta 2) | Failed, 9/11 | DASH1, OFF1 |
| `1b1760c7` (delta 3) | Failed, 9/11 | DASH1, OFF1 |
| `971f68c9` (delta 4) | Failed, 9/11 | DASH1, OFF1 |
| `c41e0341` (delta 5) | Failed, 10/11 | DASH1 cold settled 7.05 s |
| `b559dca0` (delta 6) | **Accepted**, 11/11 | Seven cells rerun; DASH2, GATE1, CAP1 and DENS1 carried from the candidate |
| `6e41f905` (exact main) | **Accepted** | Tree equals `b559dca0`; all cells carried; main push CI run 43126368607 passed |

| DASH1 median | Main baseline | Candidate | Delta 6 | Limit |
| --- | --- | --- | --- | --- |
| Cold first usable | 1.03 s | 0.74 s | 1.02 s | 2.5 s |
| Cold settled | 7.06 s | 3.60 s | 4.19 s | 5 s |
| Warm return | 0.91 s | 0.35 s | 0.18 s | 1 s |

## Limits

- Host load was not controlled; the three hosts share one kernel, and the
  recorded load average was evidence only. Chromium only.
- Timing cells were measured at the hub origin over HTTP/1.1, not through the
  public route.
- DASH2, GATE1, CAP1 and DENS1 were measured on the candidate and carried; their
  code paths did not change afterwards.
- DASH1 was measured with default update policies. Saved policies are faster;
  the first scheduled update check saves them.
- Request-only observations outside the named request classes stayed advisory.

## Open follow-ups

- #462: the shell's navigation counters read the full Security data and the
  decorated volume list on every page.
- R1E-7 (first-load tracking of seen rows across identity changes) is an
  accepted deferral without an issue.
