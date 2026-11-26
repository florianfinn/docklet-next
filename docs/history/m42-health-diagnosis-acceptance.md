# M42 health diagnosis acceptance — 2026-11-26

The accepted package is main `04028484544ff322ffe561f2b60926db5112f7e1`: the
[M42 closing PR #639](https://github.com/florianfinn/docklet-next/pull/639)
(squashed as `a9719e94`) plus the acceptance fix
[#643](https://github.com/florianfinn/docklet-next/pull/643). The sandbox kit
built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M42 explains why a container is unhealthy or why its healthcheck has not run,
and shows health per service on the stack page. It is web only: the existing
container read already carries Docker's health log and healthcheck
configuration.
- The Claude main thread planned the milestone, implemented the diagnosis (S1),
  integrated the steps and made every correction after reviews and acceptance
  runs, including #643.
- Codex workers drafted the stack Health tab (S2) and the container section and
  hover (S3). Codex reviewed the design, S1, the closing PR and #643; Claude
  reviewed S2 and S3. The sandbox runs and diagnoses were delegated to Codex.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Plan, matrix and break map | The design review passed with changes: history needs the Docker container name, not the service name; the reason classes and their precedence were made exact; the web keeps only a health projection of the inspect body. All were folded in before kickoff. |
| #635 | S1 diagnosis and health read | Failed with 1 blocker: probes of an earlier run of the same container could explain the current state, and starts were compared at millisecond precision. Only probes of the current run count now, compared at nanosecond precision; the verify passed. |
| #636 | S2 stack Health tab | Passed; the break-map row for a same-name recreate and two order checks were added as tests. |
| #637 | S3 container section and hover hint | Passed; an unhealthy service without a container now says so instead of loading forever. |
| all | Closing integration (#639) | Passed. |
| — | Acceptance fix #643 (fit view and open hover cards) | Failed with 1 blocker: keeping the mounted view after a font load kept widths measured with the fallback font, so a table that fits stayed in cards. The fix now refits from the widest view and defers that only while a popup is open; the second review passed. |

Assertion-failure red proofs cover the reviewed fixes and the break-map rows
(current-run fencing, same-name recreate, missing identities, isolated read
failures, lazy hover reads).

Maintainer decisions: D-1 an own Health tab on the stack page and a section on
the container page; D-2 per-service strips from the existing per-container
history, queried per service by the web; D-3 simple hints with raw data behind
a toggle; Q-2 admin only; Q-3 a cautious overdue hint; Q-4 the existing
container read; Q-5 the D1 defect is fixed in a standalone PR before
completion; Q-6 the security-list layout race is excluded (see below).

## Findings during acceptance

- **Exact main 1, D1.** At 390 px the hover cards of a stack row closed about
  100 ms after a tap. Each web font `loadingdone` reset the fit view to the
  table, which rebuilt the phone cards and unmounted the open card. The code
  dated from M18; it also happened on a stack without unhealthy services.
  Fixed by #643.
- **Exact main 2, layout.** The security vulnerability list sometimes stayed a
  975 px table at 390 px after a resize from 1440 px: a font reset replaced a
  pending step-down, and nothing measured again. The #643 popup paths did not
  run; the reset path is the same as before #643. The maintainer excluded this
  part (Q-6); the measuring design is to be rebuilt under #646.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
admin role, 1440 and 390 px. Fixtures were compose stacks with real probes:
BusyBox wget against a missing path and a closed port, curl missing in the
image (exec and shell forms), a timeout, a failing script, no healthcheck, a
disabled healthcheck, a one-hour interval and an always-healthy probe.

| Cell | Behaviour | Result |
| --- | --- | --- |
| A1–A10 | Hint per probe class on H (HTTP 404, tool missing twice, connection refused, timeout, other failure with raw output, no healthcheck, disabled, waiting, healthy without hint) | PASS (candidate; A1 again on exact main 1) |
| B1, B2 | Same hint and raw data on D and on E | PASS (candidate) |
| C1 | Per-service strips differ, stack strip aggregated, 24h/7d/30d toggle | PASS (candidate) |
| C2 | Same-name recreate: strip continues, hint from the new container | PASS (candidate) |
| C3 | Raw data toggle by keyboard | PASS (candidate) |
| D1 | Overview hover hint, link to the Health tab focused on the service | PASS (candidate; exact main 2 at 1440 and 390) |
| D2 | Container page health section | PASS (candidate) |
| D3 | Simple user: no Health tab or link | PASS (candidate, exact main 1) |
| REG | Standard regression on H, D and E | PASS (exact main 1 and 2) |

Exclusions agreed at kickoff: a real overdue first probe (Docker has no
reproducible overdue state; a clock test covers it) and multi-week history (the
run lasts minutes; recent buckets were checked and older 7d/30d buckets are
empty).

## Results

- Candidate `bf213fac`: ACCEPTED 18/18.
- Exact main 1 `a9719e94`: NOT ACCEPTED 4/5 (D1 at 390 px).
- Exact main 2 `04028484`: NOT ACCEPTED 3/4 (layout part); smoke, regression on
  H, D and E and D1 at both widths passed. The failed part is excluded by the
  maintainer (Q-6), so the counted parts are 3/3. The runner's verdict is
  recorded unchanged.

## Limits and follow-ups

- #646 tracks the fit-view rebuild: one measurement trigger for resize, font
  and key changes, and widths of wider views measured without swapping the
  live rows. It covers the excluded layout race and two related cases found by
  the #643 review.
- #645 tracks a flaky activity-reader test that failed once in CI during #643
  and passed on rerun.
- #648 tracks the nonblocking review notes: deep-link focus before rows settle,
  identical raw-toggle names in separate service regions, a missing strip
  without explanation when a container has no Docker name, and three test and
  provenance wording notes.
