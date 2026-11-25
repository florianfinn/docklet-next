# M41 update follow-through acceptance — 2026-11-25

The accepted package is main `2d77a2a3945e907d57ad1da9b4eac8364c4892ce`, the
squash of the [M41 closing PR #629](https://github.com/florianfinn/docklet-next/pull/629)
onto main `6dc80212`. Its tree is identical to the feature head `476829d3`,
which passed the last delta run. The sandbox kit built every package locally
from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M41 covers the feedback round of 2026-11-25 on v1.7.0: #624 (version hints and
restore points after an agent self-update), #625 (tailnet reconnect notice),
#626 (readable update-check runs and names instead of ids) and #627 (stacks
whose image is not on the host). A read-only diagnosis on the maintainer's hub
showed that #627 came from saved, undeployed stacks without local images, not
from a registry or tunnel fault.
- The Claude main thread planned the milestone, wrote the design notes,
  implemented the foundation, the update panel and the tunnel notice, and made
  every correction after reviews and acceptance runs.
- Codex workers implemented the web presentation (S2) and the producer names
  (S3). Codex reviewed the design, the foundation, the acceptance fix and the
  closing PR; Claude reviewed the web presentation. The sandbox runs were
  delegated to Codex.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design note, plan and matrix with break map | The design review failed with 6 blockers: no real witness for a waiting phase, counts carried only on list summaries, plans not preserved at settlement and retry, two acceptance cells with a fixed denominator or an unproven fault, and a tools assertion the sandbox could not meet. All were folded in before kickoff. |
| #626, #627 | S1 foundation: check plans, target names, missing-image skip, check aggregate, waiting witness, job activity counts, agent contract 24 | Failed with 1 blocker: a backing-off retry ahead of a new run made it show a waiting phase. The witness now follows the dispatch selection; count-only updates add no message rows. The verify passed. |
| #626 | S2 web presentation | Passed; three non-blocking findings (duplicate count line, malformed target ids, fixture shape) were fixed. |
| #626 | S3 producer names | Self-check and CI. |
| #624, #625 | S4 version hints refresh and reconnect notice | Self-check and CI. |
| #624, #626 | Acceptance fix: restore points after recovery, monotone count | Passed. |
| all | Closing integration (#629) | Failed with 2 blockers in the reconnect notice: success while a hub or peer link was still failed, and a replacement registration inheriting the old interruption. Both fixed; the verify passed. |

Assertion-failure red proofs cover the reviewed fixes and the individually
tested guards. Break-map evidence is still incomplete for a durable-store
restart, real retry and recovery rediscovery, a host invalidated mid-check,
decoding of older persisted outcomes, the complete Docker-to-hub producer chain
and a dedicated invariance test for the new skip; #630 tracks these.

Maintainer decisions: D-1 the disconnect notice turns into the reconnect notice
in the same toast; K1 a missing local image is a skipped target; K2 a check
without any success ends failed (check jobs only); K3 a bounded list of
activities gets names; K4 release 1.8.0 after acceptance, no deploy; Q-A1 the
delta-1 count cell is judged by the matrix rule. Main-thread decisions AUTO-1
to AUTO-5 are posted on #628.

## Findings during acceptance

- **Candidate, A6.** After a healthy agent self-update without reload, the
  orphaned restore points section kept "The host cannot be reached": its read
  had failed with 502 during the agent restart and was never repeated. Restore
  points now follow the reported agent version and are read again when the
  update settles, like the version hints.
- **Candidate, observation.** At the end of an all-host check the checked
  number dropped from 30 to 26 of 31, because running counts included failed
  and skipped targets. The checked number now counts successes throughout
  (AUTO-5).
- **Delta run 1, A1.** The run brief added a same-moment equality between the
  toast and the producer that the matrix does not require; the display lagged
  by up to two successes for a few seconds. The maintainer decided Q-A1: the
  matrix rule applies, and the lag is latency, which M41 does not assess.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
admin role, 1440 and 390 px. Fixtures per check: two deployed registry stacks
(one with a newer tag), one stack saved without deploy whose image was verified
absent on the host, and one standalone container; a registry delay held early
checks.

| Cell | Behaviour | Result |
| --- | --- | --- |
| A1 | Host check on E: growing "k of N checked" (N from the inventory), no queued phase, final sentence equals the run's counts, missing-image stack skipped with its reason | PASS (delta run 2) |
| A2 | Named rows with working stack and container links, tones and update marker in the Activity Centre and run window | PASS (candidate) |
| A3 | A1 on D (Direct parity) | PASS (candidate) |
| A5 | All-host check: "checked so far" until the plan is complete, then "of 31" | PASS (candidate) |
| A6 | Real agent self-update on D from 0.90.0 to 0.90.1 without reload: hints show the new version, no stale error | PASS (delta run 1) |
| A8 | Network, stack edit, single-image scan and hub backup activities show names | PASS (candidate) |
| REG | Standard regression on H, D and E | PASS |

Exclusions agreed at kickoff: an all-failed check (Go producer tests), the
tailnet reconnect transport (no sandbox host uses the tailnet route; tests run
through the real hosts stream), and the private sandbox tools image, which the
product classifies unknown by design.

## Results

- Candidate `9654fed6`: NOT ACCEPTED 6/7 (A6).
- Delta run 1 `6d8cefd6`: NOT ACCEPTED 6/7 (A1 brief witness, see Q-A1); A6
  and the Direct regression passed.
- Delta run 2 `476829d3`: ACCEPTED 7/7.
- Exact main `2d77a2a3`: ACCEPTED 8/8 (empty tree witness, kit smoke,
  regression on H, D and E, M41 smoke on E at both widths, contract 24).

## Limits and follow-ups

- Follow-up #630 tracks the evidence limits named by the reviews (durable
  restart, real retry and recovery, host invalidation mid-check, the full
  producer chain through both proxies, skip invariance, old state decoding,
  projection budget) and small presentation refinements.
- Agents at contract 23 (1.6.0, 1.7.0) stay compatible; their own local job
  runs show no counts.
- A cleanup error in delta run 2 lost four dormant sandbox fixture container
  definitions from an earlier milestone and their complete configuration
  archive. Their bind data was kept, but the fixtures were still not fully
  restored after the exact-main run. Product and package restoration and the
  scored acceptance witnesses passed.
