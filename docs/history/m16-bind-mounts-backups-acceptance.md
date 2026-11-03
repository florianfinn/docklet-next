# M16 bind mounts and backup completion acceptance — 2026-11-03

The accepted package is main `40187b73c1edc634d7933e69fee6fdfaa5af198b`. It is
the [M16 closing PR #393](https://github.com/florianfinn/docklet-next/pull/393),
squash-merged as `a37d77d0`, plus two acceptance fixes:
[#395](https://github.com/florianfinn/docklet-next/pull/395) and
[#397](https://github.com/florianfinn/docklet-next/pull/397). The tree of
`a37d77d0` equals the merged feature head `2e7b93e1`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:
- a Codex GPT-6.1 Sol worker implemented the backend step S1, and Claude Opus
  reviewed it;
- the Claude main thread wrote the design notes, the web steps and every fix
  round, and Codex Sol verified them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #327, #328, #287 | Design notes for bind path rules, single-file binds, size scans and backup completion | R1 failed with 6 blockers, fixed in notes v2. The single-file and size-scan notes failed twice (4 and 1 blockers): the tools image has no `cat`, `cp` or `du`. Fixed before implementation. |
| #327, #328, #287, #286, #288, #289 | Backend: path rules R1–R4, bind list and size scan, bind files, bind backups and restore, hub archive copies, registration revision, target delivery and draft test | R1 failed with 2 blockers: two write guards did not run through the production wiring. Verify round 1 failed with 6 blockers of one class, composition seams; a fake Docker daemon that runs the real helper commands closed it in round 2. |
| #328, #329, #289, #290, #291 | Web: bind entries and files, Move, overview figures and "Back up now", targets panel, delivery states | Six rounds. Rounds 1 to 5 failed with 8, 8, 2, 3 and 1 blockers: identity, stored state against live verdicts, demo state, path spellings and relative target paths. The demo state was rebuilt from a writer table, and round 6 passed. |
| #330 | Candidate fixes after the sandbox runs | Ten rounds. Two failure classes, late file view completions and the running operation chosen for the overview, each reached a third failing round and was rebuilt from a class table. Rounds 5, 6 and 10 passed. |
| #330 | Closing integration (#393) | Sol part B failed with 1 blocker: the late file view identity joined path fields with plain separators. Opus part A failed with 1 blocker: the bind self rule knew only Arcane labels, so the real hub's and agents' data binds were eligible. Both fixed; part B round 3 passed. |

## Defects found before acceptance

An early smoke on the integrated package executed the helper commands in the
pinned tools image and found seven defects. Workspace staging took the tools
image's root identity, so file writes to a volume used by a numeric user failed.
A scratch check used shell arithmetic that the pinned BusyBox lacks. Edge
refused backup IDs derived from bind keys. Four web defects followed. All were
fixed, and a guard now holds helper scripts to the pinned BusyBox.

The candidate run failed 22 of 46 applicable cells. The fixes added the Move
gesture, kept upload restore owners, added per-policy manual hub archives,
translated dialog labels, stopped startup pruning from removing internal settings
rows, and answered requests cancelled by hub maintenance with the maintenance
problem. Five reruns on candidate packages then found a writer-recovery retry
that closed every volume and an overview that reported a restore's safety
capture as a backup without an activity. The fourth and fifth reruns passed 46
of 46, the fifth on the PR head.

During these runs the maintainer set the remaining acceptance scope:
- C8, the expected request failures while a host is deliberately stopped, during
  hub maintenance, an Edge reconnect or a short hub archive pulse (Q-A2, Q-A5,
  Q-A6);
- X4: a Direct download of 3 GiB or more through the public sandbox proxy,
  which ends it after 90 s without a first byte (Q-A4);
- X5: one Edge read failed by the tunnel's full response channel, which predates
  M16 (Q-A7);
- Q-A8: a request failure during a deliberately stopped agent that shows no
  error to the user while every functional assertion passes is recorded as a
  budget deviation with a follow-up issue instead of failing the cell.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The containers were
recreated, so they carry the `io.docklet.system` image label. The sandbox had
the hub-local host, one Direct agent and one Edge agent, with SMB and NFS servers.

Package checks on each run:

- migration ledger 000–015 and SQLite quick checks on the hub and both agents;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract v12;
- the real data binds and image labels of the hub and agents;
- the tools image unchanged at its pinned digest.

The run applied 20 matrix rows on every applicable host type, through the
production UI with Playwright Chromium in German and English at 1440 and 390 px.
API and Docker calls prepared fixtures, injected deliberate faults and verified
the results. It covered:

- path rules: system, Docker and Docklet paths refused with typed reasons,
  including the real hub's and agents' data binds, their parents and children;
- bind files: browse, edit, upload, Move, and in-place edits of single-file
  binds that keep inode, owner and mode; on-demand size scans;
- bind backups: plans, schedules, retention, replace, selected-file and upload
  restore, with consumers stopped and restarted;
- hub archive copies to SMB and NFS targets, their retention and the manual
  delete;
- target delivery states, draft tests, registration revision changes and a
  target edit while the agent is stopped;
- the overview figures, unprotected markers and "Back up now", also with a host
  offline;
- Direct and hub-local downloads through the browser download manager, and
  Edge's 512 MiB refusal;
- the upgrade from the M15 package with data, and named-volume regressions.

## Result

The first run on merged main was a fresh full run. Later runs repeated the
failed cells and the cells the fix could affect:

| Package | Result | Cause and fix |
| --- | --- | --- |
| `a37d77d0` | Not accepted, 44 of 46 cells | Both failures were in the request budget; the gestures completed. TR1 H: the web cancelled the bodies of successful settings writes. DV1 D: while a Direct agent was deliberately stopped, its image list read answered 503 instead of reporting the host as unavailable. #395 reads successful status-only bodies to the end. An agent that has begun to stop withholds an uncommitted server error and ends the request like a lost connection, so the hub reports the host as unavailable; Edge sends nothing for it. |
| `b17a7a70` | Not accepted, 45 of 46 cells | TR1 H, DV1 D, and the affected BP1 H and R1 D slices passed. DV1 E: a target edit saved with HTTP 200 while the Edge agent was stopped, but the dialog showed an error. The agent going offline invalidated every host's reads, which also discarded the answered write. #397 discards an answered write only when the identity changes. |
| `40187b73` | **Accepted, 46 of 46 cells** | DV1 D and E were rerun in full. The other 44 cells passed on the earlier main packages and were reused, because the later deltas changed only the agent stop path and web data handling. |

On `40187b73` both DV1 cells saved the target while the agent was stopped, closed
the dialog without an error, showed pending, and showed current after restart.
One Edge repository read answered 502 inside the stop window, before the target
card showed pending. It is the one Q-A8 deviation and is tracked in #398. The
other request failures of the run fall under C8. X4 and X5 carry over from the
first run, each covering one action; X1 to X3 were not used, and HB2 ran against
a real NFS server.

The fix #395 took three review rounds. The first fix rewrote the error into an
unavailable response, which a later layer turned back into 503, and Edge
bypassed it. The second set the stop signal too late when the
parent context or a fatal error ended the agent. After a table of every agent
stop path, round 3 passed. #397 passed its single review round.

## Limits

- Symlink aliases and different mount views of the same data are separate
  binds.
- Operations on overlapping bind paths are not locked against each other.
- A hub archive copy proves size, not content, and goes only to targets the
  hub's own host can mount.
- In-place writes and restores of a single file are not atomic.
- Size scans stop after 120 s, and the snapshot size remains.
- The host path can change between the source witness and the engine create.
- The large Direct download was verified inside the sandbox network, not
  through the public sandbox proxy (X4).
- Chromium only, over HTTP/1.1 through the sandbox's proxy.

## Open follow-ups

#386, #387, #388, #389, #390, #391, #392, #394, #396, #398.
