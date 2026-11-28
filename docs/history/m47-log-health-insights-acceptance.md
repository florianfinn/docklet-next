# M47 log and health insights acceptance — 2026-11-28

The accepted package is main `496d99819952fc8f9082349da230e8a54cbe7a07`: the
[M47 closing PR #697](https://github.com/florianfinn/docklet-next/pull/697),
squashed from head `956c5f0a`. The sandbox kit built every package locally from
the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M47 answers three questions with data Docklet already has: why a container
stopped, which containers failed at the same time, and what a long log consists
of. There is no model, no background log capture and no new table. Design:
[log and health insights](../design/log-health-insights.md).

- #692 Health and Logs tabs: log levels read from the line, Health facts (reason,
  exit code, recent restarts, newest failing probe, last log lines), collapsed
  repeats, an error-density strip and error jumps; bounded one-shot log reads.
- #693 Disruption timeline: a hub-only host container history route, an additive
  `until` bound on log streams and a Disruptions page with a combined log window.
- #694 Pattern mode: a bounded read folded into line shapes in the browser, with
  time bins, a paged bin read, a custom range, remembered windows and a line limit.

The unreleased agent contract v26 fingerprint changed because its fixture hashes
all history schemas; agents must run the hub's package (K-5).

- The Claude main thread planned the milestone, implemented the backend (S2),
  integrated every step and made every correction after reviews and acceptance.
- Codex workers drafted the web steps S1, S2 web and S3. Codex reviewed the
  design, S2 backend, the finite-read fix, the closing PR and every later
  addition; Claude reviewed S1, S2 web and S3. The sandbox runs were delegated
  to Codex.

| Step | Review outcome |
| --- | --- |
| Plan, matrix and break map | The design review found six blockers, folded into the note before kickoff. |
| S1 web (Health and Logs tabs) | Two blockers, fixed with red proofs. |
| S2 backend (history route, `until`) | Two blockers in the tests, fixed; the verify passed. |
| S2 web (Disruptions page) | Passed; one nonblocking note became AUTO-1. |
| S3 web (pattern mode) | One blocker, fixed with a red proof. |
| Finite-read fix (own connection per one-shot read) | One blocker (missing session recheck), fixed; the verify passed. |
| Closing integration (#697) | One blocker: pattern rows were keyed by shape only, so the level filter kept a stale row. Fixed in `e35fc803`; the verify passed. |
| K-6 additions | Three blockers in the bin read (millisecond `until`, cap count, unmarked continuation lines), fixed in `03471bcf`; the verify passed. |
| K-7 paging and AUTO-2 | Two blockers (stack cursor, replay byte stall), fixed in `22d42dd5`; the next review found one more (budget from the last read only), fixed in `013f95cd`; the verify passed. |
| K-8 additions | One blocker (a refreshed window was read again after a remount), fixed in `94ccd6ca`; the verify passed. The delta 6 finding was fixed in `01329072` and verified. |
| Toolbar tooltips (`1229d52f`, `956c5f0a`) | `1229d52f` passed with no blockers. `956c5f0a` applied the review's nonblocking note (the container page's pause toggle) with its own red-proof test. |

Assertion-failure red proofs cover the reviewed fixes and the break-map rows.

Maintainer decisions at kickoff: K-1 the Disruptions page and the host history
route are administrator-only; K-2 entry from the dashboard and the Activity
Center, plus a sidebar entry behind a personal Appearance switch that defaults
to off; K-3 the run ends at a ready closing PR; K-4 phone timing is measured in
emulation, a real phone is the maintainer's practical check; K-5 agent updates
are mandatory.

Later maintainer decisions (K-6, K-8 and K-9 after real-phone checks, K-7
after delta 3): K-6 coloured summary
chips, no span per pattern row, a loading spinner, a custom range, a read of a
selected time bin and short hover hints; K-7 500 lines per bin page with "Load
more lines" and horizontally scrolling long lines; K-8 times to the second,
remembered results per window and a line limit of 10,000, 50,000 or 100,000
lines; K-9 short tooltips on the log toolbar toggles, then the merge.

