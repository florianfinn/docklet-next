# M35 simpler everyday use acceptance — 2026-11-22

The accepted package is main `fa84f18531532485f90c22bd054d9aadb3666a13`. It is
the squash of the [M35 closing PR #563](https://github.com/florianfinn/docklet-next/pull/563).
Its tree is identical to the accepted feature head `2f5586d7`. The sandbox kit
built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M35 covers all 21 findings of the main-state review of 2026-11-21. All steps
were pushed to one feature branch as commits, without step PRs. CI ran on every
pushed head.
- The Claude main thread planned the milestone, wrote the design notes, built
  every step except #558, and made every correction after reviews and acceptance
  runs.
- A Codex worker built #558.
- Claude reviewed #558. Codex reviewed the design notes, the main-thread work and
  the closing integration.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design notes with break maps for the risky steps | The bundled design review failed with 7 blockers. They were resolved in the notes before implementation, and the verify passed. |
| #555 | Update dialog and run feedback: external env drift as a complete status, one result message, readiness and restore consent reset on baseline changes | The first review failed with 4 blockers. Fixed in `d1bc3341`; the verify passed. |
| #558 | ID-addressed runtime targets read only their own row | Passed. |
| #556, #557 | Problem count; lists wait for every host in scope; capability cache refresh; lazy stack menus; volume dialog choices across host status changes | Round 1 failed with 2 blockers: missing producer-to-page lifetime guards and unguarded menu command paths. Round 2 left one blocker: the cancellation seam and the scope change were untested. Round 3 passed. |
| #559, #560, #561 | Named proxy confirmation; shorter copy; compact phone layout | Self-checks and CI; covered by the closing review. |
| all | Closing integration (#563) | The closing review failed with one blocker: for simple users, the Stacks totals never settled, because the assigned-stack reader did not name the hosts that answered. Fixed in `2f5586d7`; the verify passed. |

Every blocker fix and break-map row has a test that fails on an assertion when
its guard is reverted. One exception is the phone filter overlap. It is a layout
property that happy-dom cannot measure, so a Chromium measurement guards it
instead.

Two main-thread decisions were posted on #562:
- AUTO-1: the prune dialog marks only volumes that arrive through a host change
  as new.
- AUTO-2: Appearance and Explanations are titled "Personal settings".

## Findings during acceptance

- **Candidate, X-D1.** At 390 px in German, the Images usage filter labels
  overlapped by up to 7.7 px. The phone toolbar let the filter segments shrink
  below their text width. Fixed in `2716d1dd`: segments keep their width and the
  row scrolls. Delta 1 passed.
- **Candidate, X-B4.** On a cold start with 108 stacks on a slow hub-local host,
  the Stacks page showed "No stacks yet" with zero totals while that host had not
  answered. Stacks lacked the wait-for-every-host rule that the other lists
  received in this milestone. Fixed in `2716d1dd`; delta 1 passed in three cold
  attempts.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
- Application actions used real UI gestures in Chromium.
- API reads, request counts, DOM event traces recorded from before hydration,
  and fixed-viewport measurements witnessed their effects.
- No cell was excluded.

| Cell | Behaviour |
| --- | --- |
| X-A1 | Prune choices and the create-volume draft survive a D offline/online sequence; the confirm payload holds only chosen identities |
| X-A2-D, X-A2-E | External change of a recorded env file shows the drift and **Deploy now**, with no install or restore submit; the original bytes are restored |
| X-A3 | A container whose runtime condition is down counts as a problem; an ordinary stopped one does not |
| X-A4 | After an update finishes and the run window closes, no submission dialog remains |
| X-A5 | Containers, Images and Volumes on a cold start show no empty text and no zero totals before the first row from E; true empty and failure results still settle |
| X-A6 | "Check for updates" in the list menu runs in place |
| X-A7 | State-dependent stack controls recover after a run ends and after a stream reconnect, on H and E |
| X-B1 | Deleting a proxy route takes one named confirmation (E) |
| X-B3-D, X-B3-E | Actions on another stack succeed with an unreadable stack present |
| X-B4 | 108 stack rows: zero stack-scoped reads before the first menu gesture, one read per distinct opened stack, and a truthful list on cold starts |
| X-C-en, X-C-de | Copy subchecks: personal settings header, notification picker hint, named stack link, local host, restore hints and consent, one terminal status, easy-mode hint, Appearance order |
| X-C6-H, X-C6-D, X-C6-E | Run output, copy and download without the `{"log": …}` frame, for runs from before and after the update |
| X-D1 | Phone lists at 390×844: first row at or above 450 CSS px, no horizontal page scroll, no label overlap or clipping |
| X-D2 | Phone dashboard: attention summary above the ring |
| X-D3 | Phone users: no overflow; every action in the card menu; self and last-admin locks |
| REG-H, REG-D, REG-E | Standard regression per host |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `9967b32a` (baseline, main before M35) | Witnesses recorded | No verdict and no exclusions |
| `8ccb71b3` (early smoke) | Done | Runner corrections only |
| `8ac6e548` (candidate) | Failed, 21/23 | X-D1 and X-B4 product defects |
| `2716d1dd` (delta 1) | **Accepted 5/5** | X-A1, X-B4, X-C-en, X-C-de, X-D1 |
| `2f5586d7` (delta 2) | **Accepted 2/2** | X-C-en, X-C-de after the settings line change |
| `fa84f185` (exact main) | **Accepted 23/23** | REG-H, REG-D, REG-E and the cold part of X-B4 rerun; the other cells carry their passing evidence |

Exact main also recorded one observation outside the matrix. A simple user
assigned one stack on H and one on D saw settled numeric Stacks totals, both with
all hosts and with only D selected.

During delta 1, the runner's own density cleanup removed labelled sandbox
workload containers. The runner recreated them and verified them against
backups before running any cell. No product code was involved.

## Limits

- No cell was excluded.
- Chromium only.
- If a host's inventory read exceeds the 30-second read budget, that host settles
  as failed and the inventory notice names it. The three delta 1 cold-read
  attempts completed before that budget.
- Agents older than M35 report env drift without revisions. Hub and web show the
  status, but they offer no restore consent until the agent is updated.
- One web test (B10 hub update hint) failed once in a combined local run and
  passed on rerun.
