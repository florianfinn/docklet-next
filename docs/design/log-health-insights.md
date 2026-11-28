# Log and health insights

Milestone M47 (#695): #692 Health and Logs tabs, #693 disruption timeline, #694 pattern mode.
The milestone combines data Docklet already has. It adds no model, no background log capture and
no new table.

## Shared web foundations (owner: S1, #692)

### Log level from the line itself
The hub serialises every ordinary log record with wire level `info`
(`backend/api/ws/log_records.go`). Stream names never carry severity, and only a log source failure is
sent as `error`. On a real hub the Logs tab level filter therefore matches nothing above info. S1 adds
`web/src/shared/logs/level.ts` with `classifyLevel(message)`:
- `mapLog` uses it when the wire level is `info`, missing or unknown.
- A wire `warn`, `error` or `debug` is kept. That covers a source failure and the demo adapter's
  levels.
- The backend severity contract does not change.

Test fixtures follow the real producer: a JSON frame with wire level `info` whose message is a JSON
`level: "error"` line maps to `error`.

The classifier stops at the first rule that matches:
1. A JSON object line: the string value of `level`, `lvl`, `severity` or `log.level`.
2. logfmt: `level=` or `lvl=` as a key.
3. A bracket marker or level token at the start of the message, after an optional timestamp:
   - `[E]`, `[W]`, `[I]`, `[D]`;
   - `[error]`, `[warn]`, `[warning]`, `[info]`, `[debug]`, also `[Error]` and similar;
   - an upper-case `ERROR`, `WARN`, `INFO` or `DEBUG` token;
   - the .NET forms `fail:`, `warn:` and `info:`.
4. Keyword fallback only when no rule above matched: whole-word `error|fatal|panic|exception` gives
   `error`, and whole-word `warn|warning` gives `warn`.

Values map onto the four levels: `fatal`, `crit`, `panic` and `err` become `error`; `warning`
becomes `warn`; `trace` becomes `debug`. `stderr` alone is not an error, because many services log
everything to stderr. The prototype's misread lines are test fixtures:
- a level-INFO line that says "without giving any error";
- a JSON `{"level":"info","logger":"gunicorn.error"}` line;
- a `[W]` line whose text contains "error".

### Bounded log read
`subscribeLogs` keeps one shared live channel with `follow=true&tail=100` and a 2,000-line buffer.
Health "last lines", the combined disruption window and pattern mode need a one-shot read that ends.
S1 adds the adapter operation `readLogs(ref, options, sink, signal)`:
- `ref` is `{kind: "container" | "stack", id}`.
- `options` is `{tail?: number | "all", since?: ISO, until?: ISO, maxLines, maxBytes}`.
- It opens the existing WS route with `follow=false&format=json&timestamps=true` and maps frames
  through `mapLog`.
- It calls `sink(lines)` per frame and resolves with
  `{reason: "end" | "cap-lines" | "cap-bytes" | "aborted" | "unavailable", lines, bytes}`:
  - `unavailable`: the capability is not available, or the socket fails or closes before it opens.
    One generic notice covers both, because the transport has no stable reason; the web never
    parses log text for authorisation.
  - A log source failure (missing container, an `until` the upstream rejects) arrives as an ordinary
    error-level line followed by the end. It is shown as that line.
- It closes the socket on cap, abort or end. It never retries; the caller offers a retry.
- Lines whose timestamp lies outside `[since, until]` are dropped on the client, and the read
  continues to the end or a cap. This keeps the invariant that no line outside the window is shown;
  it is not compatibility logic. Agent updates are mandatory, so every agent applies `until`
  itself.
- Caps are reached when the count equals the limit. A frame that crosses the limit is cut at the
  limit, counted in UTF-8 bytes.
- Abort and sign-out clear any queued frames.
- It uses the same capability (`subscribeLogs` / `subscribeContainerLogs`), admission and sign-out
  reset as the live channel. A sign-out aborts every open read.
- The demo adapter implements it from its fixture lines.

`maxBytes` counts the UTF-8 length of `message`; line caps count mapped lines.

## S1 (#692) Health and Logs tabs: web only, yellow
- **Facts.** `HealthDetails` gets a new `condition` prop from both consumers (the stack Health tab
  and the container page), carrying the service's or container's existing `RuntimeCondition`.
  `runtime-conditions.ts` omits zero exit codes and zero restarts. A present condition therefore
  shows a missing restart count as 0. Stop reasons the operator caused (`intent`, `operator-stop`,
  `paused`) are shown as such, never as a crash. For
  the reasons `oom`, `restart-loop` and `exit`, and whenever an exit code is non-zero or
  `restarts10m > 0`, it shows:
  - the reason as catalogue text;
  - the exit code;
  - the restarts in the last 10 minutes.
- **Latest failing probe.** When the container is unhealthy or `failingStreak > 0`, the newest probe
  with a non-zero exit code is shown outside the collapsible "Raw data" section, with time, exit
  code and output. The raw section keeps the full list.
- **Last lines.** For a container that is not running (exited, dead, restarting, or a `down`
  condition), `readLogs` runs with `tail: 20` and caps of 20 lines / 64 KiB. Under the lines is a
  link to the Logs tab: the stack Logs tab with `service=`, or the container page log card.
  Running containers start no read.
- **Repeat collapse.** In both log views, consecutive shown lines with the same service and the same
  message (timestamp ignored) render as one row with `×N`. The row has an expand button; expanded, it
  shows the single lines. Grouping runs after the filters. Gap and delivery lines never group.
- **Error jump.** Previous and next buttons, plus the keys `[` and `]` in the log region, move the
  active row to the previous or next `error`/`warn` row in the shown lines. A jump also turns follow
  off. The keys join `use-log-keyboard`; an existing binding wins on conflict and is listed in the
  hint.
- **Density strip.** Above the log is one strip per service: in the stack tab, per service with at
  least one line; on the container card, one strip.
  - The loaded window `[first ts, last ts]` is split into 60 bins, and each bin shows its error and
    warning counts.
  - Bins with counts are buttons labelled with their count and time span. Selecting one scrolls to
    the first shown line in that bin and turns follow off.
  - Empty bins are inert.
  - Hovering or focusing a bin shows its local time span and counts above it (K-6).
  - The S2 combined view reuses the strip.

Break map S1:

| # | Row kind | Case | Decision | Test | Red mutation |
|---|---|---|---|---|---|
| 1 | external data entry | wire level `info` (real producer) with JSON, logfmt, bracket and misread messages | classifier order above | `level.test.ts` table incl. the three misreads, plus `mapLog` over real producer frames | drop the structured step → misread fixture fails; trust wire `info` → JSON error stays info |
| 2 | external data entry | wire `warn`/`error`/`debug` (demo, source failure) | kept | `log-mapping` test | always classify → a source failure line without error words becomes `info` |
| 3 | late response / identity | Health read for container A answers after the service shows container B (recreate) | read keyed by container id; aborted on unmount or id change | component test with held response | remove the abort → B shows A's lines |
| 4 | partial state | read ends on the byte cap or `unavailable` | byte cap: a "cut off" note; `unavailable`: a notice and a retry. The 20-line cap is the normal end of a tail-20 read and shows nothing | consumer render test | ignore `reason` → no note |
| 5 | consumer writes shared read result | collapse or density mutating the shared `published` array | pure derivations over a copy | test that freezes `published` | in-place sort → throws |

## S2 (#693) Disruption timeline: hub + web, red
### Contract (owner: S2 backend commit, before the S2 web commit)
- **New hub-only route.** `GET /environments/{id}/history/containers?range=24h|7d|30d`, operation
  `get-host-container-history`:
  - Not registered in agent mode, and absent from the agent and Edge command allowlists. The hub
    keeps it local: `isHistoryManagementPath` (`backend/internal/middleware/history.go`) gains the
    exact `GET /history/containers` suffix. Without it, a request for a Direct or Edge environment
    is forwarded to an agent that does not have the route.
  - Permission `containers:read` on the environment, the same as `get-container-history`.
  - Access rule `{Kind: Admin}`: simple users get `access.admin-required`. This is the maintainer
    decision of 2026-11-28 and narrows #693's "lanes follow the existing history visibility" to
    administrators. Assigned stacks keep their Health and Logs tabs.
- **Response.** `HostContainerHistory {range, hostId, containers: [{name, buckets: [{start,
  state, minutes, exitCode}]}]}`:
  - One SQL read: `kind='container' AND host_id=? AND tier=? AND bucket_start BETWEEN ? AND ?`,
    grouped by entity key in Go.
  - It returns only containers with at least one `down` or `unhealthy` minute in the range, aligned to
    the same fixed bucket series as `readHistory`; empty buckets are `nodata`.
  - It carries no resource metrics.
  - Containers are sorted by name.
  - At most 500 containers per host, by most disrupted minutes; `truncated: true` marks a cut.
- **New code.** None: existing `history.invalid-range` and `history.unavailable`.
- **Wire types.** They go in `types/history`. OpenAPI is regenerated and DTOs are generated. Contract
  version: no agent contract bump, because the route is hub-only.
- **`until`.** It is added to `logStreamParams` and to the stream key, and passed to Docker:
  - container: `ContainerLogsOptions.Until`;
  - project: compose `LogOptions.Until`.

  The value is passed through like `since`, and Docker rejects invalid values with the existing stream
  failure line. The streamer function type gains `until`, which touches
  `backend/internal/container/service_streams.go`, `backend/internal/project/service_workspace.go`,
  `projectdetails.Logs`, `backend/pkg/projects/cmds.go` and their test fakes. Edge forwards the raw
  query unchanged (`edge/ws_proxy.go`), and Direct proxies the raw query as well.
- **Finite reads are lossless.** A non-following read (`follow=false`) is served on its own
  connection, not through the shared live hub. The live hub drops a slow viewer to protect the
  others. A finite read writes every record in order, and a slow reader slows the Docker read
  instead. The read ends with a normal close after the last record. The candidate run on b80bae99
  showed the loss: 128 of 50,000 lines, then close 1006.
- **Agent versions.** Agent updates are mandatory (maintainer, 2026-11-28), so no older-agent path
  is designed or tested. The web filter in `readLogs` only keeps the window invariant.

### Web
- **Page and entry points.** A lazily loaded route `/disruptions`, in the admin shell only. It is
  linked from the dashboard problem area and from the Activity Center. The sidebar also gets its own
  entry, which shows only while a personal setting is on:
  - The setting is a switch under Settings → Appearance, "Show Disruptions in the navigation". It
    defaults to off and is visible to administrators only.
  - It is stored as `disruptions: boolean` in the existing synced `sidebar` preference section.
    `decodeSidebar` treats a missing or invalid value as off. The section is free-form on the hub,
    so the hub does not change.
  - This is the maintainer decision of 2026-11-28.
- **Data.** The range is 24 h / 7 d / 30 d. There is one query per enabled host in the inventory.
  Hosts are sections; a failed or forbidden host shows an inline notice and does not hide the others.
- **Lane.** A lane is one row of buckets per container. `down` buckets are red, `unhealthy` buckets
  are amber, and the rest are neutral. The container name links to the container when the inventory
  knows it.
- **Window selection.** Each disrupted bucket is a button. Selecting it opens the combined view for
  the window `[bucket start − 5 min, bucket end + 5 min]`.
- **Combined view.**
  - Participants are every lane, on any host, with `down` or `unhealthy` minutes in a bucket that
    overlaps the window.
  - A participant's log id is the inventory id (`host~raw`, built with the existing
    inventory-mapping helper) of the host's current container with that name. A lane without a
    current container starts no read and says the container no longer exists. A read is labelled
    with the id it was started for, so a container recreated mid-read never takes over the old
    result.
  - Each participant gets `readLogs({kind: "container", id}, {since, until, tail: "all",
    maxLines: 2000, maxBytes: 1 MiB})`, with at most 20 participants, by disrupted minutes.
  - Lines are merged by timestamp, with the container and host as prefix, and use the S1 density
    strip and error jump.
  - A participant whose read ends `unavailable` shows "Logs not available for this container".
    That covers missing log permission; the transport has no stable reason for it. Its lane and the
    other participants stay.
  - A participant with no lines says that the log may predate a container recreation, which Docker
    removes.

