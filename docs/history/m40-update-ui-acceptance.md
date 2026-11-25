# M40 update UI acceptance — 2026-11-25

The accepted package is main `962086d0d036a9deb0865c37d4f21f61ceec2511`. It is
the squash of the [M40 closing PR #619](https://github.com/florianfinn/docklet-next/pull/619)
onto main `777d39c9`. It differs from the feature head `81fd8ea4` only by M39's
acceptance record in docs/history, which is not part of any package. The sandbox
kit built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment. Release 1.7.0 ships M39 and M40
together.

## Integrated steps

M40 covers #615 (compact Settings › Updates panel) and #616 (version badges and
update link on the host cards). It changes the web client only, with no hub,
agent, OpenAPI or migration change. M40 ran in parallel to M39. Main was merged
into the feature branch after M39 landed; the only shared file was the provenance
sidecar. All steps were pushed to one feature branch as commits, without step
PRs. CI ran on every pushed head.
- The Claude main thread planned the milestone, wrote the design notes,
  implemented every step and made every correction after reviews and acceptance
  runs.
- Codex reviewed the design, the red step, the acceptance fixes and the closing
  PR. The sandbox runs were also delegated to Codex.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design note, plan and matrix with break map | The design review failed with 6 blockers: the per-host cache was not tied to the host's registration identity, the current tone could certify a different installed version than the check reported, a request guard had no red proof, and three acceptance cells had gaps or ambiguous witnesses. All were folded in before kickoff. |
| #615, #616 | Shared check and badge model, compact panel, host card badges and link | Failed with 1 blocker: a manual-commands toggle survived a change of the panel's default state and reappeared on return. The toggle now resets when the default changes; the verify passed. |
| #616 | Acceptance fix: the focused panel stays in view after an anchor jump | Failed with 1 blocker: scrolling resumed when focus left the panel and returned. Keeping the panel in view now also stops when focus leaves it; the verify passed. |
| all | Closing integration (#619) | Found no implementation defect. Its only blocker was the simple-user acceptance cell H3 (see below), resolved by the maintainer. The fix-only verify passed. |

Every blocker fix and break-map row has a test that fails on an assertion when
its guard is reverted.

AUTO-1, a planning decision, had the main thread implement every step itself.
Main-thread decisions AUTO-2 to AUTO-6 were posted on #617.
- AUTO-2: for the empty orphan-list check, the hub's historical orphaned restore
  points were removed through the product and then restored from a backup.
- AUTO-3: a sandbox preference section stays at its default.
- AUTO-4: the second hub check on a cold Hosts load is follow-up #621.
- AUTO-5 and AUTO-6 corrected the exact-main method (see Results).

## Findings during acceptance

- **Candidate, H2, H2-E and R1.** At desktop width the focused update panel
  ended up below the viewport after a card link or the hub anchor. The
  section's top was at 1951 px and 1765 px in a 1000 px viewport. Content above
  it finished loading later and pushed it down. The panel now stays in view
  while the page above it changes height. This stops at the first user input,
  when focus leaves the panel, or after five seconds.
- **Candidate, H3.** The accepted simple-user design keeps the Hosts page
  administrator-only, so a simple user never sees host cards. The maintainer
  kept the access rule and revised H3 to that design (D-7). The design note now
  states the boundary.
- **Candidate, U1.** The sandbox hub carried 22 orphaned restore points from
  earlier milestones, so the empty-list case had no fixture (AUTO-2).
- **Exact main, first run.** The run brief expected the hub card in the
  current tone from the normal kit package, which carries no version stamp and
  correctly shows a neutral "Development" badge. The rerun used the matrix's
  0.90.1 stamp (AUTO-5). The hub regression was blocked before any gesture: the
  AUTO-2 restore had left the kit's regression project unreadable to the hub
  worker. Ownership was repaired after a backup (AUTO-6). Neither was a product
  defect.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
- Packages were stamped from the same source: 0.90.1 for H and E, 0.90.0 for
  D (and E in the Edge phase), with a private release feed whose newest
  version is 0.90.1.
- Application actions used real UI gestures in Chromium at 1440×1000 and
  390×1000.
- A pathname-only request counter attached before navigation counted the
  update checks.
- Anchor cells waited for zero pending reads and at least 800 ms of request
  quiescence before measuring focus and position.
- No cell was excluded.

| Cell | Behaviour |
| --- | --- |
| U1 | Hub panel current: success badge, Update disabled, manual commands collapsed, no agent hint row on the hub host, shortened reference with full copy, no orphan heading for an empty list |
| U2, U2-E | Outdated agent panel (Direct, Edge): neutral installed and highlighted target badges, Update primary; the confirm dialog names the target and Cancel sends nothing |
| H1 | Admin host cards: hub and E current, D outdated with a target link; each card shows its version once |
| H2, H2-E | The card's target link opens that host's panel, which is in view with its Update button focused; no other panel scrolls or takes focus |
| H3 | Simple user on Hosts (revision 3): the administrators-only notice, no cards or update links, zero update checks |
| R1 | Direct `#hub-update` navigation focuses the hub panel's heading in view |
| Q1 (advisory) | A cold admin Hosts load issues at most one check per online host |
| U3, H1-C | Unreachable Edge agent: check-failed panel with a reason, Update disabled; its card neutral without a target or check |
| REG | Standard regression on H, D and E |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `69ae087a` (candidate) | Failed, 7/12 | U2, H1, Q1, U2-E, U3, H1-C and REG passed; H2, H2-E and R1 desktop position fixed; U1 fixture gap; H3 settled by D-7. Q1 counted two hub checks (advisory, #621) |
| `ac787b22` (delta 1) | **Accepted 4/4** | U1 empty list, H2, H2-E and R1 at both widths |
| `81fd8ea4` (delta 2) | **Accepted 1/1** | H3 under revision 3 at both widths, on the delta 1 package (Markdown-only change) |
| `962086d0` (exact main) | Failed, 5/8 | Method and fixture faults, no product defect (AUTO-5, AUTO-6) |
| `962086d0` (exact main, rerun) | **Accepted 8/8** | Tree witness, smoke 21/21, regression on H, D and E, M40 smoke at both widths (hub 0.90.1 current, one badge per card, hub heading focused in view), public login |

## Limits

- A cold Hosts load checks the hub twice, because the hub update session
  refreshes the check once after recovery (#621).
- Simple users do not see host versions; the Hosts page stays
  administrator-only.
- Development builds without a version stamp show a neutral "Development"
  badge and no target.
- Chromium only.
- The live checks after the update to 1.7.0 are the maintainer's practical
  acceptance and are not part of this record.
