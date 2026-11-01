# M14 container and host operations acceptance — 2026-11-01

The accepted package is main `09fc9b8729cb983ac65d55db8b78cb1212cbd7e7`. It is
the
[M14 closing PR #374](https://github.com/florianfinn/docklet-next/pull/374),
squash-merged as `561c2285`, plus the acceptance fix
[#376](https://github.com/florianfinn/docklet-next/pull/376). The tree of
`561c2285` equals the merged feature head `486318b5`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:
- Codex GPT-6.1 Sol workers implemented the step commits, and Claude Opus
  reviewed them;
- the Claude main thread wrote the fix rounds, and Codex Sol verified them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #311, #222 | Roadmap decisions in the design docs; partial-host picker test and a smaller handler size ceiling | Self-check and CI |
| #313–#317, #253 | Foundation: migration 014, agent contract v11, origins, recreate journal, stop services | R1 failed with 3 blockers; four verification rounds failed with 1, 2 and 1 blockers, and the fifth passed. The recreate-recovery class was rebuilt from a producer and consumer table. |
| #318, #302, #307, #309 | Host edits, agent restart receiver and durable one-shot restart, connection checks, heartbeat availability, typed refusals | Passed with 6 nonblockers, fixed with red proofs |
| #312 | Container shell | Passed with 5 nonblockers, fixed with red proofs |
| #313–#317 | Web origins, recreate and edit, service restart, stop mode | R1 failed with 1 blocker; the main-thread round failed with 1 blocker and R2 with 1 blocker; R3 passed |
| #318, #100 | Web host maintenance and verdict retention | R1 passed with 7 nonblockers. The verification chain of the verdict class failed nine times, mostly on host status writers; R10 passed under a bounded criterion for the structural writer guard |
| #320 | Closing integration (#374) | Opus part A failed on PR text only (provider attribution and missing interim rounds); after the text was corrected, a re-check passed. Sol part B passed. |

## Defects found before acceptance

An early smoke on the integrated feature head found three product defects; `a3d05d7` fixed
them. The first candidate run on `a3d05d7` passed 49 of 63 applicable cells and
found these defects, which `356c62d`, `583acb6`, `82d9bd1`, `1159592`, `c33b39b` and `486318b` fixed:

- a resize on an ended shell, or the end of a shell stream, hid the typed
  missing-shell reason;
- a frozen Direct agent held every browser connection with an unbounded
  inventory read, so hub-local reads timed out. Ordinary inventory reads now end
  after 20 s and answer from their snapshot or with `agent.unavailable`, also
  when a body transfer fails after its headers;
- the Edge connect write replaced a stored update-required verdict with pending.

Three focused reruns passed the remaining cells. During them the maintainer
clarified the acceptance scope, matrix v6:
- a disconnected update-required host shows offline;
- the connect-time compatibility check is expected;
- the UI card may show its previous state for up to about 10 s after an Edge
  reconnect.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The sandbox had the
hub-local host, one Direct agent and one Edge agent. The run started with the
upgrade from the M13 package with data, including v10 agents.

Package checks:

- migrations 0–14 applied on the hub and both agents;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract v11;
- the main push CI passed.

The run applied the matrix through the production UI with Playwright Chromium,
in German and English at 1440 and 390 px, on every applicable host type:

- the container shell, including a container without `/bin/sh`, shell activity
  without content, and zero orphaned execs;
- origin detection and manual assignment, origin locks, and the hidden agent
  container with 409 refusals on Direct and Edge;
- recreate and edit with recovery, and single-service restart and stop mode for
  stacks;
- host maintenance: edits, Check with typed results, the one-shot agent restart,
  the components panel, installation files and removal;
- Edge heartbeat liveness on the WebSocket and gRPC listeners, standby in poll
  mode, and typed refusals for disabled UI configuration and bad credentials;
- the update-required verdict across disconnect, hub restart and reconnect;
- the five-minute availability notification, layout and translations.

## Result

The first run on `561c2285` concluded **not accepted**: 62 cells passed, 1
failed, none excluded and 12 do not apply. In cell OR1-H, two concurrent
hub-local container list reads crashed the hub with a concurrent map write. Docker
list reads are coalesced, so concurrent requests shared label maps, and the
origin overlay decoded its result back into them. #376 decodes into a fresh
value, with tests that fail on the old code.

A focused rerun on `09fc9b8` concluded **accepted**:
- OR1-H passed, and so did OR2-H, OR3-H, RC1-H and a container list and detail
  smoke;
- 400 list and 100 detail reads at concurrency 16 against each host type
  returned 200 with origin fields, with no hub restart and no fatal log line.

The composed matrix has 63 passed cells and none failed or excluded. Failed
attempts in both runs were harness issues and are retained in the private
evidence.

Measured in the sandbox:
- a frozen Edge agent went offline after 88 s on both listeners and returned
  online about 2.3 s after it resumed;
- a poll-mode Edge host reached standby after 303 s of idle;
- after a v10 Edge reconnect the card showed offline for 1.4 s and never showed
  pending or online;
- the offline notice for a frozen Direct agent arrived after 360 s.

## Limits

- An idle poll-mode Edge host never reaches sustained standby with default
  settings, because the health job is standing tunnel demand. The cell used the
  verified recipe with deferred jobs and no open browser (#373).
- Standalone webhook redeploys use the legacy, unjournaled path outside runtime
  admission.
- The host status writer guard is syntactic (#372).
- Inventory reads are not cancelled on the web side, and a host whose read takes
  longer than 20 s answers from its snapshot.
- Chromium only, over HTTP/1.1 through the sandbox's proxy.

## Open follow-ups

#366, #367, #368, #369, #370, #372, #373, #375.
