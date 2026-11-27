# Update everything

M46 (#682). One confirmed administrator action updates the hub first and then, after the hub
update settled healthy on the target version, every eligible agent to the hub's version. It builds
on the correlated operations of [in-app updates](app-updates.md); the earlier update-all routes
stay removed.

## Decisions

- **Durable plan.** Admission writes one plan into the key-value store (`selfupdate.fleet`, one slot)
  and its parent activity (`system.update-all`) in one transaction. The plan freezes the target
  version, the registered agents with credential revision and registration time, and one operation
  ID per host derived from the plan ID. A second admission while a plan is unsettled is refused with
  `update.all-busy`. When the hub is already current, admission needs at least one compatible agent
  below the hub version, and that agent's registration must still be the one the plan freezes;
  otherwise it is refused with `update.current`. The plan has a schema
  version; an unknown version, a missing field or a state combination the driver cannot advance fails
  closed (`update.fleet-unreadable`) and starts nothing. Admission replaces only a validated settled
  plan or the release marker; any other stored value is released first, failing its parent.
- **Driver.** The hub's update service advances the plan and resumes it on every start, like the
  remote observers. The process that admits the plan is replaced by the hub update; the new process
  continues from the stored state.
- **Hub first.** The hub operation is reserved with its planned ID. A reserved operation found after a
  restart is joined, never reserved again; a helper is launched only by the reservation that created
  it. The parent activity is exempt from host admission only for its own hub child.
- **Gate.** Agents start only when the planned hub operation settled healthy and the running hub
  version equals the target. Any other settlement ends the run as failed; every agent is skipped
  with `update.hub-failed` and no agent request is sent.
- **Agents.** At most three agents update at a time. Each agent is read first; an offline agent,
  a host below the minimum agent contract (read explicitly, because the update recovery routes
  answer older agents too), a manual or custom installation, a host on the target or a newer
  version, a replaced registration and a host the starting administrator may no longer update are
  skipped with their code. Authorization is checked right before each reservation. A busy
  host is retried every 15 seconds for up to five minutes, then skipped with `host.busy`. The
  reservation compares the frozen credential revision and registration time in its transaction.
- **At most once.** Only the caller that moves an agent operation from reserved to helper-started
  sends its single request. An operation still reserved after a restart is observed, never sent; it
  settles as unknown at its deadline. A lost reply after the request is observed, never repeated.
- **Children.** Every host has a child activity in the parent's batch: the hub and agent update
  runs, and a terminal child for each skipped host, the hub included (success for current,
  cancelled otherwise). Message codes stay in the shared activity vocabulary; the reason is in the
  child's metadata, so the agent contract is unchanged.
- **Result.** A failed hub fails the run. All hosts updated or current: success. Otherwise the run is
  partial when the hub or at least one agent updated, or when only non-current skips remain;
  it fails when nothing updated and an update failed. Partial uses the job convention: status failed
  with `jobStatus: partial`, plus counts.
- **Routes.** `POST /system/upgrade/all` (system upgrade permission, body: the confirmed target
  version) and `GET /system/upgrade/all` (active or last plan, one row per host). Both are hub-only;
  the agent contract is unchanged.
- **Backup and scheduling.** The confirmation links to hub backups as the hub update does; no backup
  is required. The action is manual only. A retry is a new confirmed action.

Rejected alternatives: a scheduler job run as carrier (no atomic initial payload, scheduler reasons
and target types do not cover hosts); browser-driven agent fan-out (the update view reloads the
page); replaying a reserved agent request after a restart (it can duplicate an update).

## Break map

| # | Case | Guard | Test |
|---|---|---|---|
| 1 | concurrent admissions | one unsettled plan per slot | eight parallel admissions |
| 2 | admission, then restart before the hub reservation | the started driver reserves the planned operation once | admission with a started service |
| 3 | restart after the hub reservation, before the plan recorded it | join the planned operation | reserve, new service, no second reserve |
| 4 | hub reserved, helper never launched | no relaunch; the observer settles unknown, the run fails | no second reservation; unknown settlement fails the run |
| 5 | parent blocks its own hub child | exempt exactly the parent | another running activity still blocks |
| 6 | hub settled healthy on another version or operation, or failed | gate on planned ID, healthy and version | outcome table, zero agent operations |
| 7 | hub refused before any run | terminal hub child with the reason | skipped hub child |
| 8 | reserved agent operation after restart | observe only | restarted hub sends nothing |
| 9 | marker written, request never sent | observe; unknown at the deadline | restarted hub settles unknown, zero requests |
| 10 | dispatched agent operation, two restarts, restart before the aggregate | observe only, one aggregate | Direct and Edge, one request across three hub processes, one child per host |
| 11 | dispatch marker | reserved-only transition | second caller does not acquire |
| 12 | registration replaced during the agent check | frozen identity in the reservation transaction | re-registration while the check is held |
| 13 | offline or failing agent | skip and continue | closed port and 503 |
| 13a | recovery check answers 503 or drops the connection | skip and continue | both modes, zero requests |
| 13b | Edge agent without a live tunnel | the fleet path registers tunnel demand | on-demand Edge agent connects and receives its one request |
| 14 | agent below the minimum contract | explicit contract read | Direct and Edge, zero requests |
| 15 | agent newer or current after admission | never downgraded | newer and current cases |
| 15a | feed or hub version changes after admission | frozen target version | hub on a newer version, feed offering it, operation keeps the plan target |
| 16 | hub current and no agent below the target | refused admission | no agents, all current, all newer, only old contracts; qualifying host removed during admission |
| 17 | busy agent | bounded retry, then skip | retry state, skip after the limit |
| 18 | permission lost during a round | check at each reservation | revocation while the check is held |
| 19 | parallel limit | at most three in flight | fourth agent stays pending |
| 20 | unreadable or inconsistent plan | fail closed, slot released, never overwritten | future schema, corrupt value, invalid phase and states, settled with open targets |
| 21 | parent during restart repair | key-value owner exclusion | startup repair keeps the parent |
| 22 | agent surface | hub-only routes | agent route set has no combined route |

Browser rows (lost start reply, reload, logout, progress polling) belong to the web step.
