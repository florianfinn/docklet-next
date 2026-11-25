# Update-check progress and results

Update checks (`update-check-host` for one host, `update-check` for all hosts) report what they
check, how far they are and what they found. This note records the contract behind that report.

## Plan and counts

- A check knows its targets after inventory discovery: stacks plus eligible standalone containers.
  The job records them with `jobcontext.Plan(ids, known)`. The coordinator persists the plan in the
  run outcome (`plannedIds`, `plannedKnown`) under the execution-ownership fence, and the final
  settlement keeps it, because a job returns its targets but not its plan.
- The all-host check adds each host's targets as that host is discovered. The plan is complete
  (`plannedKnown`) only after the last host in the run's host snapshot. A run without hosts has a
  complete, empty plan.
- An undiscoverable host fails every retained policy target, or one environment target when it has
  no policies, and plans exactly those targets.
- Counts are derived, not stored. `Outcome.Counts()` counts only targets in the current plan, so
  targets kept from an earlier attempt (retry, recovery rediscovery) never inflate a new attempt's
  numbers. Runs without a plan (earlier releases, single-target jobs) report no counts.
- The job Activity carries the counts as `bulk` (`unit`, `planned`, `plannedKnown`, `completed`,
  `succeeded`, `failed`, `skipped`, `updates`) on the base Activity. List, detail and stream envelopes
  therefore all carry it. It is re-projected with every persisted target result. Full target rows
  stay in the run read and never in the stream.

## Target results

- Each target result carries a display-name snapshot (`name`) and `updateAvailable`. Links resolve
  at render time from `targetKind`, `environmentId` and `targetId`. A container's `targetId` is its
  stable name, so a link opens the current logical container, including a replacement instance.
- A saved stack whose image is not present on the host has nothing installed to compare. It is
  `skipped` with `update.image-not-pulled`, not failed. Every other unavailable check stays a failed
  target with `update.check-unavailable`. As for any skip, the policy keeps its last successful
  check time and candidates. Images are never pulled to make a check possible.
- Check jobs aggregate their targets as follows. Failed: at least one failure and no success.
  Partial: successes and failures. Skipped: every target skipped. Succeeded: otherwise. Auto-update
  windows keep their own aggregate.

## Waiting phase

Submission records `deferredBy` when an unfinished run of the same job is ahead of the new run.
Only such a run projects a `queued` phase. A run that starts without a predecessor goes straight to
`running`. Coalesced manual host checks join the existing run and add no phase. Capacity waits inside
the job engine are not exposed. Elapsed time is never taken as evidence of waiting.

## Presentation

The web separates phases (an optional waiting phase, then running) from the outcome. A terminal
status is never shown as a failed phase. Bulk runs show "k of N checked" while running ("k checked
so far" while the plan is incomplete), where k counts successful checks so it never drops at the
end; failed and skipped counts follow. The outcome adds the update count, with zero parts left out.
Item rows show the name with a link when the target still resolves, a status tone and the localized
reason.
