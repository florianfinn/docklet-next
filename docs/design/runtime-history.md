<!--
Source: docklet-next/issues/335
Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
Retrieved: 2026-11-03
License: Apache-2.0
Deviations: Original runtime history, condition, capacity and load alert rationale.
-->

# Runtime history, conditions and load alerts

Decision for #335, #336, #337, #338 and the data part of #371 (M17, maintainer kickoff
2026-11-03; history sections, grey Docklet stops and hub-only load alerts were agreed on
2026-10-30).

The hub samples every host, container and stack once a minute and keeps a downsampled
history in SQLite. The same classification drives the history bars, the dashboard
fleet ring and the sustained-load alerts. Docklet is a homelab tool with normally one
administrator; the rules below keep the data truthful, they do not add monitoring
infrastructure.

## 1. Sampling

- A hub-only job runs at second 0 of every minute. Each enabled host gets one sample,
  at most one in flight per host, with a 30 second budget and real cancellation.
- The sample comes from the agent route `GET /api/environments/0/history/sample`: the
  hub-local host calls the service in process, Direct hosts are read with a 4 MiB
  response limit and Edge hosts through the `history.sample` command with the same
  collection limit. The route belongs to agent contract v13; the minimum agent version
  is 13, so agents are updated together with the hub.
- A sample carries cumulative counters and the hub computes deltas:
  - host CPU ticks, memory used and total, disk used and total for the configured
    disk path (and the path itself), network bytes over non-loopback interfaces;
  - every container's id, name, Compose project and service, Docker state, health,
    exit code, OOM flag, restart count, start and finish time and stop intent;
    running containers add one-shot Docker stats (CPU, memory and limit, network,
    block I/O);
  - every Docklet project on the host with its id, name and status, so a stack that
    was taken down without containers is still known.
- The decoder checks counts (at most 2,000 containers and 1,000 projects), name length,
  finite non-negative numbers, known states and duplicate identities. An invalid or
  oversized sample marks the host minute as down and writes nothing for its children.
- The minute is taken from the hub clock at job start. Before writing, the hub checks
  that the host still exists, is enabled and has the same endpoint and credential
  revision as at the start, and that the minute is newer than the last one written.
  A late or outdated result is dropped.
- Previous counters, restart windows, load episodes and the latest conditions are kept
  in memory per host and host revision. A rate needs the same container id and growing
  counters; otherwise that minute has a state but no rate.

## 2. Conditions

Per container, by Docker state:

1. `restarting` → **unhealthy** (restart loop).
2. `running`: restart count grew by at least three within the last ten sampled minutes of
   the same container → **unhealthy** (restart loop); failing health check →
   **unhealthy**; otherwise **running** (also while the health check is starting).
3. `paused` → **stopped**.
4. `exited`, `created`, `dead`:
   1. a Docklet stop intent for this container (section 3) → **stopped**, whatever the
      exit code;
   2. created and never started → **stopped**;
   3. dead or killed for memory → **down**;
   4. exit code 0 or 143 → **stopped** (a clean stop outside Docklet);
   5. any other exit code → **down**, with the code kept.

A host is **running** when its sample was written and **down** when it was unreachable or
its sample failed; its containers and stacks then have no data for that minute. A stack
takes its worst container (down, then unhealthy, then running). It is **stopped** only
when every container is stopped; a stack with running and stopped containers is
running. A project without containers counts as stopped when Docklet stopped it.

The web maps the four conditions to the four colours of the fleet ring decision
(#371): green running, yellow unhealthy, red down (an unreachable host is drawn pale and
dashed), grey stopped. The same mapping is used for status dots, lists, pages and
history bars. Live inventory decides running, restarting and health at once; the hub
condition decides grey or red for a stopped container and adds the restart-loop signal.

## 3. Stop intents

Every Docklet stop runs in the process that owns the Docker daemon: container stop
(which Stop All uses), stack stop and stack down. After a successful stop that process
records an intent per container id in its own database. The sample reports an intent
only while Docker's start time of that container is not later than the intent; any later
start ends it, whoever started the container. A new container with the same name has a
different id and no intent. Failed stops record nothing. Hub-local, Direct and Edge hosts
therefore behave the same without a hub-side hook.

## 4. Storage and retention

- One table of buckets per entity (host, container by name, stack by project id) and
  tier: 15-minute buckets for 24 hours, 2-hour buckets for 7 days and 8-hour buckets for
  30 days, aligned to UTC. Each bucket holds minutes per condition, CPU and memory sums,
  counts and peaks, the memory limit, network and block I/O deltas, the last exit code
  and, for hosts, the last disk usage and its disk path.
