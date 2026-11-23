# M37 feedback round 2 acceptance — 2026-11-23

The accepted package is main `7779ea9f5930b1987bbc9d3e3673864afeb5f149`. It is
the squash of the [M37 closing PR #588](https://github.com/florianfinn/docklet-next/pull/588).
Its tree is identical to the accepted feature head `7811e74f`. The sandbox kit
built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M37 covers the feedback round of 2026-11-23 on 1.4.0 (#579–#586). All steps
were pushed to one feature branch as commits, without step PRs. CI ran on every
pushed head.
- The Claude main thread planned the milestone, wrote the design notes, built
  host cards, the Unraid switch, CPU bars, the loader, the Tailscale stepper and
  the agent contract 22 bump, integrated every step, and made every correction
  after reviews and acceptance runs.
- Codex workers built the setup test backend, the Action Centre titles with scan
  counts, and paging.
- Each implementation received a review from the other provider. The first
  review of the setup test backend (Claude) also covered the Claude main
  thread's contract 22 bump, so it was not provider-independent for that
  commit; a later Codex review covered the bump and its corrections.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design notes with break maps for the risky steps | The bundled design review failed with 8 blockers. The verify left 7 partly resolved; the kickoff decisions and revision 3 of the notes settled them. |
| #579, #581, #582, #580 | Host cards, per-host Unraid switch, fixed CPU bars, wordmark loader | Round 1 failed with 4 blockers. Three classes failed twice and received root-cause tables: freshness of host details, every view that names containers, and the evidence for the production HTTP path and lifecycle. The host details read was rebuilt around answers and failures stamped with the source state they were read under; a complete reader inventory classified every container view. The sixth round passed. |
| #586 | Setup test backend and retained candidate run read | Failed with 1 blocker: the candidate run read treated the answers of agents at contract 21 as errors instead of unavailable. Fixed; the verify passed, including a cross-provider review of the contract 22 bump. |
| #585 | Paged findings and folder views | Failed with 3 blockers in occurrence targets and page anchoring. Fixed; the verify passed. |
| #586 | Tailscale stepper and setup test UI | Failed with 2 blockers: results had to follow the credential revision, and an ambiguous mint had to be explained truthfully. Fixed; the verify passed. |
| #583, #584 | Action Centre titles, host targets and scan counts | Failed with 1 blocker: system backups and shell sessions were titled "Action". Fixed; the verify passed. |
| all | Closing integration (#588) | Both acceptance fixes passed their own reviews. The closing review on `30f576f6`, which includes the first fix, passed without findings; the second fix (`7811e74f`) was reviewed afterwards. |

Every blocker fix and break-map row has a test that fails on an assertion when
its guard is reverted.

Main-thread decisions AUTO-1 to AUTO-4 were posted on #587. AUTO-1 keeps scan
counters out of the shared activity list refresh; the dialog, toast and
Security box keep their own observed counts. AUTO-2 to AUTO-4 define which
views leave out per-user hidden Unraid containers: lists, counts, search and
proxy suggestions hide them. Operation consequence texts, security advisories,
platform coverage, explicit detail routes and proxy instances keep every
container.

## Findings during acceptance

- **Candidate, H3.** After a Direct agent restart, the open host card kept
  "not reported" values while the agent answered again; only a reload restored
  them. A failed read for the current source state never retried. Fixed in
  `30f576f6`: a failed read repeats every 15 s while the host is available and
  the card is open. Delta 1 passed; the card recovered 20.6 s after the agent's
  first successful answer.
- **Candidate, A1.** Update-check, Auto Heal, volume backup and redeploy rows in
  the Action Centre lacked the host name, and scheduler rows repeated their
  title as the target. Fixed in `30f576f6`; delta 1 passed in EN and DE.
- **T1, run 1.** On a real Tailscale account the setup test passed on the hub,
  but the UI showed a generic failure. The hub encodes JSON with
  `encoding/json/v2`, where `omitempty` keeps a zero number, so every passing
  check carried `providerStatus: 0`; the web decoder accepts only HTTP statuses.
  The web fixtures never sent the field. Fixed in `7811e74f` with `omitzero` on
  every tailnet provider status and a wire test through the hub's JSON format;
  the rerun passed.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
T1 used a real Tailscale account with a test OAuth client; the runs removed
every device and test key they created.
- Application actions used real UI gestures in Chromium at 1440×900 and
  390×844.
- API reads, Docker inspection, the provider's key list and heap measurements
  witnessed their effects.
- U1 ran with Unraid-labelled containers, because no Unraid host is connected
  (named environment limit, maintainer decision).

| Cell | Behaviour |
| --- | --- |
| H1 | Local host card: hub version and project directory; no agent-only rows (desktop and phone) |
| H2 | Each host card shows its own daemon's OS and Docker version |
| H3 | Agent stopped: unavailable values without a crash; after the restart the open card recovers without a reload |
| U1 | Per-host Unraid switch only on hosts with Unraid containers; ring, counts and lists drop exactly that host's containers; persistence and switch-back |
| B1 | Update overlay: looping wordmark loader while running, static logo when done; light theme, reduced motion and phone width |
| B2 | Host chart draws 60 fixed-width bars from the first sample; placeholders muted, real bars in the host colour |
| A1 | Action Centre in EN and DE: specific titles and the host name on every row; no bare zero; raw output only where a run has output |
| A2 | Manual scans on Direct and Edge: increasing "X of Y" counts in dialog, toast and Security box; final counts persist |
| S1 | 12,787 findings at phone width: 100 rows per page, a deep link to an occurrence on page 50 |
| S1-mem | Heap growth while paging stays at or below 150 MB |
| F1 | Folder with 10,000 files: 200 rows per page, a file on page 10 opens, a folder change resets the page |
| C1 | Connection check rows carry a specific title, the host name and the persisted action |
| T1 | Real Tailscale account: stepper in EN and DE at desktop and phone width, setup test with six passing checks and the verified mark, test key tagged, one-use, not preauthorized, 300 s, deleted after each test |
| REG | Standard regression on H, D and E |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `c74c3334` (early smoke) | Done | Build, regression, update-overlay and heap methods |
| `bf8a3a14` (candidate) | Failed, 11/13 | H3 and A1 product defects; T1 separate |
| `30f576f6` (delta 1) | **Accepted 2/2** | H3 and A1 |
| `30f576f6` (T1, run 1) | Failed | Provider status zero on the wire |
| `7811e74f` (T1, run 2) | **Passed** | Part B left out, see Limits |
| `7779ea9f` (exact main) | **Accepted 6/6** | Smoke and regression on H, D and E; the other cells carry their passing evidence on the identical tree |

S1-mem measured 24.8 MB of heap growth with 100 mounted rows in all three
cycles. The base package `34fb271f` crashed the renderer on the same data.

## Limits

- By maintainer decision, T1 did not check a refused agent key on the real
  account (an OAuth client without `tag:docklet-agent`). Unit and render tests
  cover that path; the real-account check is tracked in #589.
- U1 ran with Unraid-labelled containers on sandbox hosts; a real Unraid check
  is tracked in #589.
- Chromium only.
- Agents at contract 21 show candidate scans without counts.
- A saved Tailscale credential can only be replaced, not removed (#589).
