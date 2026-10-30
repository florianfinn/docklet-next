# M13 notifications and Auto Heal acceptance — 2026-10-30

The accepted package is main `5a1c021284a9c458d18ac3aed25d7cb512709750`, the
squash merge of the
[M13 closing PR #306](https://github.com/florianfinn/docklet-next/pull/306).
Its tree equals the merged feature head `80e4201a`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #292, #294 | Shared agent access codes on proxied routes; test isolation of the volume usage cache | Passed with 2 nonblockers |
| #296 | Notification outbox and hub delivery, live producers, Auto Heal per host, migration 013, agent contract v10 | Design review failed with 10 blockers and its delta verification with 6, both folded into the notes before implementation; step review passed with 10 nonblockers and 3 product questions |
| #297 | Live Settings › Notifications and Settings › Auto Heal | R1 failed with 5 blockers, R2 with 1 blocker (write ownership across navigation), R3 passed |
| #298 | Closing integration (#306) | Sol closing review passed with 1 nonblocker; Opus closing review failed with 1 blocker and 7 nonblockers; five verification rounds of the fix chain failed with 3, 2, 4 and 1 blockers, the fifth passed on `2e3c5498`. The minor send-result finding from that pass was fixed in `80e4201a` without another review round. |

## Defects found before acceptance

An early smoke on the Settings and producer flows found five product defects;
`eed5fd2` and `68ed0f5` fixed them:

- undeclared Auto Heal reason codes left runs running, so later containers were
  never healed;
- an interval change while Auto Heal was disabled was refused;
- the delivery-failure event lost its reason and locale;
- Auto Heal restarts showed as generic activity at 0 %;
- target actions overlapped the event badges at 390 px.

The first candidate run found that `notification-deliver` stopped claiming rows:
a retried admission appended an already joined run again. `b678c74` fixed it and
startup now repairs duplicated run IDs. Two required automated tests were added
(`cb36cac`, `e1db8d0`). The second and third candidate runs passed the remaining
cells, including the poll-only Edge standby case.

The closing reviews found one error class, "a terminal notification ignores
cancellation of its work", first as a cancelled restore that still notified. Four
failing verification rounds found further producer paths. The class was rebuilt
from a table of every producer with one rule: work is interrupted, and silent,
when cancellation or shutdown arrives before its outcome is determined; a
determined outcome is notified even when shutdown interrupts saving or sending
it. `40d721e`, `91cd1a1`, `bb08d65`, `eda9176`, `2e3c549` and `80e4201`
implemented it for restores, volume and hub backups, deployments and Auto Heal,
and bound an agent producer's wait for its send to 30 seconds on every
transport.

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups, with the `unless-stopped`
restart policy from the install templates. The sandbox had the hub-local host,
one Direct agent and one Edge agent.

Package checks:

- migrations 0–13 applied on the hub and both agents;
- `/api/health` UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract v10;
- main push CI and the identical-tree PR-head CI passed.

The run applied the agreed matrix through the production UI with Playwright
Chromium, in German and English at 1440 and 390 px, on every applicable host
type:

- notification targets for webhook, ntfy, email and Gotify against local test
  sinks: create, edit without re-entering secrets, disable, delete, field
  validation, test sends with typed failures, per-event toggles, and one sink
  down while another receives;
- deployment, backup and restore failure notices once each, and silence for
  cancelled, successful and safety-capture work;
- host offline after more than five minutes and back online, silence for a short
  stop and a hub restart, and a poll-only Edge standby host that stayed silent
  for 380 s;
- host-side delivery of an agent backup failure and an Auto Heal outcome over
  Direct and Edge;
- delivery retries, expiry, a hub restart during delivery, a hub database backup
  with a pending delivery and a database restore without an alert burst;
- the upgrade from the M12 package with data, keeping the notification
  configuration;
- Auto Heal per host: settings, environment-managed read-only hosts, restarts
  with activity and notice, the restart limit across a process restart,
  exclusions, busy ownership, refusal of contract v9 agents and recovery after a
  process restart;
- the shared agent access codes on proxied routes, layout and translations.

Two extra checks covered the closing fixes. A volume restore started through
the UI and cancelled through the activity-cancel API stayed silent on Direct
and Edge, while the failing-restore control notified once. Eight named
regression tests of the closing fixes passed on the exact package.

## Result

The acceptance runner concluded **accepted**. Of the 84 matrix cells, 52 passed
and 32 do not apply; no cell was excluded. Both extra checks passed.

No product defect was found in this acceptance run. Failed attempts were harness
issues; one harness kept a browser open across related rows, and every affected
UI slice was rerun with separate browsers. All attempts are retained in the
private evidence.

In the sandbox the first offline notice was observed 334 s (Edge) and 449 s
(Direct) after the agent stopped; the threshold is five minutes.

## Limits

- Host-side events produced while the hub is unreachable are lost; delivery is at
  least once.
- Runs interrupted by a process restart are not notified. While an Edge tunnel
  stays stalled, each further determined agent notice keeps one background send
  until the tunnel progresses or closes.
- The interrupted Auto Heal attempt has no practical cell; its timing window
  cannot be hit reliably, and automated tests cover it.
- Discord, Telegram, Signal, Slack, Pushover, Matrix and Google Chat were checked
  through forms and typed test results only, without real accounts.
- Chromium only, over HTTP/1.1 through the sandbox's public proxy.

## Open follow-ups

#299, #300, #301, #302, #303, #304, #305, #307, #308, #309.