Break map S2:

| # | Row kind | Case | Decision | Entry boundary | Test (real transport?) | Red mutation |
|---|---|---|---|---|---|---|
| 1 | authorisation | simple user calls the route | `access.admin-required` | access rules table | Go access test over the real router | rule `AppList` → 200 |
| 2 | authorisation | API key with `containers:read`, no `containers:logs` | lanes 200, log WS 403 | permission middleware | Go route test + web participant notice test | require `containers:logs` on the route → 403 |
| 3 | agent surface | route in agent mode | not registered | `RegisterRoutes(agent=true)` | agent-surface test lists the route as absent | register before `if agent` → test fails |
| 4 | transport parity | `until` via Direct and Edge, container and project | raw query forwarded; Docker gets `Until` | `parseLogStreamParamsInternal` → service → SDK options | Go tests: handler passes `until` to the streamer; container and compose paths put `until` into the Docker logs query (existing transport/compose test fakes); Direct and Edge raw-query forwarding tests | drop `until` at any hop → assertion fails |
| 5 | identity change | stream key without `until` shares a hub between two windows | `until` is part of the key | `buildLogStreamKeyInternal` | Go test: two keys differ | remove from key → test fails |
| 6 | external data entry | a frame outside the window (boundary second, out-of-order arrival) | the client drops it; the read continues to the end | `readLogs` | web test: an out-of-window line between in-window lines is dropped, and later in-window lines are kept | remove the filter → extra line shown |
| 7 | external data entry | range invalid, host unknown, zero rows, 500+ containers | 400 `history.invalid-range`; empty list; truncation | handler | Go table test | drop the cap → more than 500 returned |
| 8 | late response | range switched while a host query is in flight | query key includes range; stale data never shown under the new range | react-query key | component test with held response | key without range → wrong range shown |