- Each minute writes one transaction per host with three upserts per entity. Every upsert
  only counts a minute newer than the bucket's last minute, so a repeated minute after a
  restart counts once.
- An hourly job deletes buckets older than their tier. Removing a host deletes its rows.
- A stable entity has at most 273 rows (about 50 KB); 300 entities need about 15 MB.
- Container history follows the container name across recreate, redeploy and update.

## 5. Reading

| Route | Access |
| --- | --- |
| `GET /environments/{id}/containers/{containerId}/history?range=24h\|7d\|30d` (name) | like reading the container |
| `GET /environments/{id}/projects/{projectId}/history?range=` | like reading the project |
| `GET /environments/{id}/runtime-conditions` | administrators: all; users: their assigned projects |
| `GET /environments/{id}/history/host?range=`, `GET /history/capacity` | administrators |

These paths are served by the hub and never forwarded to an agent. A range returns 96,
84 or 90 buckets, oldest first, with the open bucket last. A bucket shows the worst
condition that has minutes in it (down, unhealthy, running, stopped), otherwise no data.
Uptime is (running + half of unhealthy) divided by (running + unhealthy + down); stopped
and missing minutes do not count. A simple user's container history needs the owning
host online, because membership is read there.

Capacity growth is the least-squares slope of disk usage over the 2-hour buckets of the
last seven days that use the current disk path, after at least 24 hours of data. The
dashboard capacity widget shows used of total, the time until full, the daily growth and
a 30-day line.

## 6. Sustained load alerts

Hub settings: alerts on (default), CPU threshold 90 % (50–100), memory threshold 90 %
(50–100) and duration 10 minutes (5–60). CPU is a share of the host; container memory is a
share of its limit, or of host memory without a limit. CPU and memory are evaluated
separately for hosts and running containers: when every one of the last N written minutes
is above the threshold, one alert is raised, and the metric is armed again after N minutes
below it. A minute without a value breaks the run. The alert always appears in the
Activity Center; the notification event `sustained_load` is off for every target until an
administrator selects it. Episodes are kept in memory, so a hub restart starts them again.

## 7. Break map

| ID | Row | Decision | Test |
| --- | --- | --- | --- |
| HB1 | repeated minute | last-minute guard per tier | same and later minute twice on a reopened database |
| HB2 | counter reset, new id, hub restart | rate only with the same id and growing counters | first, replaced and reset samples |
| HB3 | unreachable host, partial sample | host down, children no data, missing stats keep state | offline host, stats error, recovery |
| HB4 | name continuity | key by container name | new id continues, rename starts new |
| HB5 | stop intents | owning-host producers, start-time ordering | real Direct and Edge paths, failed stops, later start, same-name replacement |
| HB6 | condition order | section 2 | crossed cases such as restart loop then stop |
| HB7 | tiers and retention | three tiers, 96/84/90 slots | injected clock at every edge |
| HB8 | sample size and shape | 4 MiB at both transports, validation | oversized bodies through the real Direct and Edge paths |
| HB9 | Direct and Edge parity | route and command in agent and Edge modes | same sample from both transports |
| HB10 | migration 016 | new tables, outbox event list, preference column | upgrade through the real initializer with outbox rows |
| HB11 | shared snapshot | immutable snapshot, deep copies | a consumer mutating its copy changes nothing else |
| HB12 | late result, host edit | commit gate | result released after removal, disable or revision change |
| HB13 | host removal | rows and memory state removed | removal during a sample |
| HB14 | access | section 5 rules | real middleware chain, users with and without assignment |
| HB15 | uptime and bucket state | section 5 formulas | mixed buckets, all stopped |
| HB16 | load episodes | per metric, once, re-arm, gaps | injected minutes |
| HB17 | new codes | events, activity type, notification event, error codes | contract and consumer tests |
| HB18 | capacity growth | slope over the current disk path | series with gaps and a path change |

## 8. Limits

- No minutes are recorded while the hub is down.
- A stop outside Docklet with an exit code other than 0 or 143 counts as a failure.
- After a hub restart or a host edit, rates resume one minute later and load episodes
  start again; each container that is down, unhealthy or in a restart loop is reported
  once more as a condition event, and a load that lasts through the restart can alert
  again.
- A failed host minute is no transition: condition events and load episodes continue
  from the last successful minute. A container that disappears between listing and
  inspection is left out of that minute and loses its stop intent. One whose inspection
  times out is listed as uninspected: it gets no minute but keeps its stop intent, its last
  condition and its alerted load episode. Any other inspection error fails the host minute.
- Commits of all hosts share one lock, and the runtime conditions route waits for a
  running commit; this was not measured and is checked in practical acceptance.
