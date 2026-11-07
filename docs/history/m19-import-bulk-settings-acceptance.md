# M19 import, bulk actions and settings parity acceptance — 2026-11-07

The accepted package is main `d17b6224b19061c4a78160b782da0f94cb174cfe`. It
consists of two squash merges:

- the [M19 closing PR #423](https://github.com/florianfinn/docklet-next/pull/423),
  merged as `fd950832`;
- one acceptance fix, [#424](https://github.com/florianfinn/docklet-next/pull/424).

The tree of `fd950832` equals the merged feature head `ec0dffa2`. The squash
commit was first published with a wrong message, as `d836c811`. With the
maintainer's approval, main was rewritten once: the new commit has the same tree
and the same parent, and its message is the PR text. The ruleset was suspended
for about two minutes for this and is unchanged.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:

- Codex GPT-6.1 Sol workers implemented the wire-code guard, the backend
  foundations, the import resolver and the web step, and Claude Opus reviewed
  them;
- the Claude main thread wrote the design notes, the notification and scan
  follow-ups and every fix round, and Codex Sol reviewed them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #348–#356 | Design notes for stack import, bulk actions, notification targets, unhealthy notices and cooldown, activity retention and global search | R1 failed with 12 blockers. A delta check failed once more on the import note, which was revised to v3. |
| #308 | S0 wire-code guard: every contract code the web renders maps to a catalogue entry | R1 failed with 1 blocker: producer text of carrier codes was hidden. The fix verify passed. |
| #351, #352, #353, #355, #300, #299 | S1 backend foundations: named targets, migration 019, unhealthy history, Auto Heal cooldown, retention, agent contract 15 | R1 failed with 1 blocker in the unhealthy history. The fix verify passed. |
| #348 | S2 import resolver: GitHub raw files and registries, bounded egress | R1 passed with 5 nonblocking findings, all fixed. The verify passed. |
| #354, #308, #269, #270, #301, #303 | S3 link base, readable notices, scan receipt repair, classified delivery errors, event order | R1 failed with 3 blockers: Telegram lost line breaks, ntfy had no title, and the test endpoint had no guard. The fix verify passed. |
| #348, #349, #350, #351, #353, #355, #356 | S4 web: import, bulk actions, image details, targets, cooldown, retention and the search palette | R1 failed with 1 blocker: host results opened a missing page. All ten findings were fixed and the verify passed. |
| #357 | Closing integration (#423) | Sol passed with 2 wording findings, both applied. Each candidate fix before it had its own verify; one verify round failed on a response contract and the correction passed. |
| #356 | Acceptance fix (#424) | The independent review passed with no findings. |

## Defects found before acceptance

An early smoke and four candidate runs on the integrated feature head found
these defects. All were fixed before the closing review:

- a manual connection test of a Direct agent, or an Edge heartbeat, brought the
  agent back online without resending the synced retention settings. Only the
  scheduled health job did. Every authenticated online write now announces the
  transition once;
- the German search palette had English accessible names for its input and
  result list;
- a bulk action refused an item as busy when the same command, started outside
  this browser, was already running on it. The hub now decides, and joins an
  identical command;
- a command for a stopped Direct agent or a disconnected Edge agent failed
  before admission with an uncoded 503, so the bulk summary showed a generic
  error. It now answers `agent.unavailable` in the envelope each route declares;
- the environment schema inventory test did not list the new import mirror
  variable.

During these runs the main thread decided AUTO-6 and AUTO-7. AUTO-6 lets the hub
decide about a same-kind run started elsewhere. AUTO-7 keeps the health retry
backoff, so automatic recovery after an outage is bounded by about five minutes.
AUTO-1 to AUTO-5 were decided during the build. All seven are recorded on #357.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The sandbox had the
hub-local host, one Direct agent and one Edge agent, a local OCI registry for
import and update fixtures, a fixture server for GitHub raw files and
notification receivers.

Package checks on each run:

- migration ledger at 019 on the hub and both agents;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract 15.

The accepted matrix has 23 rows and 40 applicable cells on hub-local, Direct
and Edge. They were exercised through the production UI with Playwright
Chromium in German at 1440 and 390 px and in English at 1440 px. API, Docker
and sqlite3 calls prepared fixtures, injected deliberate faults and verified the
results. It covered:

- the upgrade from the M18 package with one notification target, Auto Heal
  settings and existing history;
- stack import from GitHub raw files and a registry, its typed refusals and its
  administrator-only access;
- bulk start, stop, restart, redeploy and update across hosts, including joined,
  skipped, busy and unreachable items in one summary;
- image details, named notification targets, readable notices with the link
  base, and the unhealthy-container notice after 10 minutes with one notice per
  hour;
- the Auto Heal cooldown and its invalid-value hint;
- activity retention with pruning, resend to agents after reconnect, and
  persistence across restarts;
- newest-first events, the search palette for administrators and simple users,
  and its German labels;
- the named tests of the CI run of the exact package, and a short regression of
  stack operations, a stack update, a volume backup and the dashboard.

## Result

Every candidate fix was reviewed in the closing review. The runs on merged main
were deltas over the last candidate run: they repeated the upgrade, the cells
that later commits could affect and a short regression, and reused the other
cells' candidate evidence.

| Package | Result | Cause and fix |
| --- | --- | --- |
| `fd950832` | Not accepted | The palette hid the "Check all updates" quick action. The action runs the check-all operation, but was gated on the per-target check. #424 gates it on the operation it runs. The hub-local regression stayed incomplete because the runner used up its attempts. |
| `d17b6224` | **Accepted, 40 of 40 cells** | The smoke gate passed on H, D and E. The palette, its German labels and the regression on H, D and E were rerun and passed. The hub-local regression needed a second bounded run after a runner error. The other cells passed on the earlier runs and were reused. |

On the exact main runs, no browser request received a 5xx outside a deliberate
fault window. Automatic recovery of a restarted Direct agent took 148 s,
including the back-online notice.

## Limits

- The upgrade on exact main was a backed-up redeploy over M19 state. The
  upgrade from M18 data was proven on the early smoke; migration 019 has not
  changed since.
- The CLI commands `notifications delete`, `test` and `set` still use the
  removed per-provider routes until the CLI is removed in M20 (AUTO-1).
- Registry imports that need more than six requests fail with
  `import.source-unavailable` (AUTO-4).
- `configs` and `secrets` entries with a relative file path and absolute build
  contexts get no missing-requirement code (AUTO-5).
- A host back after an outage is marked online by the health job within about
  five minutes, or at once by a manual connection test (AUTO-7).
- Events that arrive after a prune stay until the next prune.
- Two passing cells logged start-up 401s and aborted requests without a visible
  error (#425).
- Direct and Edge agents ran on separate Docker daemons on one shared kernel.
- Chromium only, over HTTP/1.1 through the sandbox's proxy.

## Open follow-ups

#418, #419, #420, #421, #422, #425.
