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
  `update.all-busy`. The plan has a schema version; an unknown version or a malformed plan fails
  closed (`update.fleet-unreadable`) and starts nothing.
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
  a manual or custom installation, a host on the target or a newer version, a replaced registration
  and a host the starting administrator may no longer update are skipped with their code. A busy
  host is retried every 15 seconds for up to five minutes, then skipped with `host.busy`. The
  reservation compares the frozen credential revision and registration time in its transaction.
- **At most once.** Only the caller that moves an agent operation from reserved to helper-started
  sends its single request. An operation still reserved after a restart is observed, never sent; it
  settles as unknown at its deadline. A lost reply after the request is observed, never repeated.
- **Children.** Every host has a child activity in the parent's batch: the hub and agent update
  runs, and a terminal child for each skipped host (success for current, cancelled otherwise).
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
| 2 | restart after hub reservation, before the plan recorded it | join the planned operation | reserve, new service, no second reserve |
| 3 | parent blocks its own hub child | exempt exactly the parent | another running activity still blocks |
| 4 | hub settled healthy on another version or operation, or failed | gate on planned ID, healthy and version | outcome table, zero agent operations |
| 5 | reserved agent operation after restart | observe only | restarted hub sends nothing |
| 6 | dispatched agent operation after restart | observe only | Direct and Edge, one request across two hub processes |
| 7 | dispatch marker | reserved-only transition | second caller does not acquire |
| 8 | replaced registration | frozen identity in the reservation transaction | refused reservation |
| 9 | offline agent | skip and continue | offline host skipped, other host updated |
| 10 | agent newer or current | never downgraded | newer and current cases |
| 11 | busy agent | bounded retry, then skip | retry state, skip after the limit |
| 12 | permission lost | no new effect | permission check before effects |
| 13 | unreadable plan | fail closed, slot released | future schema and corrupt value |
| 14 | parent during restart repair | key-value owner exclusion | startup repair keeps the parent |
| 15 | agent surface | hub-only routes | agent route set has no combined route |
