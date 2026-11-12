# M15 simple users acceptance — 2026-11-01

The accepted package is main `1f4578e11faebf6816f3e1cb2afdc96c56823eba`. It is
the [M15 closing PR #380](https://github.com/florianfinn/docklet-next/pull/380),
squash-merged as `d2603df1`, plus three acceptance fixes:
[#381](https://github.com/florianfinn/docklet-next/pull/381),
[#382](https://github.com/florianfinn/docklet-next/pull/382) and
[#384](https://github.com/florianfinn/docklet-next/pull/384). The tree of
`d2603df1` equals the merged feature head `7ce1ba4e`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:
- Codex GPT-6.1 Sol workers implemented the step commits, and Claude Opus
  reviewed them;
- the Claude main thread wrote the design notes and the fix rounds, and Codex Sol
  verified them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #326 | Design notes for users, resource rights and identity | R1 failed with 9 blockers; the notes were rewritten under the maintainer's guidance that users are a rare edge case. The delta check failed with 2 blockers, fixed before implementation. |
| #321, #322, #324, #325, #255, #268, #275, #276 | Backend: migration 015, rule table, access codes, user management, personal keys, proxy mapping, sign-out | R1 failed with 4 blockers. CI then found that a revoked terminal could still forward input. Verify rounds 1 and 2 failed with 1 and 2 blockers, mostly in stream revocation; round 3 passed. |
| #323, #324, #325 | Web: Settings › Users, user view, profile keys, sign-out | R1 passed with 7 nonblockers, fixed with red proofs |
| #326 | Closing integration (#380) | Opus part A passed with 3 nonblockers. Sol part B failed with 1 blocker: removing a host closed the streams of an administrator who kept assignment rows from before a promotion. After a table of every revocation producer, the fix passed part B round 2 and an Opus delta check. |

## Defects found before acceptance

An early smoke on the integrated backend and web found two copy defects in the
user run dialog and the profile, fixed in `e009279c`. It also found that the
Edge container log download returns 502. That predates M15, is not used by the
web, and is tracked in #379.

The candidate run on `e009279c` found that an account without assignments listed
the name, status and colour of every host. `7d9d97d9` returns no hosts to such an
account. Stack and administrator routes stayed closed to it.

The candidate reruns passed the upgrade and user view cells. Several cells
passed their functional assertions but failed on console entries from
intentional denials and app-cancelled reads. During the candidate reruns and
after the first acceptance run, the maintainer set the acceptance scope, matrix
v2.3:
- the live event stream may be checked on the sandbox's internal route,
  because the public sandbox proxy chain buffers streamed responses (X3);
- a closed list of expected console entries (C1–C6): a sign-in probe, statuses
  the cell asserts on purpose, ended streams, network changes during Docker
  network steps, a secondary test hub that is not paired with the Edge host,
  and reads the app cancels before their body is complete.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The sandbox had the
hub-local host, one Direct agent, one Edge agent, and a second hub in proxy mode
for the proxy mapping and sign-out. The run started with the upgrade from the
M14 package with data.

Package checks:

- migrations 0–15 applied on the hub and both agents;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract v11;
- the main push CI passed on `1f4578e1`.

The run applied the matrix on every applicable host type, through the
production UI with Playwright Chromium in German and English at 1440 and 390 px.
API calls supplemented the UI where the matrix names them, for foreign IDs,
administrator-only operations, keys and run access. It covered:

- user management: creation with a forced first password change, disable,
  enable, reset, delete, promotion and demotion, the last-administrator
  protection and the own-account locks;
- the user view with only assigned stacks, and 404 for foreign stacks and
  containers, including standalone containers;
- Restricted and Full work on assigned stacks: lifecycle, logs, shell, Compose
  and `.env` editing, redeploy, pull and image updates;
- revocation: removing an assignment, lowering the level and disabling a user
  close open log and shell streams, and the next request fails;
- personal API keys with expiry, one-time secret and revocation;
- run access limited to the user's own runs;
- sign-out in local, proxy and none mode, and the proxy mapping with
  `AUTH_PROXY_ADMIN_USERS` unset and set, including unknown and disabled
  identities;
- the folded follow-ups #255, #268, #275 and #276;
- administrator regressions against M14 on every host type.

## Result

Four runs on merged main were needed:

| Package | Result | Cause and fix |
| --- | --- | --- |
| `d2603df1` | Not accepted, 25 of 34 cells | The administrator regression cell on the hub-local host was incomplete in the harness. Eight cells failed only on aborted reads. Of 81 unmatched diagnostic entries, 61 concerned vulnerability reads; four failures had a recorded complete HTTP 200 response. #381 stops aborting completed reads. |
| `3ddb5a89` | Not accepted, 25 of 34 cells | The first fix also cancelled the completed body. #382 releases completed vulnerability reads and image pulls without cancelling them. A stale hub-local stack page after its own lifecycle command was measured to be identical on M14 and is tracked in #383. |
| `9e5bf789` | Not accepted, 26 of 34 cells | The evidence retained 78 diagnostic observations of HTTP 200 vulnerability responses with completed bodies reported as `net::ERR_ABORTED`; these are observations, not a count of unique requests. A reproduction outside the hub showed that Chromium can report a fully received `no-store` response read through a manual stream reader as cancelled. #384 reads these responses natively. |
| `1f4578e1` | **Accepted, 34 of 34 cells** | The eight blocking cells were rerun; the other 26 passed on earlier main packages and were reused, because each later delta changed only web data reads. |

On `1f4578e1` the browser made 312 vulnerability reads: 304 finished, none failed,
and 8 were still open when the browser context closed. No response had status
5xx. The event stream for administrators and users was checked on the internal
route (X3); no other exclusion was used.

## Limits

- Full access is not a security boundary: a Compose file can mount the Docker
  socket, and container names are user-chosen.
- A command accepted before a revocation completes; only streams are cut.
- Docklet cannot end the reverse proxy's own session.
- Two stacks with the same Compose project name on one host may list each
  other's containers; runtime actions on them are refused.
- When an agent hides a Docker read error behind an empty runtime view, a user
  sees "not found" instead of "unavailable".
- A vulnerability response without a declared length is read completely before
  the 32 MiB limit refuses it.
- The event stream was not verified through the public sandbox proxy chain.
- Chromium only, over HTTP/1.1 through the sandbox's proxy.

## Open follow-ups

#378, #379, #383.