## S3 (#694) Pattern mode: web only, yellow
- **Mode switch.** `Raw | Patterns` is a toggle in the stack Logs tab and the container log card.
- **Reading.** Selecting Patterns starts `readLogs` with:
  - window 1 h / 24 h / 7 d (default 24 h) as `since`, or a custom range from the browser's date
    and time inputs as `since` and `until` (K-6);
  - `tail: 100000`, so Docker returns the newest lines inside the window. For a stack, compose applies
    `tail` per container, so a stack read shows the time span it actually covered. It does not claim
    to hold the globally newest lines;
  - `maxLines: 100000` and `maxBytes: 20 MiB`.

  Leaving the mode or the page aborts the read.
- **Folding.**
  - Continuation lines are lines that start with whitespace, `at `, `Caused by`, `Traceback` or
    `File "`. One counts as part of the last head line of the same source: the service in a stack,
    or the container. It never becomes a pattern of its own. State is kept per source, because
    stack logs interleave services.
  - The pattern key is built from the head line only. Raw-mode pattern filtering applies the same
    `patternKey(line)` to the live lines, so both modes match the same way. A continuation line
    counts towards the pattern's lines but is not shown as a match in Raw.
  - ANSI codes are stripped. Timestamps, UUIDs, hex ids of 8 or more characters, IPv4/IPv6
    addresses, numbers and quoted strings are normalised to placeholders. The normalised text is
    the pattern key.
  - Each pattern keeps level (`classifyLevel` on the first sample), count, first and last time,
    30 time bins and up to three samples.
  - Raw lines are not kept.
