# M39 feedback round 4 acceptance — 2026-11-25

The accepted package is main `a24e00e98c5648437c814288e0d959483025772b`. It is
the squash of the [M39 closing PR #614](https://github.com/florianfinn/docklet-next/pull/614).
Its tree is identical to the feature head `4f51ac33`, the last package with a
delta run. The sandbox kit built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment. By maintainer decision M39 has no
release of its own; 1.7.0 ships it together with M40.

## Integrated steps

M39 covers the feedback round of 2026-11-24 on 1.6.0 (#609–#612). It changes
the web client only, with no hub, agent, OpenAPI or migration change. All steps
were pushed to one feature branch as commits, without step PRs. CI ran on every
pushed head.
- The Claude main thread planned the milestone and wrote the design notes. It
  built the run-summary read, the result lines and the grouped scan
  notification. It integrated every step and made every correction after
  reviews and acceptance runs.
- A Codex worker built the security overview changes.
- Each implementation received a review from the other provider: Claude
  reviewed the worker step, Codex reviewed the main thread's work and every fix.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design notes, plan and matrix with break maps | The design review failed with 11 blockers; all were folded in before kickoff. |
| #609, #610 | Security overview: finding labels, usage by images, scanned tile, fixable tile per setting | Passed; 3 small findings fixed (readable image total, plural, short German label). |
| #612 | Run-summary read per host | Failed with 3 blockers: cached counts survived a host re-registration, the demo contract was wrong, and guards were missing. A registration change now resets the cached summaries; the verify passed. |
| #611, #612 | Result lines and the grouped scan notification | Failed with 4 blockers: a deferred toast dismissal removed the next notification, partial candidate scans were not kept, a failed row hid its result, and guards were missing. Fixed. While writing the guards, the main thread found that a first list read still in flight missed a run that had just settled; the list is now read once more. |
| all | Closing integration (#614) | Round 1 failed with 2 blockers in the notification lifecycle: a toast id was reused across sign-in identities, and the success countdown started before the scan outcome was known. Being the second failing round in that class, it was fixed as a whole from a root-cause table. The verify found 1 more: a failed detail refresh kept the running copy. The fix-only verify passed without findings. |

Every blocker fix and break-map row has a test that fails on an assertion when
its guard is reverted.

Main-thread decisions AUTO-1 to AUTO-6 were posted on #613. AUTO-1 counts an
affected image once in its usage state. AUTO-2 groups every scan run shown at
the same time into one notification. AUTO-4 gives the grouped view one card
layout for every toast design. AUTO-5 leaves the demo adapter without run
summaries. AUTO-6 bounds the lifecycle fix to the scan-notification surfaces.

## Findings during acceptance

- **Candidate, N2.** A grouped partial row showed only the error and omitted
  its result line. This had already been fixed in `6b3376cd` before the
  candidate result arrived; the delta run confirmed it.
- **Candidate, N1.** At phone width the notification closed 10.2–10.7 s after
  the hub finished the last scan, outside the 8 s + 2 s bound. The UI saw the
  last result 2.7 s after the hub; its own countdown took 8.0 s. The matrix did
  not name the reference point. The maintainer decided that the bound counts
  from the moment the notification shows every scan settled (D-7).
- **Candidate, O2.** The no-access fixable tile cannot appear for a simple
  user, because the security page is administrator-only. The maintainer kept
  the access rule and decided to check the neutral tile through an
  administrator whose scan-settings read fails at the browser boundary (D-8).
- CI full lint failed once on `87efd624`: two test probes assigned a module
  variable during render. Fixed in `9c0d76be`.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
- Application actions used real UI gestures in Chromium at 1440×900 and
  390×844, in EN and DE.
- Real scanner workloads, reversible scanner holds and induced partial runs
  provided the scan states. Run reads, image lists and per-image finding counts
  witnessed them. No stored vulnerability report was edited or fabricated.
- Notification timing came from DOM samples every 500 ms.
- No cell was excluded.

| Cell | Behaviour |
| --- | --- |
| O1 | Overview on D: severity bar and critical/high tiles say "findings" and equal the API sums; the usage bar counts affected images per state; the scanned tile reads "X of Y images" over the readable images |
| O2 | Setting on: the tile explains that only fixable findings are stored and links to the setting; setting off: "X of N findings have a fix" with X < N; failed settings read: neutral count without share or link |
| N1 | Scan all hosts with E finishing at least 10 s after D: one notification, distinct-host header, one row per run, no countdown while a scan runs, self-close within 8 s + 2 s after all rows show settled |
| N1F | D ends partial: its row shows the failure and the result and stays until closed; a failure after closing reopens the notification with that run only |
| N2 | Result line equal to the run read in the notification, the collapsed Action Centre entry (also after reload) and at the top of Details |
| N3 | A single host scan keeps the ordinary notification with its result line; a single-image scan shows status only |
| REG | Standard regression on H, D and E |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `87efd624` (candidate) | Failed, 4/7 | N2 product defect (already fixed); N1 and O2 matrix wording settled by D-7 and D-8; regression 18/18 |
| `4f51ac33` (delta 1) | **Accepted 6/6** | Scanned tile, fixable tile including the failed settings read, self-close 8.5 s desktop and 8.0 s phone, partial persistence (76 s) and reopen, 40 result-line observations warm and cold, single scans |
| `a24e00e9` (exact main) | **Accepted 8/8** | Tree witness, smoke 21/21, regression on H, D and E, overview and Action Centre result line on D, public login; the other cells carry their passing evidence on the identical tree |

## Limits

- Single-image scans show status only, because the hub stores no run record
  for them.
- Collapsed entries show the result line for a host's newest 30 runs; older
  runs show it after expanding.
- The demo adapter offers no run summaries.
- When a host is unreachable, the overview's finding counts still include its
  stored findings while the severity tiles leave them out; this predates M39
  (#618).
- A fully failed host scan shares the persistence rule of a partial one and is
  covered by unit tests, not by a sandbox fixture.
- Chromium only.
- The live checks after the update to 1.7.0 are the maintainer's practical
  acceptance and are not part of this record.
