# M36 feedback fixes and private Tailscale route acceptance — 2026-11-23

The accepted package is main `4907bc9cd13b1e00dc702b1a48691c63198df46c`. It is
the squash of the [M36 closing PR #575](https://github.com/florianfinn/docklet-next/pull/575).
Its tree is identical to the accepted feature head `aeb77902`. The sandbox kit
built every package locally from the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M36 covers the feedback round of 2026-11-22 (#567–#573) and the CI runner fix
#536. All steps were pushed to one feature branch as commits, without step PRs.
CI ran on every pushed head.
- The Claude main thread planned the milestone, wrote the design notes, built
  S0, S2a and S3, integrated every step, and made every correction after reviews
  and acceptance runs.
- A separate Claude worker built S4a (Tailscale backend).
- Codex workers built S1 (#567), S2b (#568, #570, #572) and S4b (Tailscale UI).
- Every review came from the other provider than the implementation.

| Issue | Step | Review outcome |
| --- | --- | --- |
| all | Design notes with break maps for the risky steps | The bundled design review failed with 12 blockers. The verify left 4; the kickoff decisions and revision 3 of the notes settled them. |
| #568, #571 | No-baseline reason, repository check steps and counts (S2a, S3) | Round 1 failed with 4 blockers. Round 2 left one blocker, the binding of check results to their run, which received a root-cause table. Round 3 passed. |
| #567, #568, #570, #572 | Deploy phases and pull detail (S1); record baseline, backup volume name, hub address help (S2b) | Failed with 1 blocker: a service pull policy overrode the record action's no-pull request. Fixed; the verify passed. |
| #573 | Tailscale backend: tailnet module, credentials, key minting, ingress, agent route, migration (S4a) | Round 1 failed with 7 blockers, among them secret redaction, retained node identity and provisioning order. Round 2 found 2 new blockers in route activation, which received a root-cause table. Round 3 passed. |
| #573 | Tailscale UI: setup step, Connections, onboarding route, hosts chip and offline chain (S4b) | The review failed with 2 blockers (a credential save froze the status; the offline chain disagreed with the hub). The control URL hand-off failed with 1 blocker (a URL with user info reached the browser). The verify left 2 residual cases; their fix was checked against root-cause tables in the closing review. |
| all | Closing integration (#575) | The closing review failed with one blocker: a control URL with a port but no host passed the hub check. Fixed in `aeb77902`; the verify passed. |

Every blocker fix and break-map row has a test that fails on an assertion when
its guard is reverted.

Main-thread decisions AUTO-1 to AUTO-11 were posted on #574. The maintainer
decided the acceptance method of one cell during the run (see Findings).

## Findings during acceptance

- **Candidate, T1.** The response of a newly created host dropped the one-use
  tailnet key and hub URL in its custom JSON encoder, so no installer could be
  generated. T3 and the chain part of T6 could not run. Fixed in `7366e12a`;
  delta 1 passed.
- **Candidate, R1.** On Direct and Edge hosts, a cached image layer hid the byte
  detail of the whole pull. Fixed in `59814279`; delta 1 passed on H, D and E.
- **Candidate, A1.** With a portless saved address and no reachable published
  candidate, Edge onboarding defaulted to the public origin instead of the saved
  address with the published port. Fixed in `96cc1025`, with bare IPv6 in
  `d1624df6`; delta 1 passed.
- **Delta 1, T3 link 2.** The mounted Hosts page kept the tailnet peer online
  for up to 44 s after the hub had reported it offline. Two causes: the sandbox's
  shared public portal delivered each stream frame only with the next one, and
  the hosts list let that older frame win over a newer catalogue read. The list
  was fixed in `d95c5455`. Latency is measured at the hub origin under the
  standing acceptance rule; delta 2 passed.
- **Delta 1, T3 link 3.** A control-server policy that denied the agent port did
  not cut an established session. The maintainer chose a new method: deny the
  port, then restart only the agent, so the peer stays online and the fresh
  tunnel is refused. A relay cut that interrupts an established session passed as
  extra evidence.
- **T5 run 1.** The real-account run stopped before enrolment: the provided
  OAuth client lacked auth-key write permission and tags. After the client and
  the tailnet policy were corrected, T5 passed. The setup hint now says to save
  the policy before creating the client (`fe3e2c3f`).

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D, one Edge agent E, and
for the tailnet cells one Edge agent T on an isolated network that reached the
hub only through a self-hosted Headscale tailnet with an embedded relay. T5 used
a real Tailscale account; the run removed every device it created.
- Application actions used real UI gestures in Chromium.
- API reads, Docker inspection, recorded stream frames and fixed-viewport
  measurements witnessed their effects.
- No cell was excluded.

| Cell | Behaviour |
| --- | --- |
| R1 | Redeploy that pulls a changed image shows layer and byte detail, then create and start; the trace keeps the reached phases (H, D, E) |
| R2 | Deploy with a cached image shows no pull phase |
| P1 | First Docklet redeploy explains the missing baseline; the next update creates a restore point (H, D, E) |
| P2 | Record baseline pulls nothing and keeps the container IDs |
| N1 | Fresh hub and agent use `docklet-backups`; upgrades keep `arcane-backups` and its snapshots; no Arcane names in Appearance |
| A1 | Address candidates, missing-port warning, Edge default with the published port, not-reported hint after 90 s |
| S69 | Settings picker at 390 px opens the full list below the trigger |
| R3 | Repository check title, steps and snapshot count on card, focus and stored target (H, D) |
| T1 | Tailnet route in manual-key mode: plain TCP to the hub fails; pairing, WebSocket, poll and gRPC; stack deploy; badge and chip |
| T2 | The tailnet ingress refuses UI and non-agent API paths from a tailnet node |
| T3 | Offline chain names the first failing link: hub disabled, agent stopped (within 30 s), agent port denied |
| T4 | Connections in manual mode: wrong key rejected, connected state, disable and enable keep the identity, reset needs confirmation |
| T5 | Real Tailscale account: OAuth states in setup and Connections at desktop and phone width, tagged hub enrolment, per-host key, pairing and deploy, provisioning rejection keeps the connection truthful, cleanup |
| T6 | Setup step, Connections, onboarding route and host chain at 390 px without horizontal scroll |
| REG | Standard regression on H, D and E |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `31f7f722` (early smoke) | Done | Build, regression and S69 |
| `ac5ba12b` (candidate) | Failed, 9/14 | T1, R1 and A1 product defects; T3 and T6 blocked by T1; T5 separate |
| `96cc1025` (delta 1) | Failed, 5/6 | R1, A1, T1, T6 and REG passed; T3 failed |
| `fe3e2c3f` (delta 2) | **Accepted 2/2** | T3 and REG |
| `fe3e2c3f` (T5, run 2) | **Passed** | Run 1 on `d95c5455` stopped at the provider permission prerequisite |
| `4907bc9c` (exact main) | **Accepted 2/2** | Smoke, regression and T1 rerun; the other cells carry their passing evidence |

## Limits

- No cell was excluded.
- Chromium only.
- Behind a buffering reverse proxy, live stream frames can arrive one frame
  late. The T3 latency was measured at the hub origin.
- The T3 link-3 method refuses a fresh tunnel. The interruption of an
  established session is backed only by the extra relay-cut evidence.
- Agents older than contract 21 report a missing baseline with the generic hint.
- Six test-stability follow-up items in #576 record tests that failed under
  load and passed on isolated reruns.