Main-thread decisions: AUTO-1 the combined disruption window reads at most
2,000 lines / 1 MiB per participant; AUTO-2 a one-shot read whose socket opened
but closed without the normal close ends as `interrupted`, and every consumer
says that lines may be missing.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
administrator and simple-user identities, Playwright Chromium against the hub
origin at 1440 px and 390 px, and English and German locales.

| Cell | Behaviour |
| --- | --- |
| A1 | Unhealthy container: the newest failing probe is visible without expanding anything (D) |
| A2 | Exited container: exit code, reason, last log lines and the link to its logs (E) |
| A3 | OOM-killed and restart-looping containers show their reason and recent restarts (D, E) |
| A4 | Noisy log: `×N` counts, structured levels, density strip and error jumps (D) |
| A5 | Overlapping disruptions on D and E and their combined, time-ordered log window |
| A6 | `until` bound: no frame after the window's end on D and E |
| A7 | Scoped API key: history and log access follow the key's scope |
| A8 | Simple user: no Disruptions page, no navigation entry, `access.admin-required` |
| A9 | Pattern mode: exact counts on a uniform log, the cap and notice on a variable log, socket closed on leaving, per-service patterns in a stack |
| A10 | Pattern mode timing over 100,000 lines: longest main-thread task at most 500 ms on desktop |
| A11 | German UI without raw keys |
| A12 | Regression of the live Logs tab, the Health status strip and the standard regression set |
| A13 | Appearance switch for the sidebar entry, off by default and synced |
| K6-1–K6-4 | Bin read with exact bounds, custom range, hover hints, phone layout |
| K8-1–K8-3 | Remembered windows, line limit on the badge, times to the second |
| T1–T3, R | Exact main: toolbar tooltips on desktop and with touch emulation at 390 px, short regression |

Exclusion E1 (agreed at kickoff): the runner has no real phone, so A10's verdict
covers the desktop bound; throttled phone emulation is documented. The
maintainer checked the package on a real phone after delta 1, delta 5 and
delta 7; K-6, K-8 and K-9 came from those checks.

## Results

| Run | Head | Result |
| --- | --- | --- |
| Early smoke | `0eb90bfa` | All checks and the standard regression passed on H, D and E |
| Candidate | `b80bae99` | 11/13: pattern mode received only part of the log because the shared live hub dropped a slow one-shot reader (fixed in `b60de6b2`) |
| Delta 1 | `b60de6b2` | ACCEPTED 13/13 |
| Delta 2 (K-6) | `6ce32007` | ACCEPTED 17/17 |
| Delta 3 | `03471bcf` | NOT ACCEPTED: a dense final bin exceeded the then 2,000-line cap; superseded by K-7 |
| Delta 4 (K-7) | `d4e3ad77` | NOT ACCEPTED: an Edge pattern read ended early without a notice (AUTO-2) |
| Delta 5 | `013f95cd` | ACCEPTED 17/17 |
| Delta 6 (K-8) | `94ccd6ca` | NOT ACCEPTED: same-second density hints repeated the time |
| Delta 7 | `01329072` | ACCEPTED over the full matrix |
| Exact main | `496d9981` | ACCEPTED 4/4 (T1, T2, T3, R) |

Pattern mode over 100,000 lines took 22.4 s on desktop with a longest
main-thread task of 203 ms (216 ms with the K-6 bin buttons). With a 4× CPU
throttle at 390 px it took 144.8 s with a longest task of 1,080 ms; this is
documented under E1, not part of the verdict.

In the exact-main run the first kit smoke went 20/21: one host-filter step
waited for a preference write that the already selected host never sent. The
unchanged smoke then passed 21/21 on the same deployment. The runner also
corrected its own tooltip pointer and focus driver; product failures: none.

## Limits and follow-ups

- On Edge agents, the tunnel's existing 16 MiB overflow and 5-second delivery
  limits can end a long read early. When the socket then closes without the
  normal close, the web reports the read as interrupted; some upstream errors
  can still arrive as a normal close. Flow control is in #698.
- Throttled pattern mode over 100,000 lines takes minutes and has single long
  tasks (#698).
- #698 tracks the other nonblocking review and acceptance notes.
