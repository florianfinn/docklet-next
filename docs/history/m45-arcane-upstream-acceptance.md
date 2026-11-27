# M45 Arcane upstream fixes and minimum image age acceptance — 2026-11-27

The accepted package is main `6e6d0ad5ac0eaae319070015f33b9b3982c1507c`: the
[M45 closing PR #689](https://github.com/florianfinn/docklet-next/pull/689),
squashed from head `a9b8a59d`. The sandbox kit built every package locally from
the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M45 ports the Arcane upstream fixes that apply to Docklet and adds a minimum
image age for scheduled updates (#667-#680). Agent contract 26 with migration
009; the minimum agent version stays 18.

- The Claude main thread planned the milestone, implemented the volume step
  (S1), integrated every worker step and made every correction after reviews
  and acceptance.
- Codex workers drafted security (S2), compose and image fixes (S3a),
  scheduler and backups (S3b), the minimum-age backend and contract (S4a) and
  its web part (S4b). Codex reviewed the main thread's work and the closing PR;
  Claude reviewed the worker steps. The sandbox runs were delegated to Codex.

| Step | Review outcome |
| --- | --- |
| Plan, matrix and break maps | Design review round 1 failed with 9 blockers and round 2 (reworked minimum-age design) with 5; all were folded in before implementation. |
| S1 volumes (#667) | Failed, fixed; the verify passed. |
| S2 security (#668-#672) | Passed. |
| S3a compose, images, prune (#673, #674, #677-#679) | Failed with 2 blockers (a tag check failing on lookup errors, restart dependents expanded after verification); both fixed. |
| S3b scheduler and backups (#675, #676) | Failed with 1 blocker (startup repair cleared `@daily`-style schedules); fixed, the verify passed. |
| S4a minimum age backend and contract 26 (#680) | Failed with 1 blocker (a missing guard for the unknown-age fallback); fixed. The verify also covered a shutdown-result fix for #675 and failed twice: completed partial results were dropped, then a determined auto-heal failure could recover as succeeded. Interruption is now signalled by the worker's own error; the fix-only verify passed. |
| S4b web (#680) | Failed with 1 blocker (an age hint shown with the default setting 0); the age line now appears only for scheduled policies while the setting is above 0. |
| Closing integration (#689) | Passed with no findings. |

Assertion-failure red proofs cover the reviewed fixes and the break-map rows.

Maintainer decisions at kickoff: Q-1 mixed stacks install partially, with the
checked image identity bound to the executor; Q-2 one global setting, default
0 (off), age measured from Docklet's first observation; Q-3 overlapping
scheduled backups wait without a cap; Q-4 the run ends at the READY closing PR.
During the run: Q-5 the practical moved-tag race subcell of C5 is excluded and
decided by the real-transport tests.

Main-thread decisions: AUTO-1 a per-feature contract gate 26 instead of a
higher minimum agent version; AUTO-2 compact expected candidates within the
existing intent size limit; AUTO-3 the platform tag filter keeps a tag on a
lookup error, with at most 8 refused lookups per check; AUTO-4 the job-schedule
route keeps accepting schedule descriptors; AUTO-5 the web age line only for
scheduled policies with a minimum age above 0.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
all on the candidate package with contract 26.

| Cell | Behaviour | Result |
| --- | --- | --- |
| C1 | Anonymous volume and its file survive image update (H/D/E), redeploy and an environment-only edit (H); named and bind mounts unchanged | PASS (run 1) |
| C2 | A forged `X-Forwarded-For` from an untrusted private peer is not recorded; the trusted gateway still forwards the client | PASS (run 1) |
| C3 | Agent token of a disabled environment: 200, 401 while disabled, 200 after re-enable (D/E) | PASS (run 1) |
| C4 | Direct image pull with progress through the UI completes | PASS (run 1) |
| C5 | Minimum age 2 days: in mixed stacks only the aged service installs (H/D/E), "Waits until" dates, young containers skipped and aged ones installed, manual update immediate, check-only and setting 0 without age text, setting 0 installs at once, first-seen dates survive a restart | PASS (run 1) |
| C6 | An overlapping scheduled hub backup waits, then succeeds with its own archive | PASS (run 1) |
| C8 | A builder without `pull_policy` and a sibling using its image deploy without pulling the built image; a `restart: true` dependent is recreated | FAIL (run 1), PASS (run 2) |
| C9 | Kit regression set H/D/E | PASS (runs 1 and 2) |

Exclusions agreed at kickoff, covered by automated tests: the #672 duplex
race, the #675 shutdown race, the #678 bare-address network list (the sandbox
daemon refuses that shape), and the header, role and timing items of #669 and
#670. During the run the maintainer also excluded the moved-tag race of C5
(Q-5); the real Direct and Edge transport tests decide it.

## Results

- Candidate `72a33af5`: NOT ACCEPTED. C1-C6 and C9 passed; C8 failed in all
  three fresh deploys because the deploy default pull policy removed the build
  fallback of the builder; the CI gate failed on a stale web fixture.
- Delta 1 `6c07a163` (after the build-fallback fix and the fixture update): C8
  passed in three fresh deploys and on redeploy, the dependent restart passed,
  regression smoke 21/21. Its CI run was superseded by the push of the final
  head `a9b8a59d`, which adds only main's docs commit; the full checks passed
  there.
- Exact main `6e6d0ad5`: ACCEPTED. The tree differs from delta 1 only by the
  two docs files of main's commit 8984d4af, so C1-C6, C8 and C9 carry their
  PASS. Deployed over the run 2 data with migration 9 applied, D and E online on
  contract 26, regression smoke 21/21 and the regression set on H/D/E passed,
  and a C5 spot check showed setting 0 without age text; the main push checks
  passed.

In run 1 the hub's container window first collided with the hub's still
running stack window and reported that stack as its blocking target, the
existing per-host sequencing. The runner judged it a setup collision and
decided the cell from an isolated container window; both runs were kept.

## Limits and follow-ups

- The kit's upgrade baseline is 1.0.0, so the exact-main run deployed over the
  run 2 data instead of upgrading a fresh v1.10.0 installation; migration 009
  is covered by the schema upgrade tests.

- #691 tracks the nonblocking review notes: sign-in timing paths, test breadth
  for transport and old-agent rows, per-candidate first-seen reads, duplicated
  minimum-age docs, an over-broad refresh after saving the setting, producers
  that fold a cancellation into a partial result, and three flaky tests.
