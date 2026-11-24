# M38 feedback round 3 acceptance — 2026-11-24

The accepted package is main `cd87d1e4e7853d9fd14107066dcad05c9753afab`. It is
the squash of the [M38 closing PR #603](https://github.com/florianfinn/docklet-next/pull/603).
Its tree is identical to the feature head `d218dd64`, which differs from the
last fully run package `7fae5341` only in one web test file. The sandbox kit
built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M38 covers the feedback round of 2026-11-24 on 1.5.0 (#469, #592–#601). All
steps were pushed to one feature branch as commits, without step PRs. CI ran on
every pushed head.
- The Claude main thread planned the milestone, wrote the design notes, built
  inventory isolation, the agent contract 23 bump and the polish items,
  integrated every step, and made every correction after reviews and
  acceptance runs.
- Codex workers built proxy adoption, the Tailscale changes and the update
  actions (container and stack checks, refusal explanations, the per-host check).
- Each implementation received a review from the other provider: Claude
  reviewed the worker steps, Codex reviewed the main thread's work and every fix.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design notes with break maps for the risky steps | The bundled design review failed with 13 blockers; the verify's remaining fixes were folded into revision 3 of the notes. |
| #600 | Inventory isolation | Five rounds failed. After round 3 the row boundary was rebuilt: every reader keys its entries per row, and every consumer counts failures under its own page kind. Round 6 passed. |
| all | Agent contract 23 | Failed with 2 blockers: the container check route was missing, and the refused-tag pattern applied to the list instead of its items. Fixed; the verify passed. |
| #592 | Proxy adoption | Failed with 2 blockers. Fixed; the verify passed. |
| #599, #598, #601 | Tailscale setup, refused tags, masked secrets, tailnet host details | Failed with 1 blocker, then 1 more: an identical save during the token wait was overwritten. Fixed; the third round passed. |
| #595, #594, #597 | Update checks and refusal explanations | The first part failed with 2 blockers. Its correction review found stale refusal predictions after a failed eligibility read. The per-host review found that an unknown admission was kept and never resent. A combined verify passed. |
| #596, #593, #469 | Dashboard filter, menu entry, schedule reads, job targets | Passed. |
| all | Acceptance fixes | The first failed with 1 blocker (stale candidates on build-only projects); fixed and passed. Partial counts passed. |
| all | Closing integration (#603) | Round 1 failed: eligibility offered an update beside an unchanged `${VAR:-…}` sidecar that admission refused. Round 2 failed on unrelated YAML anchors and round 3 on multi-line sidecars and the cost on a 256-service stack. Eligibility was rebuilt on the writer's own read-only validation; one parse brought the dense 256-service stack from 84 s to 3.3 s. Round 4 passed without findings. |

Every blocker fix and break-map row has a test that fails on an assertion when
its guard is reverted.

Main-thread decisions AUTO-1 to AUTO-11 were posted on #602; AUTO-8 was posted
late, at the close. AUTO-8 keeps deploy-point refusals without binding detail
(follow-up #604).
AUTO-9 keeps the first-run empty state when no host has stacks.
AUTO-10 explains a writer refusal of the whole file, such as any YAML anchor, as
a project binding that blocks every update. AUTO-11 skipped a sandbox run for
the final test-only commit.

## Findings during acceptance

- **Candidate, U2.** A project whose services are all built locally made the
  per-host check partial, because it reported no registry images. Fixed in `93948bfe` so build-only checks succeed. Review then found that this
  shortcut retained stale candidates; `e7329a87` treats the result as an empty
  observation and clears them.
- **Candidate, U3.** The build service's explanation disappeared once its peer
  became selectable. Fixed in `93948bfe`: the notice explains every current
  binding again.
- **Candidate, I1 and U4.** Network member counts included stopped containers
  with a configured attachment, and the client-internal read tag never reaches
  the wire. By maintainer decision the witnesses were revised (matrix
  revision 3): configured memberships count, and a unit witness covers the tag.
- **Candidate, T1.** The setup test failed because the maintainer's policy test
  for a refused agent key (T1B) was still active. After the restore, delta 2
  passed.
- **Delta 1, U2.** The all-hosts check ended partial on targets whose image
  exists only locally and showed only an error. By maintainer decision a partial check now reports its update
  counts and links to the run. It gives a failed-target count when target
  details are readable; the all-hosts check says that some targets could not be
  checked. Local-only images are a follow-up (#604). One more delta 1
  failure came from a sandbox registry relay that kept dead connections; it was
  diagnosed and repaired in the kit and was not a product defect.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
T1, T2 and H1 used a real Tailscale account with two test OAuth clients; the
maintainer removed the sandbox devices afterwards.
- Application actions used real UI gestures in Chromium at 1440×900 and
  390×844, in EN and DE.
- API reads, Docker inspection, registry delays and byte comparisons of Compose
  and env files witnessed their effects.
- No cell was excluded.

| Cell | Behaviour |
| --- | --- |
| I1 | A build-only, never-created Compose project beside running projects on D and E: Stacks, Dashboard, Hosts and Networks stay complete |
| I2 | The build-only stack's detail route renders its services and marks the build-only service |
| I3 | One unreadable row per list at the browser boundary: the rest of the host stays visible, and the pages name the unreadable count |
| P1 | Proxy container recreated outside Docklet with identical mounts: the instance follows it, also on an open page after focus returns |
| P2 | Changed mount source, rw→ro, or an unfinished proxy operation: adoption refused, old binding kept |
| P3 | Two registrations for one container name: the usable one opens, the stale one is marked and can be removed |
| T2 | Agent-only client: connection status and setup test name the refused hub tag and the way out |
| T1 | Hub-only client with the owner chain: stepper, masked saved secret with Replace, setup test with six passing checks in EN and DE |
| H1 | Host card of E on the tailnet route shows its MagicDNS name and tailnet address |
| H2 | Local agent check warns with the concrete fix when a non-root agent cannot write its project directory |
| U1 | Visible container and stack check with a fresh result, busy during the run, recorded in the Action Centre under its host |
| U2 | Per-host check scoped to its host; a second click joins the running check; all-hosts check reports its counts |
| U3 | Refusal explanations before any dialog: variable reference, build service, include, an unchanged `${VAR:-…}` sidecar beside an updatable peer, an anchored file |
| U4 | Update menu entry with icon and one-line hint; one hub jobs read per schedule reload; job targets with kind, target and host |
| R1 | Dashboard hides stopped units, keeps failed ones and names the hidden count |
| REG | Standard regression on H, D and E |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `81b5fe05` (candidate) | Failed, 11/16 | U2 and U3 product defects; I1 and U4 witnesses revised; T1 during the policy test |
| `93948bfe` (delta 1) | Failed, 4/5 | I1, U3, U4 and regression passed; U2 partial without counts |
| `275fa97a` (delta 2) | **Accepted 3/3** | U2, T1 with the real account, regression |
| `7fae5341` (delta 3) | **Accepted 2/2** | U3 after the closing-review rebuild, including a real update beside a byte-identical sidecar; regression on D |
| `cd87d1e4` (exact main) | **Accepted 2/2** | Smoke (21 checks, agent contract 23 on D and E, served bundle equal to the build) and regression on H, D and E; the other cells carry their passing evidence on a tree that differs only in one test file |

## Limits

- The in-place image writer still refuses Compose files with YAML anchors,
  aliases or merge keys; the stack page explains it before any update (#604).
- Containers whose image exists only locally make a check of their host partial
  (#604).
- Agents at contract 22 show no update explanations before a refusal.
- Network member counts include stopped containers with a configured
  attachment (maintainer decision).
- Chromium only.
- The live checks after the update to 1.6.0 (inventory rows, Tailscale setup,
  tailnet host details, proxy adoption at the next proxy recreation) are the
  maintainer's practical acceptance and are not part of this record.
