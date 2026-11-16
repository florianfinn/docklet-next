# M27 optional modules and proxy route management acceptance — 2026-11-16

The accepted package is main `d638121548551bba94d5c00634ebf3df86d17433`, the
[M27 closing PR #490](https://github.com/florianfinn/docklet-next/pull/490). Its
tree equals the feature head `21d66386`, which passed the final sandbox delta and
U-cell runs. The sandbox kit built the packages locally from the exact source; no
registry image was used.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment. No real proxy installation was written.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head.
- Codex workers built S1 to S5.
- The Claude main thread planned the milestone, wrote the design notes, made every
  correction after the reviews, and integrated the steps and acceptance fixes.
- Codex-built steps were reviewed by Claude; main-thread corrections were verified
  by Codex. The closing review was Codex; the Codex-written tests adopted near the
  end received a separate Claude review.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #472 | S1 module foundation: module setting, access-then-gate order, agent admission fence and reservations | R1 failed with one blocker (admission fence without a red proof); its correction was verified with S2. |
| #473 #474 | S2 proxy contract foundation: DTOs and codes, routes, agent contract 19, migration `005_proxy.sql`, journal and recovery table | R1 failed with four blockers (caller cancellation, identity wedge, startup settlement, intent fields). Verify rounds R2 to R5 failed with 3, 6, 4 and 1 blockers; R4 and R5 found proof gaps, not production defects, and decision AUTO-2 closed the guard-proof criterion. R6 passed with no findings. |
| #473 | S3 Traefik engine | R1 failed with three blockers (no-route response shape, log scope, hidden files). The first verify failed, the second passed. |
| #474 | S4 Caddy engine | R1 failed with three blockers (stale test, package-graph pin, Caddy no-route response). R2 passed with one non-blocker, fixed. A separate review of the main thread's integration corrections failed with three blockers (lost restore and eligibility diagnostics, a stale package-graph pin); its verify passed with no findings. |
| #471 | S5 production web | R1 failed with one blocker (rewritten backend URL). Two verify rounds failed on round-trip edge cases; decision AUTO-14 moved that class to the closing review. |
| #471 | Closing integration (#490) | The closing review failed with four blockers (round-trip edge values, transient read errors treated as classifications, a failed baseline probe accepted as routing evidence, guards without red proofs). Verify rounds 1 to 3 failed with 2, 1 and 2 blockers in the same classes; verify round 4 passed with no findings. The Claude review of the Codex-written tests failed with three blockers (conditional assertions); its verify passed with three non-blockers, tracked in #504. |

## Findings during acceptance

Five early smoke runs on intermediate heads found six product defects before the
candidate run, all fixed with regression tests:
- discovery split image references at a registry port;
- a read-only proxy-side mount made every source read-only;
- the Caddy admin readback used a Host header Caddy refuses;
- deleting the last Traefik router left an empty map Traefik rejects;
- the route dialog overflowed at 390 px;
- a Caddy rename that leaves the response unchanged ended unverified.

The candidate run and the deltas found:
- **F3, PX15.** On a Direct host, a transient Docker read during a rename was
  taken as a read-only source. Delta 1 returned such errors as errors.
- **F4.** After a proxy container restart, every new write ended
  `applied-unverified` because the registration kept the old start time. The
  agent inventory now reports the current start time and new operations use it.
- **PX15, delta 1.** Two of three name-only renames on D ended `exec-running`
  when the observation window cut the probe. Delta 2 kept the last reason.
- **F5, delta 1.** A recreated proxy instance could not be removed on its own
  because its settings needed a snapshot. Delta 2 renders settings without one.
- **MOD1.** Hub startup writes the default module row, so "never written" could
  not hold literally. The maintainer read it as never set by an operator
  (decision Q-MOD1-ROW (a) on #471).
- **PX16.** The pinned tools image cannot prove that a file has no extended
  attributes, ACLs or extra hard links. The maintainer made helper-only sources
  (named volumes, other allowed binds) read-only with reason
  `unsupported-metadata` (decision Q-HELPER-META (a) on #471). PX16 then checks
  read-only behaviour.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
with Traefik and Caddy fixture variants on D and E.
- Every write used a real UI gesture in Chromium. API reads, `docker cp` and
  probes served only as witnesses.
- D ran in English at 1440 px and E in German at 390 px.
- 42 U-cells covered the module rules and the proxy break map. A separate runner
  checked each against its test and an assertion-red mutation, and checked CI.

| Cell | Behaviour |
| --- | --- |
| MOD1 to MOD4, MOD6, MOD7 | Module off after upgrade; toggle with kept data and confirmed deletion; simple users and switched-off hosts get no module routes; switching off waits for unfinished operations; first-run setup step |
| PX1, PX2 | Discovery and inventory per source kind; shared-file classes, opaque and secret files listed by name only |
| PX3, PX4, PX19 | Traefik edit changes one span; own route file create and delete; shared service, priority and free-text rule |
| PX5, PX6, PX21 | Invalid sibling and external change refused with zero writes; an undefined middleware is restored automatically |
| PX7, PX13 | Routes survive Traefik and Caddy restarts |
| PX8, PX14, PX18, PX20, PX22 | Caddy create, edit and delete; real validation errors; `--resume` divergence; import-line insertion; admin-off restart path |
| PX9, PX11 | Missing import line and label routes are read-only with a link to the stack editor |
| PX12 | Proxy pages in German at 390 px and English at 1440 px, keyboard reachable |
| PX15, PX23 | Verification states: routing, marker load, configuration readback, unobservable |
| PX16 | Single-file mounts, system-path binds and helper-only sources read-only with reason |
| PX17 | `recovery-required` operation released through the UI |
| REG | Standard regression on H, D and E in English and German |

PX10 was dropped at kickoff and replaced by PX18 and PX22.

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `db7f510a` (candidate) | Failed, 26/29 | MOD1, PX15 (F3) and PX16 failed; F4 found |
| `7da74018` (delta 1) | Failed, 4/5 | F4, PX7, PX13 and REG passed; PX15 failed on two of three renames; F5 found |
| `7714d657` (delta 2) | **Accepted**, 3/3 | PX15, F5, PX12 |
| `e4ca973c` (delta 3) | **Accepted**, 11/11 | After the closing review fixes: PX3, PX4, PX8, PX15, PX17, PX18, PX19, PX22, F4, PX12, REG |
| `7c55a030` (delta 4) | **Accepted**, 6/6 | MOD1 and PX16 under the maintainer decisions; PX1, PX4, PX8, PX12 |
| `21d66386` (delta 5) | **Accepted**, 4/4 | PX1, PX4, PX8, PX16 after the last closing review fix |
| `d6381215` (exact main) | **Accepted**, 29/29 | Tree equals `21d66386`; deploy, REG on H, D and E in English and German, one Traefik create and delete on D, one Caddy create and delete on E; the log scan found no product failure, only advisory Edge request cancellations |

Unchanged passing cells carry their evidence forward, so the exact-main
denominator counts all 29 cells, not 29 newly executed ones. The U-cells passed
42/42 on `7714d657` and again on `21d66386`, where the runner reran 78
mutations, each assertion-red.

## Limits

- Helper-only sources stay read-only; writes there need a metadata check the
  tools image does not provide.
- Caddy snippet imports make an instance read-only (#499).
- Without `--configfile`, Traefik's static file is searched in `/etc/traefik`
  only (#500).
- A redirecting entry point can leave the Traefik marker `applied-unverified`
  (#501).
- Chromium only. The M26-to-M27 upgrade was exercised once, in the candidate
  run; the exact-main run did not repeat it.
- Certificate resolvers and the probe helper are M28.

## Open follow-ups

#498 (remaining proxy guard inventory), #499 (Caddy snippets), #500 (Traefik
finder locations), #501 (redirecting marker entry point), #502 (operation list
decoding), #503 (import hint link), #504 (proxy test witnesses).
