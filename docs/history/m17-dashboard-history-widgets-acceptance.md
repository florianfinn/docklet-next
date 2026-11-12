# M17 dashboard, history and service widgets acceptance — 2026-11-04

The accepted package is main `aafcd09b857d406c0b35f0c4c3887d6079fabc77`. It is
the [M17 closing PR #402](https://github.com/florianfinn/docklet-next/pull/402),
squash-merged as `4155c8fe`, plus two acceptance fixes:
[#403](https://github.com/florianfinn/docklet-next/pull/403) and
[#404](https://github.com/florianfinn/docklet-next/pull/404). The tree of
`4155c8fe` equals the merged feature head `086acb65`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:
- Codex GPT-6.1 Sol workers implemented the backend steps S1 and S2, and Claude
  Opus reviewed them;
- the Claude main thread wrote the design notes, the web steps and every fix
  round, and Codex Sol reviewed them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #331, #335, #339, #371 | Design notes for runtime history, service widgets, profile preferences and the fleet ring | R1 failed with 12 blockers: intents recorded on the owning host, rights per route, total decoders, a resumable preference migration, condition precedence and a commit gate. Fixed in notes v2; the delta check found one more blocker, an owner-bound legacy import, fixed in text. |
| #335, #338, #339, #371, #337 | S1 backend: minute sampling and history buckets, runtime conditions, sustained-load alerts, capacity trend, profile preferences, contract 13 | R1 failed with 3 blockers: operation identity selection, the condition baseline and pruning before a successful sample. Verify 1 failed with 2 more, a lost uninspected state and swallowed daemon errors. Verify 2 passed. |
| #332, #333 | S2 widget backend: framework, stored credentials, certificate pins, Plex, Overseerr and Minecraft drivers | R1 passed with no blockers. |
| #332, #334, #335, #336, #337, #338, #339, #371 | Web: four-colour ring, app links, history sections, capacity line, widgets, preference sync and the reduced dashboard for simple users | R1 failed with 8 blockers. Verify rounds 1 and 2 left 2 and then 1 blocker, the runtime condition decoder. It was rebuilt as a total snapshot decoder, and round 3 passed. |
| #340 | Candidate fixes after the sandbox runs | The first correction failed with 2 blockers: capacity values were lost within a bucket, and Direct running counts disagreed with the ring. Its verify passed. A prepared statement brought the sampling write p95 under the HS8 limit. |
| #340 | Closing integration (#402) | Opus part A passed. Sol part B failed with 1 blocker: an unsaved widget settings draft was lost when a configuration read failed. Fixed; part B rounds 2 and 3 passed. |

## Defects found before acceptance

The candidate run on the integrated feature head found seven defects:
- an existing default disk path stopped all history sampling on remote hosts;
- German state text was untranslated;
- the loading breadcrumb overflowed at 390 px;
- the preference storage text contradicted hub synchronisation;
- a real stack update did not draw the ring's orange arc;
- the "latest" feeds were ordered by ID instead of event time;
- a Direct outage did not show the unreachable ring state.

All seven were fixed. The candidate rerun then failed only HS8: with debug
logging, the sampling transaction p95 was 380 to 487 ms. A prepared statement
reduced it, and the HS8 recheck on the corrected head passed with p95 96, 144
and 114 ms.

During these runs the main thread decided CP1's capacity source: the meter's
"used of total" stays the live metric, and history supplies the line, the growth
and the time until full. The two sources can differ when the configured disk
path is missing; #401 tracks aligning them.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The sandbox had the
hub-local host, one Direct agent and one Edge agent. Test services were fake
Plex and Overseerr servers built from the documented response shapes, one served
over HTTPS with a self-signed certificate, a real Minecraft Java server and a
webhook receiver.

Package checks on each run:

- migration ledger at 017 and SQLite quick checks on the hub and both agents;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract 13.

The run applied 24 matrix rows, 41 cells, on every applicable host type, through
the production UI with Playwright Chromium in German and English at 1440 and
390 px. API, Docker and sqlite3 calls prepared fixtures, injected deliberate
faults, seeded 7- and 30-day history buckets and verified the results. It
covered:

- availability and resource history on container and stack pages: 24-hour,
  7-day and 30-day ranges, conditions for Docklet stops, outside stops,
  crashes, restart loops and OOM kills, recreates and updates, agent outages and
  a hub restart;
- the sampling write load with at least 100 sampled entities per host over the
  full run, and the projected 30-day database growth;
- the four-colour fleet ring and its mapping in lists and detail pages, the
  capacity widget, and sustained-load alerts with webhook delivery;
- preferences on the user profile across fresh browser profiles and users, and
  the one-time upload of pre-M17 browser values;
- the service widgets with stored credentials, failure isolation, the pinned
  certificate fingerprint, Overseerr approval and the reduced dashboard for
  simple users;
- app links, the upgrade from the M16 package with data, and translation and
  layout in both languages.

## Result

The first run on merged main was a fresh full run. Later runs repeated the
failed cells and the cells the fix could affect:

| Package | Result | Cause and fix |
| --- | --- | --- |
| `4155c8fe` | Not accepted, 37 of 41 cells | R1 H, D and E: retained backup job rows in the Activity Center showed the raw code `volume.backup.interrupted`. PR1 H: after signing out, background reads of lists and network details answered 401 outside the request budget. Review of the fix found the same gap in request retries and in metrics, log and shell sockets. #403 translates known backup codes without backup context. Around a logout it holds new non-auth reads, retries, downloads and socket handshakes and refuses them after sign-out; writes and requests already sent are not held. |
| `236db98e` | Not accepted, 38 of 41 cells | R1 D passed. R1 H and E: the "notification sent" activity title was English in German. PR1 H: six reads sent just before the logout answered 401 after it completed. #404 lets reads in flight land before the logout, for at most 5 s, and translates the titles of all 75 hub event codes, including their phases and outcomes. |
| `aafcd09b` | **Accepted, 41 of 41 cells** | R1 H, D and E and PR1 H were rerun in full. The other 37 cells passed on `4155c8fe` and were reused, because both fixes changed only the web client. |

On `aafcd09b` all three PR1 sign-outs stayed within the request budget and
reached the sign-in page in 3.0 to 3.6 s. The first R1 H attempt failed on one
Edge vulnerability read that answered 502 outside any fault window; the second
attempt passed. It is tracked with the other request noise in #389.

Under the matrix rule, request failures during deliberate faults that show no
error to the user count as budget deviations, not failures: HS5 D and E, HS6 H
and UP1. On `4155c8fe` the HS8 measurement spanned 175 minutes with p95 72, 109
and 107 ms on H, D and E, and a projected 30-day history size of 13 to 15 MB per
host. X1 and X2 were not used; X3 covers the fake Plex and Overseerr servers.

The fix #403 took four review rounds. Logout handling failed the first two and
was rebuilt from a table of every physical request path; round 3 found a
deadlock and unguarded download links, and round 4 passed. #404 also took four
rounds. Its first review found English titles for older hub events in German.
The maintainer widened the fix to every hub event title instead of excluding
them (Q-A9). The new mapping then failed twice: phase and outcome titles were
lost, and resource names were treated as prune markers.

## Limits

- The capacity meter and its history can use different disk sources when the
  configured path is missing (#401).
- The capacity line spaces samples by index, not by time (#400).
- Direct and Edge agents ran on separate Docker daemons on one shared kernel, so
  a CPU burner could raise sampled CPU on more than one host.
- Plex and Overseerr were fakes built from their documented response shapes.
- Seven- and 30-day ranges used seeded history buckets; real accrual covered
  more than two hours.
- Chromium only, over HTTP/1.1 through the sandbox's proxy.

## Open follow-ups

#389, #400, #401.
