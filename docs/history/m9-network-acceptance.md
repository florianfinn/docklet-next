# M9 network management acceptance — 2026-10-24

The accepted package is main `ec84ed6e6b9b7187e746456535c0cbf186c205f6`.
[M9 closing PR #215](https://github.com/florianfinn/docklet-next/pull/215) merged
at `be813fd8a9f61eaf99161d4141e5c324b5c34094` after one independent closing
review round (pass). Practical acceptance of that first main package failed on
hidden dialog rejections; acceptance R2 passed after the fix in #216. This
records sandbox acceptance, not a product release or production deployment.

The integrated steps were #206 (#203 network contract; risk review R1 failed
with one blocker, R2 passed), #208 (#204 web; R1 passed, selected corrections
were made, R2 passed) and #212 (F1 partial topology reads; review passed).
Sandbox-found standalone fixes merged to main in #213 at `54f5825` (event list
rate limit; review passed) and #216 at `ec84ed6` (rejections inside open dialogs;
R1 failed with two blockers, R2 passed).

Sandbox runs found three product defects:

| Issue | PR | Defect found in the sandbox |
| --- | --- | --- |
| #204 | #212 | A host vanished from the Networks page when network inspect reads failed (F1) |
| — | #213 | Event list GETs shared the agent ingestion rate limit and returned 429 after two tabs and a few reloads |
| #205 | #216 | Server rejections in open dialogs appeared only as toasts that the modal covered fully or partly in three of four combinations (D1) |

## Accepted scope and method

Hub and agent images were built from `deploy/hub/Dockerfile` at the exact main
commit and deployed to the disposable sandbox hub, one Direct agent and one Edge
agent, after data backups. Hub and agents used contract v6. The served bundle
hash matched the local build. Playwright Chromium drove the production UI in
four combinations: Direct German at 1440 px, Direct English at 390 px, Edge
English at 1440 px and Edge German at 390 px. API and Docker reads verified the
real producer state; setup and cleanup did not replace the UI gestures.

1. Load Networks and inspect built-in usage details by hover and keyboard focus.
2. Create an IPv4 bridge with gateway, IP range, label and auxiliary address;
   verify the table, topology and API inspect. Optional driver options were skipped.
3. Create dual-stack and IPv6-only bridges; verify the family flags.
4. Reject an out-of-subnet gateway without a request and show a translated
   overlap validation error.
5. Connect running and stopped containers with fixed IPv4 addresses and an
   alias; verify usage details, state, IPAM configuration and alias.
6. Reject reuse of the stopped container's reserved address.
7. Block removal with a stopped attachment, protect built-ins, and recognise
   and protect a Compose-labelled network unknown to the hub.
8. Reload and switch hosts and back without stale rows; disconnect both
   containers, delete the unused network, and verify its absence after reload.
9. Check translated error rendering without literal "error" or untranslated
   English in German.
10. At 390 px, check create dialogs with Advanced open and connect dialogs with
    aliases for usability and horizontal overflow.
11. Check production requests and console output, including absence of
    demo/design-lab requests, with a strict gate of zero 429 and zero `ERR_FAILED`.

All 42 applicable matrix cells passed on their initial R2 runs: 11 steps in
four combinations, with step 10 not applicable at the two desktop widths. No
acceptance cell was retried. The inherited corrected pointer harness was used
from the start. No unexpected console errors or demo/design-lab requests were
observed. Expected pre-login 401 responses and navigation/identity
`ERR_ABORTED` entries were recorded separately; intentional 400/409 rejections
were checked in separate sessions. The matrix observed no event list GETs, so
its zero-429 result does not independently prove event-list coverage; the
standalone #213 review exercised that route.

Additional real server rejections were forced after valid drafts were prepared,
without response interception or client bypass. Creating an overlapping network
between draft and submit produced 409 `network.overlap`; occupying a selected
fixed address with another container produced 400 `network.invalid-ip`. Both
errors were translated, visible and uncovered inside the still-open dialog in
all four combinations. The drafts were retained; correcting them succeeded and
closed the dialogs, with producer state verified. One volume rename conflict,
checked only in Direct German at 1440 px, also appeared inline and retained its
draft, using the translated generic message rather than a specific conflict
reason (#217).

## Limits and follow-ups

- The gating browser ran with HTTP/2 disabled. A native Fetch client reproduced
  Chromium surfacing a request-creation race as generic `ERR_FAILED` when the
  shared front proxy retired an HTTP/2 connection after about 1,000 requests.
  The failure was intermittent: one of three focused HTTP/2 repetitions failed.
  The corresponding HTTP/1.1 probe passed. The original matrix net-logs were not
  captured, so the diagnosis does not prove the cause of every original failure.
  R2 did not repeat the non-gating HTTP/2 observation; safe-read recovery and
  excessive read fan-out remain in #211.
- Stack files referencing a network as `external` are not detected. Docklet does
  not check whether a macvlan/ipvlan parent interface exists; the daemon checks
  that. Topology shows the first pool per family.
- The volume rename conflict has a generic translated message (#217).
- Open follow-ups: #207 (contract review findings), #209 (write outcome on proxy
  timeouts), #210 (network UI states and review findings), #211 (transport and
  read fan-out), #214 (event limiter tests and audit), #217 (volume conflict message).
- This run does not establish other-browser coverage, older-agent compatibility
  or release acceptance. It did not exercise macvlan/ipvlan creation, fixed IPv6
  membership or every dialog covered by #216.

## Coordination lessons

The early API-only smoke established no product defect; the UI candidate and
practical acceptance runs found three. Repeated transport and tooltip failures
were investigated with root-cause plans and measurements before further fixes
or acceptance attempts. Minimal reproductions separated the shared front-proxy
failure and a harness pointer-trajectory defect from product defects. R2 used
the corrected harness without retries; earlier failed runs remain part of the
evidence rather than being counted as successful acceptance.
