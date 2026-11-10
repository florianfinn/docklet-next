# M21 first release readiness acceptance — 2026-11-10

The accepted package is main `5b4ab4c29478518ac638310d52073630d01c9142`, the
squash merge of the [M21 closing PR #442](https://github.com/florianfinn/docklet-next/pull/442).
Its tree equals the merged feature head `7f6ce39b`. The acceptance runs used
`1.0.0-rc.N` dry-run packages built from those trees.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment. Publishing the first release needs its own
approval, after the release package follow-up #446.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Risky steps had interim reviews,
and implementation and review always came from different providers:

- **Codex GPT-6.1 Sol workers** implemented the migration baseline, the release
  package, the in-app updates (backend and web), the header help, the
  vulnerability detail, the candidate scans (backend and web), the documentation
  and both acceptance fix packages. Claude Opus reviewed them.
- **The Claude main thread** wrote the design notes, the version, CLI and feed
  step, the Edge destroy fix, the pattern-tag update, the failure notices and
  the review fix rounds. Codex Sol reviewed them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #361 | S1 release baseline `001`, pre-release refusal `database.prerelease` | R1 passed with 6 non-blockers. Five were fixed; the removal of the old update-all routes moved into S8. |
| #362, #235 | S2/S3 version and Edge protocol marker 1, CLI `upgrade --image`, release feed | R1 failed with 5 blockers; the fix verify passed. |
| #362 | S4 scratch images (linux/amd64), OCI builder and uploader, package renderer, release workflow | R1 passed with 9 non-blockers. Six were fixed; three went to #446. |
| #434 | S6 Edge stack destroy body | R1 failed with 1 blocker (real handler and tunnel coverage); the fix verify passed. |
| #409 (item 3) | S7 pattern-tag stack update installs the selected tag | R1 failed with 5 blockers and R2 with 1; the verify passed. |
| #439 | S8 in-app hub and agent updates | Backend: R1 failed with 1 blocker, the first verify with 1, and the second verify passed. Web: R1 failed with 1 blocker; the verify passed. |
| #436 | S9 Security header help | Checked in the closing review. Touch support went to #449. |
| #437 | S11 vulnerability detail sheet and page | R1 passed with 7 non-blockers. Six were fixed; one went to #445. |
| #438 | S12 candidate scans and verified updates | Backend: R1 failed with 1 blocker; the verify passed. Web: R1 failed with 2 blockers; the verify passed. Deferrals went to #445. |
| #443 | Failure notice for failed container updates | Checked in the closing review. |
| #363 | Installation, upgrade and compatibility documentation | The closing review checked about 110 claims against the code. Six wording findings were fixed in `7f6ce39b`. |
| #364 | Two acceptance fix packages | The risk review passed with 3 non-blockers. Two are tracked in #447; one needed no change. |
| #365 | Closing integration (#442) | Both closing reviews passed without blockers. |

## Defects found before acceptance

An early smoke and the first candidate run on the integrated feature head found
these defects. All were fixed before the merge:

- the hub added a field to candidate scan requests for Direct and Edge agents,
  which then refused them with 422;
- after an in-app hub update, the existing browser session was signed out. A
  token from the previous release was treated as revoked; signature, expiry, user
  and session state now decide;
- the install sheet refused the accepted operator override
  `DOCKLET_AGENT_IMAGE=…:latest` as invalid discovery data;
- for standalone (non-Compose) containers, stored update checks did not reach
  the container detail or the candidate scans, so "Update container" and their
  candidate proofs were missing (an older gap since M10);
- after a failed, non-starting standalone update, no manual recovery to the
  previous image was offered (an older gap since M18);
- a stack update of the Docklet container was refused with
  `update.invalid-target` instead of `image.system-protected`;
- an assigned simple user could restart a stack but got 403 reading their own
  run;
- at 390 px the global search hid an exact container and volume name behind
  earlier fuzzy matches.

The early smoke also showed an update refused as busy right after a successful
run. A diagnosis found a window in the browser run gate, unchanged since M20,
and corrected the acceptance runner's synchronisation. #431 tracks the remaining
user-visible window.

## Accepted scope and method

Hub and agent images were built from the exact package tree and installed in the
disposable sandbox after data backups. The sandbox had the hub-local host, one
Direct agent and one Edge agent, a local OCI registry for packages and fixtures,
and a mirrored Trivy database. The in-app update cells used two test-only stable
builds, 0.90.0 and 0.90.1, from the same tree, with the sandbox registry as the
release repository.

Package checks on each run:

- package checksums;
- the served bundle bytes matched the build;
- health UP;
- version `1.0.0-rc.N` in the UI and the CLI;
- both agents online at contract 17.

The matrix had 55 cells:

- fresh installation from the package and adding Direct and Edge hosts through
  the generated installation files;
- default pins and free operator overrides;
- the release baseline, and the refusal of pre-release databases on copies of
  earlier sandbox data;
- the upgrade from one release candidate to the next by following
  `docs/upgrade.md`;
- the release feed and the Edge protocol marker;
- in-app hub and agent updates, a failing target and the refusals for
  non-Compose and custom installations;
- the release workflow dry run with the publish job skipped;
- Edge stack destroy, German UI and the regression walk;
- Security header help, the vulnerability detail, candidate scans with real
  proofs on Direct and Edge, and the failure notice;
- the named tests of the exact package's CI run;
- a walk of the approved UI target, TGT1 to TGT25.

Playwright Chromium drove the production UI in German and English at 1440 and
390 px. API calls, Docker readbacks and read-only database copies prepared
fixtures and verified the effects.

The agreed substitutions covered:

- **publication itself:** sandbox OCI digest readback and a workflow-structure test;
- **real Unraid hardware:** live synthetic-label actions and named origin tests;
- **DST transitions, the 30-day capacity line and notification providers other
  than webhook and SMTP:** named tests on each exact-head CI run. For Discord,
  Signal, Slack, Pushover, Matrix and Google Chat, that evidence covers URL
  building only;
- **the public Docker Hub service and live third-party apps:** approved local
  fixtures;
- **other reverse proxies:** documentation only, backed by named
  authentication-mode tests.

No vulnerability-database or candidate-proof substitution was used.

## Result

| Package | Result | Cause and fix |
| --- | --- | --- |
| `e683d402` (candidate) | Not accepted, 46 PASS / 9 FAIL | The defects listed above, from the session loss onwards. Fixed by two bounded packages. |
| `c0d7d23d` (delta 2) | **Accepted**, 55 PASS / 0 FAIL | The 9 failed cells and the cells their fixes touched were rerun; the others kept their run-1 evidence. |
| `5b4ab4c2` (exact main) | **Accepted**, 55 PASS / 0 FAIL | Tree check against the tested head plus a documentation-only commit. Upgrade from rc.1 to rc.2 by following `docs/upgrade.md` on all three hosts, the corrected page names and TZ instruction, REG and a short regression on the hub-local host. The other cells were carried. |

## Limits

- The hub-local host held more images than the matrix density bound, and
  label-only cleanup could not lower it. This is an environment precondition, not
  a product verdict.
- Request-only events without a visible error remained, such as anonymous
  start-up checks and navigation aborts. They are advisory and tracked in #425.
- The run gate can still refuse a command for a few seconds after the blocking
  run finished, until its next observation (#431).
- On touch screens the condensed and card help icons do not open (#449).
- The scratch images contain no GPU utilities, so the UI shows no GPU values; this
  is a product decision, not a tested cell.
- Direct and Edge agents ran on separate Docker daemons on one shared kernel.
  Chromium only.
- After the merge, the main push check of PR text read a line in the squash
  message as a network address. An exact reviewed commit-message exception fixed
  it in #451 (`180b94c8`), and #450 tracks the guard correction.

## Open follow-ups

#445, #446 (blocks publication), #447, #448, #449, #450, plus #409 items 1 and 2,
#435 and #440 after the release.
