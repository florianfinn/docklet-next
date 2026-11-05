# M18 updates and schedules acceptance — 2026-11-05

The accepted package is main `ba0cf2ae95c61f45fb1bc3b4c81096444a0870b1`. It is
the [M18 closing PR #413](https://github.com/florianfinn/docklet-next/pull/413),
squash-merged as `6827ad22`, plus one acceptance fix:
[#414](https://github.com/florianfinn/docklet-next/pull/414). The tree of
`6827ad22` equals the merged feature head `698ec093`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation:
- Codex GPT-6.1 Sol workers implemented the foundation, the restore points, the
  container and service updates and the web step, and Claude Opus reviewed them;
- the Claude main thread wrote the design notes, the follow-ups and every fix
  round, and Codex Sol reviewed them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #341, #346, #319, #343, #344 | Design notes for the update policy model, the schedule rule, deploy restore points and container and service updates | R1 failed; the notes were revised to v3 after a delta check. |
| #341, #242, #346, #319, #254 | S1 foundation: policy model, migration 018, update-check and window jobs, schedule rule and hub zone, agent contract 14 | R1 failed with 2 blockers: a manual image check fanned out and stopped at the first error, and one notify proof was aggregate. Verify 1 failed once more; verify 2 passed. |
| #344, #252 | S2 restore points before deploy and redeploy, startup reconcile | R1 failed with 3 blockers: evidence could name a removed point, lease contention lost a point, and startup reconcile failed hard. The fix verify passed. |
| #342, #343 | S3 standalone container update with previous-image restore, single service update | R1 failed with 1 blocker: a rebuild rewrote the reference. The fix verify passed. |
| #341, #345, #250 | S4 web: update settings, policy and install controls, recovery gestures | R1 failed with 3 blockers: the managed-container rule, the untranslated zone refusal code and the saved-zone display. The verify passed. |
| #248, #256, #265, #266, #267, #277, #280 | S5 follow-ups | R1 failed with 3 blockers in total: compact filter focus, degraded image spellings and the findings-chip guard. All were fixed, and their verification was folded into the closing review. |
| #347 | Closing integration (#413) | Sol failed with 1 blocker: equivalent reference spellings bypassed the shared-reference refusal. Fixed with two smaller findings; the fix verification passed. |
| #392 | Acceptance fix (#414) | Round 1 failed: the regression test was ordered by a sleep. It was rewritten with `testing/synctest`, and round 2 passed. |

## Defects found before acceptance

Six candidate runs on the integrated feature head found these defects. All were
fixed before the closing review:
- the web policy decoder rejected identities the hub really sends, and the hub
  accepted non-IANA zones;
- Direct agent requests were cut off after 10 s by a shared client timeout;
- every native update check downloaded the image manifest and used up Docker
  Hub's anonymous pull allowance. A HEAD lookup with a cache of unchanged
  digests now spends no pulls;
- update observations of several replicas contradicted their own state, and a
  container could show a check made for another tag;
- a standalone update compared the pulled index ID with the platform manifest
  digest, under the containerd image store;
- a scheduled container update was admitted under the wrong run ID and stayed
  unresolved;
- a locked container's update answered with a generic refusal code instead of
  `container.origin-locked`;
- a fresh bind backup plan showed UTC instead of the host zone;
- a stopped stack's open page kept showing it as running;
- a single-service update could move an image tag that another service in the
  stack still used;
- hub job runs showed English names and raw scheduler states in German.

During these runs the main thread decided AUTO-15 to AUTO-17: the HEAD-first
registry cache, keeping the German term "Pull & Redeploy", and refusing a
partial rebuild of a shared reference with `update.service-coupled`. The
maintainer decided Q-C4-1: when several local tags share one image ID, only the
tag the update record was checked for shows a result, and the others show
"unchecked". #412 tracks per-reference results.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups. The sandbox had the
hub-local host, one Direct agent and one Edge agent, a local OCI registry for
newer test image versions and the existing notification test channels.

Package checks on each run:

- migration ledger at 018 and SQLite quick checks on the hub and both agents;
- health UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract 14.

The accepted matrix has 22 rows and 43 applicable cells on hub-local, Direct
and Edge. They were exercised through the production UI with Playwright
Chromium in German and English at 1440 and 390 px. API, Docker and sqlite3 calls
prepared fixtures, injected deliberate faults and verified the results. It
covered:

- the upgrade from the M17 package with configured old policy states, checked
  row by row against K-1 and K-2;
- update policies, the global check every 6, 12 or 24 hours, notifications and
  their deduplication across checks and a restart;
- scheduled windows for stacks and standalone containers;
- manual container updates with previous-image restore, the origin lock, and
  single service updates against whole-stack updates;
- restore points before deploy and redeploy, recovery after a failed deploy,
  points held during an agent restart, and their retention;
- the shared schedule rule (test vectors and CI evidence for DST), the hub zone
  and the scan schedule;
- the update settings layout, narrow security tables and filters, platform
  coverage markers and refused upload archives;
- regressions in dashboards, backups and stack operations.

## Result

Every candidate fix was reviewed in the closing review. The runs on merged main
were deltas over candidate run 6: they repeated the cells that the fixes after
that run could affect, and reused the other cells' candidate evidence.

| Package | Result | Cause and fix |
| --- | --- | --- |
| `6827ad22` | Not accepted, stopped at the smoke gate | A Direct page received a 502 for a background vulnerability read of the Edge host, outside any fault window. The Edge agent had answered 200: the hub failed the request when a burst of response chunks filled its 256-message buffer. This was #392, first seen in M16. #414 makes response delivery wait up to 5 s for the reader, as stream data already did. |
| `ba0cf2ae` | **Accepted, 43 of 43 cells** | The smoke gate passed on H, D and E. The German activity view of a real partial update check (running and settled), shared-reference refusals in two spellings, and repeated update checks without Docker Hub pulls were rerun and passed. The other cells passed on candidate runs 4 to 6 and were reused. |

On `ba0cf2ae` no browser request received a 5xx, and the hub logged no
response-delivery warning. Two first update-check attempts failed their
allowance assertion. On H, a newly published Alpine version was correctly
fetched once. On E, the shared allowance refilled during the measurement. Both
second attempts passed. The first login after replacing the hub reached it
before it was ready and answered 502; the retry passed.

## Limits

- When several local tags share one image ID, only the tag the record was
  checked for shows an update check (#412).
- Settings › Updates keeps one table per host; the remaining reference-layout
  items are tracked in #411.
- Live DST transitions cannot be produced in the sandbox; the schedule rule is
  covered by shared test vectors and the CI run of the exact package.
- Notification delivery was checked against the sandbox test channels only.
- Direct and Edge agents ran on separate Docker daemons on one shared kernel.
  Containers for a foreign platform could be created but not run.
- Server-only 5xx completions without a browser-visible error remain in the
  request noise tracked in #389.
- The Edge agent drops stream input when its buffer is full (#415), and a slow
  reader can delay other traffic on the same tunnel (#416).
- Chromium only, over HTTP/1.1 through the sandbox's proxy.

## Open follow-ups

#389, #406, #407, #408, #409, #410, #411, #412, #415, #416.