- **Chunks.** Frames are queued and folded in slices of at most 8 ms per animation frame. Progress
  shows lines, size and the local time span covered, with a spinner while reading. Pattern rows do
  not repeat the span (K-6).
- **Caps.** At most 5,000 distinct patterns, plus one "other shapes" row that counts every further
  new shape (count conservation is tested). The queue of frames waiting to be folded is bounded by
  the same byte cap, and abort or sign-out clears it.
  The list shows the top 200 by count, with an error/warn filter. With more than 2,000 patterns, a
  notice says the log does not group well. Prototype logs that did not group well had about 3,500 and
  6,800 patterns; logs that did had between 31 and 1,145. Reaching the line or byte cap is shown
  together with the covered time span.
- **Selecting a pattern.** It switches to Raw with a pattern filter. The filter keeps live lines
  whose key matches. When none of the loaded live lines match, the pattern's samples are shown with
  a note.
- **Time bins (K-6).** Each pattern's 30 bins split the covered span evenly. Hovering a bin shows its
  span and line count. Selecting a bin with lines reads that bin's span (`since`, `until`,
  `tail: all`) in pages of 500 lines / 1 MiB from its start (K-7) and lists those lines below the pattern,
  with the pattern's own lines marked and the first one scrolled into view. Raw keeps only live
  lines, so the bin read is the way to reach older log positions.
  - Bounds are the integer milliseconds the fold counted in the bin. The browser keeps millisecond
    timestamps while Docker filters producer nanoseconds, so `until` ends at the last nanosecond of
    the bin's last millisecond.
  - Marking follows the fold: matching head lines plus the continuation lines after them in the same
    source. Continuation lines before their source's first head in the read stay unmarked.
  - A full page states how many lines are shown and offers "Load more lines". The next page reads
    the bin from its start again, because a stack merges its containers concurrently and no
    timestamp cursor is safe. Its budget is the previous read's lines and bytes plus one page
    (500 lines / 1 MiB), and the lines already shown are taken out as a multiset (source, time,
    text), so each page adds at least one page of new lines or ends the bin. A page that adds no
    line offers no further page. The list is sorted by time; the sort is stable, so each source
    keeps its own order.
  - Long lines stay on one row; the list and the samples scroll sideways (K-7).
