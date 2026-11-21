# M34 simpler stack actions, recorded env files and proxy UI acceptance — 2026-11-21

The accepted package is main `8f4cc2f73fa1d0f9ec2925976f41340621855400`, the
squash of the [M34 closing PR #551](https://github.com/florianfinn/docklet-next/pull/551).
The sandbox kit built every package locally from the exact source.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment.

## Integrated steps

All steps were pushed to one feature branch as commits, without step PRs, and CI
ran on every pushed head.
- The Claude main thread planned the milestone and wrote the design notes.
- It also built #545, #546 and #547, and made every correction after reviews and
  acceptance runs.
- Codex workers built the #548 step (recorded env files) and the #549 step
  (proxy UI adoption).
- Claude reviewed the Codex-built steps. Codex reviewed the main-thread work and
  the closing integration.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #547, #548, #549 | Design notes `stack-env-and-peers.md` and `proxy-ui-adoption.md` with break maps | The bundled design review failed with 8 blockers. Its verify still had 5 not fully resolved; the main thread closed them in the notes (`1d8ef389`) before implementation. |
| #545, #546 | Settings section picker; update dialog reason once with **Deploy now** | Self-checks and CI; covered by the closing review. |
| #547 | Unreadable stacks never block other stacks; per-stack load problems; duplicate name warning | Covered by the closing review (below). |
| #549, #541, #503 | Proxy UI adoption from the design lab | The first review failed with 2 blockers: entry points were locked under "Keep current", and the instance bar overlapped at 390 px. Both were fixed, and the second round found one more blocker (step rail button names), which was then fixed. |
| #548 | Recorded env files | The first review failed with 2 blockers. The Traefik certificate resolver bypassed the recorded inputs, and a typed `${VAR}` field made the service projection fail. Both were fixed with five related corrections; the Codex verify passed. |
| all | Closing integration (#551) | The closing review failed with one blocker: Compose name peer warnings exposed other stacks' names to users restricted to assigned stacks. The fix's first verify found the same class in the hub's offline snapshot fallback. The second verify passed. |

Behaviour guards carry red proofs, run with the guard reverted:
- 7 for #547;
- 22 for the #548 step and 8 for its review fixes;
- 5 for the proxy fixes;
- 9 for the closing review fixes.

The agent contract 20 fingerprint is unchanged.

## Findings during acceptance

- **Candidate, X2.** A route created in Easy mode in the source root was sent as
  `./name.yml`, and the operation API refused it as a non-canonical path. The
  defect already existed in 1.2.0. It was fixed in `6fb4ca0e`, and delta 1 passed.
- **Candidate, X6.** The matrix fixture, a single-file Caddy source, can never be
  registered (`source-ineligible`), so the manual-import hint could not be
  reached. Main-thread decision AUTO-22 (posted on #550) corrected only the
  fixture: X6 now uses a Caddy instance that the product really registers, with
  the import missing. The cell's behaviour was unchanged, and no database rows
  were seeded. Delta 1 passed.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
- Real proxy fixtures ran on D and E: Traefik and Caddy, in containers on the
  sandbox daemons.
- Application actions used real UI gestures in Chromium.
- API reads, container label inspection, file hashes and HTTP probes through the
  proxies witnessed their effects.
- Stacks with an external env file were started with the real Compose CLI
  (`--env-file`).

| Cell | Behaviour |
| --- | --- |
| A1 | Settings at 390 px: one grouped section picker; desktop sidebar unchanged |
| A2 | Never-deployed stack: reason shown once, no update controls, **Deploy now** deploys, then the update is offered |
| P1 | With an unreadable stack present, actions on another stack run without a dialog (H, D, E) |
| P2 | Two stacks with the same literal name both show a warning; redeploy runs |
| P3 | The unreadable stack names its missing variable; the `${VAR:?message}` text reaches no API body or hub log |
| P4 | A stack started with `--env-file` (file inside the projects directory, outside the stack) is readable. Its update baseline is captured after one deploy. A forced redeploy keeps the `environment_file` label, and the env file bytes are unchanged (H, D) |
| P5 | A label pointing outside the projects directory gives a named problem; an inotify witness shows zero reads |
| X1–X5, X7 | Proxy list, editor, suggestions, tooltips, step editor, route creation and minimal edit, certificate details width, Caddy save with `caddy validate` (D) |
| X6 | Manual-import hint links to the proxy container's stack editor (D) |
| X8 | Edge destination suggestions list exactly that host's containers (E) |
| REG | Standard regression on H |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `0c0cb0d3` (baseline, main before M34) | Witnesses reproduced | P1 dialog, P3 log leak, P4 unreadable, X5 squeeze; no environment limit established |
| `0123fac3` (candidate) | Failed, 17/19 | X2 product defect; X6 fixture prerequisite |
| `6fb4ca0e` (delta 1) | **Accepted 19/19** | X2 and X6 rerun |
| `8f4cc2f7` (exact main) | **Accepted 19/19** | P1 on H, D and E, P2 and REG rerun after the closing fixes |

Delta 1 reran two cells and carried 17; exact main reran five cell rows and
carried 14. Unchanged passing cells carry their evidence forward.

Exact main also recorded one observation outside the matrix. On H and D, a
simple user assigned to one of two same-named stacks received no unassigned
stack row and no peer-name fields in list or detail responses. The assigned
stack's own name remained present. Administrators kept the warning.

## Limits

- No cell was excluded.
- Chromium only.
- A stack forgets its recorded env file after `docker compose down`.
- A deploy while the container listing fails runs without the recorded env file
  (#552).
- Remote agents on 1.2.0 keep the old peer confirmation until they are updated.

## Open follow-ups

#552 (deploy while the container listing fails).
