# M28 proxy certificate resolvers, probe helper and setup clarity acceptance — 2026-11-20

The accepted package is main `3c00052a074f4733c24e00ce9a564a34189a75af`, the
[M28 closing PR #535](https://github.com/florianfinn/docklet-next/pull/535). Its
tree equals the feature head `bcf30bac`, whose last commit fixed two closing-review
findings in resolver admission. The sandbox kit built every package locally from
the exact source.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment. Certificates were issued only by a sandbox test
CA (Pebble).

## Integrated steps

The tools foundation landed separately on main through #532 and #534. The
remaining steps were pushed to the feature branch as commits, without step PRs,
and CI ran on every pushed head.
- Codex workers built S0, S1, S4 and S5.
- The Claude main thread planned the milestone, built S3 and made every
  correction after reviews and acceptance runs.
- Claude reviewed the Codex-built steps. Codex reviewed the main-thread
  corrections, the tools default pin and the closing integration. S0 also had
  a Claude recheck before its independent Codex delta review.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #493 | S0 `docklet-probe` in the tools image (#532), then the published tools image 0.2.0 and its default pin (#534) | The first review failed with two blockers, which were fixed; the delta review passed. |
| #491 | S1 contract foundation: resolver, certificate and eligibility vocabulary, OpenAPI, agent contract 20 | The first review failed on an error response echoing submitted input. Verify 1 failed on a second echo path; verify 2 passed. |
| #528 | S3 setup clarity: modules navigation group, instance select, per-source eligibility with server revalidation | First reviewed in the closing review; a nonblocking test-synchronisation finding was deferred to #539. |
| #493 | S4 probe client: helper probes in the target network namespace, certificate observation, manual check, wget fallback | The first review failed on tools image pulls that ignored stored registry credentials. Delta 1 found credentials possibly reflected in pull error text, delta 2 a duplicated pull audit event. Delta 3 passed. |
| #492 | S5 resolver operation: journaled backup, write, Traefik recreate, observation, rollback and recovery | The first review failed with three blockers: a reservation stuck after a failed adoption, a refused default-mode `.env`, recovery wedged by changed file metadata. Deltas 1 to 4 failed with 3, 2, 1 and 1 blockers, all in the confirmation and error-classification class. Delta 4 swept every error producer; delta 5 passed. |
| #491 | Closing integration (#535) | The closing review failed with one blocker: a Traefik service without a compose `command:` could not get a resolver. A selected related non-blocker, admission of a service borrowing another service's namespaces, was also fixed before merge. The verification passed. |

CI caught one more defect: hub and agent failed to start without a reachable
Docker daemon, because the startup sweep of probe containers returned its error.
The sweep now only logs a warning.

## Findings during acceptance

- **Candidate, A-R3.** Settings showed no resolver details, because the Traefik
  inventory never filled the resolver summaries. It now does, with variable names
  and whether each is set, never values or email.
- **Candidate, A-U3.** A candidate without any mounted source showed an empty
  source selector instead of the fix explanation.
- **Q-A1.** On a `.env` created by the stack editor (mode 0644), a DNS-01 resolver
  with new secret values was refused. Maintainer decision D-A1: the resolver now
  writes a changed `.env` with mode 0600, says so in the dialog, and a rollback
  restores the previous bytes and mode.
- **Q-A2, A-R4.** The approved fault (stopping the replacement Traefik right after
  its first start) is caught while compose starts it, before Docklet observes it.
  Maintainer decision D-A2: this method suffices, and A-R4 is judged by its Pass
  column.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E.
Fixtures included Traefik stacks with a static file or command flags and a Pebble
test CA.
- Every write used a real UI gesture in Chromium; API reads and container
  inspection served only as witnesses.
- D ran in English at 1440 px and E in German at 390 px.
- Sentinel credentials were scanned in memory across proxy responses,
  activities, events, hub and agent databases and Docklet logs.

| Cell | Behaviour |
| --- | --- |
| A-R1, A-R2 | HTTP-01 resolver on the file layout (D) and the flags layout over Edge (E); instance adopted, route verified, certificate observed |
| A-R3 | DNS-01 with sentinel variables; effective environment witness; zero sentinel hits; Settings shows names and configured state only |
| A-R4 | Replacement stopped after its first start: rolled back, files byte- and mode-identical, routes answer, instance usable |
| A-R5 | `.env` edited during observation, then a fault: `recovery-required` with `third-version`, edit kept, confirmed Release |
| A-R6 | Environment-source and unmanaged Traefik refused with a typed reason, nothing changed |
| A-R7 | Agent restart during observation: rolled back and adopted, next operation works |
| A-P1, A-P2 | Helper probe on D and E (path-only route, redirect not followed, SNI routing, certificate observation); wget fallback with tools 0.1.0 |
| A-U1 to A-U4 | Module navigation, instance select, setup eligibility on D and E, fresh Traefik and Caddy registration |
| REG | Standard regression on D and E |

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `82a9cd63` (candidate) | Failed, 11/14 | A-R3, A-U3 and A-R4 failed; Q-A1 observed |
| `64571e14` (delta 1) | 13/14 | A-R3 (also on a 0644 `.env` with rollback), A-U3 and REG on H, D and E passed; A-R4 pending Q-A2; complete log scan |
| `3c00052a` (exact main) | **Accepted**, 14/14 | Tree equals `bcf30bac`; A-R1 and A-R6 rerun for the admission fixes, plus an admitted Traefik project without `command:`; A-R4 rerun under D-A2; one helper probe on E, A-U1, REG, log scan |

Unchanged passing cells carry their evidence forward, so the exact-main
denominator counts all 14 cells, not 14 newly executed ones.

## Limits

- Only the Pebble test CA issued certificates. Real CA and DNS issuance were
  excluded (no public DNS or CA in the sandbox); A-R3 checks configuration,
  environment, adoption and secret scans instead.
- TLS-ALPN challenges and resolvers on Caddy are out of scope.
- The upgrade from release 1.1.1 was not exercised. The sandbox database already
  had a newer schema from the preceding milestone, which 1.1.1 refuses to start
  on.
- Chromium only.
- At 1440 px, expanded certificate details squeeze the route row (#541).

## Open follow-ups

#537 (certificate check diagnostics and entrypoint), #538 (resolver dialog
provider catalogue and `ProjectPath`), #539 (setup test synchronisation), #540
(source overlap test and docs), #541 (certificate details layout).