- **Measurement.** Fold time for 100,000 synthetic lines: desktop Chromium unthrottled, and Chromium
  with 4× CPU throttle and a phone viewport. Both are documented with the longest main-thread task.

Break map S3:

| # | Row kind | Case | Decision | Test | Red mutation |
|---|---|---|---|---|---|
| 1 | external data entry | stack trace lines, ANSI, ids, IPs | normalisation rules | `patterns.test.ts` fixtures with exact counts | skip continuation join → counts differ |
| 2 | external data entry | highly variable log | notice above 2,000 patterns, cap at 5,000 | test with 6,000 unique shapes | remove the cap → more than 5,000 rows |
| 3 | partial state | cap reached mid-frame | stop folding at the cap; show reason and covered span | `readLogs` + fold test | ignore cap → 100,001 counted |
| 4 | lifetime | leave the mode during a read | abort the socket; drop the queued frames | component test asserting socket closed | no abort → socket stays open |

Additional rows from the design review:

| # | Row kind | Case | Test |
|---|---|---|---|
| S2.9 | hub ownership | Direct and Edge environment read of `/history/containers` | real middleware chain test: answered by the hub, never forwarded; red when the classifier entry is removed |
| S2.10 | persisted data | buckets written before M47, read after hub restart | Go test that reads rows inserted directly; no migration |
| S2.11 | identity change | container recreated while the combined read runs | web test with held read: result stays with the old id |
| S1.6 | lifetime | sign-out during capability admission or a running read | web test: no socket after reset; queued frames dropped |
| S3.5 | lifetime | queued folding after leaving the mode | web test: no state update after unmount |

## Acceptance cells (sandbox H, D, E)
See the M47 kickoff matrix (closing issue #695).

## Rejected alternatives
- Event table as the timeline source: 500 entries in total, about one day on a real hub.
- Server-side pattern computation: it needs a new agent endpoint and protocol, for data the browser
  can fold within the cap.
- A sidebar entry shown by default: the page is reached from problems, not as a daily view. The
  entry exists, but only behind a personal switch that defaults to off (K-2).
