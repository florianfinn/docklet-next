# Image cleanup progress and compact report

The image cleanup dialog shows how far each host is and how a cleanup ended, using counts rather than
long name lists. This note records the contract behind that view (issue #661).

## Producer

- Each host prunes its own images. Once the guard holds the host's image lease and has selected the
  images, the prune handler starts an `image_prune` activity. A prune without candidates records no
  activity. Because of the lease, a host runs at most one cleanup at a time.
- After every processed image the activity's `bulk` counts are rewritten (`unit: "target"`;
  `planned` = selected images; `completed`, `succeeded` = removed, `skipped`, `failed`). The activity
  ends successfully when no image failed, including when every image was skipped. It fails when at
  least one removal failed or on a panic. A listing failure happens before the activity exists and
  is reported by the request alone. A cancellation stops before the next
  image and keeps the counts so far. A removal that cancellation cuts off is counted neither as
  removed nor as failed.
- A failed progress write is logged and never repeats or stops a removal. Startup repair settles an
  activity that a restart left running and keeps its counts.
- The prune request still returns the full synchronous report. Neither its contract nor the
  agent/hub contract changes, and scheduled and system cleanups prune without an observer.

## Transport

Hub-local activities reach the web through the activity stream right away. Direct and Edge agents'
activities arrive through the hub's regular remote activity poll (every 5 s), so their progress
moves in steps of that interval. The Edge tunnel delivers HTTP bodies only once they are complete,
so the activity is the only live channel there. An agent without `image_prune` activities shows
indeterminate progress and the report at the end.

## Presentation

- The web keeps `bulk` only when its counts are whole, non-negative and consistent. Otherwise
  progress stays indeterminate.
- The dialog records the image cleanup runs it already knows before it submits. While the request
  is pending it binds, for each host, the first run that was not among them and never moves that
  binding; after the request settled no new run is bound. This needs no clock comparison between
  hub and agent. The last valid counts of a bound run stay with the dialog session, also when the
  run later leaves the list.
- Start view: counts of images to remove, their size and what is kept. The names sit behind
  "Show details".
- Running view: one row per host with "k of N images processed" and a bar. A host that has not
  reported its plan yet shows a spinner.
- Report: totals when there are several hosts, then one row per host with its counts and reclaimed
  space. Failed and skipped images always show their reason; removed names are collapsed.
- When a host's request ends without a result (for example the Edge proxy timeout), its row shows
  the error and the last observed counts, marked as incomplete. It is never presented as a complete
  report.
- Image cleanup runs get no run toast. A cleanup that finishes after the dialog closed reports one
  compact toast with the totals and the hosts that had no result.
