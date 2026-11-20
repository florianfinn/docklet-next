# M30 hub update experience and peer stack confirmation acceptance — 2026-11-20

The accepted package is main `3c00052a074f4733c24e00ce9a564a34189a75af`. It
contains the [M30 closing PR #530](https://github.com/florianfinn/docklet-next/pull/530)
(squash `9531a5a3`) followed by the M28 closing PR #535. The later main commit
`c0e2da4c` adds only the M28 record. The sandbox kit built every package locally
from the exact source.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment.

## Integrated steps

All steps were pushed to one feature branch as commits, without step PRs, and CI
ran on every pushed head.
- The Claude main thread planned the milestone, built S1 and the logo, and made
  every correction after reviews and acceptance runs.
- Codex workers built S2, the address web part of S3 and S4.
- Claude reviewed the Codex-built steps; Codex reviewed the main-thread work and
  the closing integration.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #529 | Design note with break map | The bundled design review failed with 8 blockers; its verify still had 4 unresolved. Design v2.1 addressed them. |
| #529 | S1 foundation: setup without a code, discovery `origins`, address write validation | The first review failed with two blockers, which were fixed; the verify passed. |
| #529 | S2 web: hub update view, navigation hint, styled gate screens | The first review failed with one blocker, which was fixed; the verify passed. |
| #529 | S3 logo colours and local hub addresses | The address review failed on bare IPv6 in links and discovery; its verify found mapped IPv6 and bracketed IPv6 legacy values. All were fixed and covered by the closing review. |
| #531 | Design for unreadable peer stacks | The design review failed with 6 blockers; its verify had one unresolved and one new blocker. Design v3.1 addressed them. |
| #531 | S4 peer confirmation, agent contract 20, migration 006 | The first review failed with two blockers: unsafe stack names dropped the confirmation, and **Run anyway** was offered where the request cannot carry it. Both were fixed; the verify passed. |
| #531 | Fix for confirmed remote runs (found in acceptance) | The review passed without findings. |
| #529 | Closing integration (#530) | The closing review failed on the IPv6 setup prefill; the verify passed. The delta round over #531 failed on compatibility and release text still naming contract 19 and no migration; the verify passed. |

CI caught three more defects on the feature branch: root contract tests still on
contract 19, the frozen schema baseline missing migration 006, and a proxy upgrade
test that ran later migrations. CI hid the last one behind a truncated test log;
#536 tracks that.

## Findings during acceptance

- **Candidate, A3.** Both packages came from the same source and served an
  identical bundle, so "asset hash differs" could not pass. Maintainer decision
  Q-A3: build the lower package from the earlier head; delta 1 passed.
- **Delta 2, A10b.** A confirmed stop on a Direct agent removed the containers,
  but the hub run stayed "outcome unknown": hub and agent stored the audit entry
  under the same ID, and the copy of the agent messages failed. The hub now keeps
  its own entry. The audit was also stored but not shown in the run details; it
  now is. Delta 3 passed on Direct and Edge.
- **Exact main, UP.** Four old activities were missing after the upgrade,
  removed by the existing retention limit of 100 runs per host at startup.
  Maintainer decision D-X1: preserved means every row the configured retention
  keeps; a read-only check showed exactly the rows beyond the limit were removed.
- **Exact main, A1.** The reused package was a development build without a
  release version, so no hub update could be offered. Maintainer decision D-X2:
  rerun on a test package with a release version built from the exact main
  source, on H only.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
Tested application actions used real UI gestures in Chromium; API reads and
container inspection witnessed their effects. Feature cells ran in German at
1440 px; the standard regression kept H in German and D in English at 1440 px and
E in German at 390 px. Fixture preparation and restoration were separate sandbox
operations.

| Cell | Behaviour |
| --- | --- |
| A1 | Admin badge at Settings and at the Updates section with the version; click focuses the hub update control. A simple user gets no badge and no version read |
| A2 | The update view appears at confirm and stays through the real hub outage |
| A3 | Done after the healthy settlement, reload 10 s later, new bundle, session kept |
| A4 | Styled gate screen with automatic retries while the hub is stopped |
| A5 | An unhealthy target ends on the failed view, without a reload |
| A6 | Fresh hub: setup without a code; a second claim is refused |
| A7 | Logo violet and cyan under a custom palette, light and dark |
| A8 | Separate IP and domain fields, legacy hint, validation without a write |
| A9 | Standard regression on H, D and E |
| A10 | Distinct peer runs without a dialog; unreadable peer asks, Cancel changes nothing, confirm runs once with a visible audit (D and E); proven duplicate refused |
| UP | Pre-006 hub data upgraded to the exact main package |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `ddeff0d0` (candidate) | Failed, 8/9 | A3 failed (identical bundle) |
| `2142e7ab` (delta 1) | Passed | A2 and A3 with the lower package from `ddeff0d0` |
| `060f8d10` (delta 2) | Failed | A10a, A10c and A9 (18/18) passed; A10b failed |
| `da95bc84` (delta 3) | Passed | A10 on D, A10b on E, regression on D |
| `3c00052a` (exact main 1) | Failed | Tree, A10 on D and E, regression passed; UP and A1 failed |
| `3c00052a` (exact main 2) | **Accepted** | UP under D-X1 and A1 under D-X2 passed; every other cell carried |

Exact main reran A1 and A10, because M28 changed the sidebar and the contract 20
fingerprint. The regression on D and E comes from the M28 exact-main run on the
same images. Unchanged passing cells carry their evidence forward.

## Limits

- The cold reload while the hub is stopped was excluded from A4 by agreement.
- Chromium only.
- The upgrade started from pre-006 data of a test package, not from a published
  release.
- M28 changed the contract 20 fingerprint after the M30 runs. Agents from
  earlier M30 test packages cannot pair with this package; no release shipped
  them.

## Open follow-ups

#536 (truncated Go test output in CI).
