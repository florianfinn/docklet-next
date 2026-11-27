# M46 update everything acceptance — 2026-11-27

The accepted package is main `6a1046ef772f24713bdf8c6ede732ebd895efb6d`: the
[M46 closing PR #686](https://github.com/florianfinn/docklet-next/pull/686),
squashed from head `67d5ecc3`. The sandbox kit built every package locally from
the exact source.

This file records sandbox acceptance only. It is not a product release, a
published image or a production deployment.

## Integrated steps

M46 adds one confirmed administrator action that updates the hub first and,
once the hub update settled healthy on the target version, every eligible agent
to the hub's version (#682). A durable plan in the key-value store survives hub
restarts; at most three agents update at a time; busy hosts are retried every
15 s for up to 5 minutes; every skipped host gets a terminal child run with its
reason. Agent contract 25 is unchanged.
- The Claude main thread planned the milestone, implemented the backend (S1),
  integrated the web step and made every correction after reviews.
- A Codex worker drafted the web control and progress (S2). Codex reviewed the
  design, S1 and the closing PR; Claude reviewed S2. The sandbox runs were
  delegated to Codex.

| Step | Review outcome |
| --- | --- |
| Plan, matrix and break map | The design review passed with blockers, folded in before kickoff. |
| S1 backend (plan, driver, admission, dispatch, routes) | Review failed with 6 blockers; the first verify failed on three partially fixed classes (persisted plan validation, admission identity, missing break-map evidence). Each class was fixed as a whole; the second verify passed. |
| S2 web (control, confirmation, progress, en/de) | Failed with 2 blockers: a hub skip before reservation left the combined view running, and the confirmation counted agents that would not follow. Both were fixed with the plan's hub row and an exact following-agent count. |
| Closing integration (#686) | Failed with 1 blocker: the count could still be too low while agent checks were loading. The confirmation is now offered only after every agent was read; a 5xx hub refusal also keeps its reason. The verify passed with no new findings. |

Assertion-failure red proofs cover the reviewed fixes and the break-map rows.

Maintainer decisions at kickoff: Q-1 agents in parallel, at most three; Q-2 a
busy host is retried every 15 s for up to 5 minutes, then skipped; Q-3 the
confirmation links to the hub backup instead of requiring one; Q-4 at-most-once
delivery to each agent (a crash between reservation and sending leaves the
agent's result unknown instead of sending twice).

Main-thread decisions: AUTO-1 the skip reason is activity metadata, so the
shared message vocabulary and the agent contract stay unchanged; AUTO-2 a
healthy hub settlement with a different running version fails the gate; AUTO-3
to AUTO-5 fixed the three S1 classes as a whole; AUTO-6 the parent activity
detail shows host rows only for the latest plan; AUTO-7 the exact
following-agent count and a failed combined view with the plan's reason.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
all Compose installations eligible for self-update, administrator role, 1440 px.
Each run built two packages of the same source that differ only in the version
stamp, served the higher one as newest stable release from a private feed and
installed the lower one on H, D and E.

| Cell | Behaviour | Result |
| --- | --- | --- |
| A1 | Whole update: hub, then D and E healthy on the target; one parent run with hub and agent children | PASS (candidate; A1r on delta 1 and exact main) |
| A2 | Hub health failure: parent failed, agents untouched and skipped `update.hub-failed` | PASS (candidate) |
| A3 | Offline agent skipped `agent.unavailable`, the other updated, parent failed as partial | PASS (candidate) |
| A4 | Hub restart during the agent phase: one operation per agent, no duplicate children, parent settles | PASS (candidate) |
| A5 | UI en and de: visibility, confirmation text and count, progress rows, activity detail | PASS (candidate; A5a count and A5b rows on delta 1; A5a on exact main) |
| A6 | Hub already current: no full-screen view, only the outdated agent updated | PASS (candidate) |

No exclusions were agreed or applied.

## Results

- Candidate `54cb5e75`: ACCEPTED 6/6.
- Delta 1 `3b4489f9` (after the S2 fixes): ACCEPTED 3/3 (A5a, A5b, A1r).
- Exact main `6a1046ef`: ACCEPTED 2/2 (A5a, A1r).

The candidate's first A1 attempt ended partial: the sandbox's registry relay
for the restarted hub was recovered only after the plan had read its agents, so
both were skipped `update.check-unavailable`. The repeat with immediate relay
recovery passed; both plan records were kept. This is a sandbox method
correction, and the product behaviour behind it is tracked in #687.

## Limits and follow-ups

- #687 tracks the nonblocking review and acceptance notes: a bounded read window
  after a lost POST reply, three guards without a failing test, a fleet read on
  every hub activity detail, a sleep instead of a barrier in one backend test,
  and a retry before skipping agents on a transient registry error right after
  the hub restart.
