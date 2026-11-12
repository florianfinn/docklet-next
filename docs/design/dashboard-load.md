<!--
Source: docklet-next/issues/435
Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
Retrieved: 2026-11-11
License: Apache-2.0
Deviations: Original M23 decision for Dashboard load: bounded events, active vulnerability counts, inventory scope retention, metrics lifetime and readiness (#435, #428).
-->

# Dashboard load

The Dashboard must be usable quickly on the reference fleet (three hosts, Direct and
Edge, each with workloads and scanned images) and must not issue more requests
than its widgets need. A measured baseline showed the security image catalogue and
full findings download as the largest cost, followed by request scheduling.

## Events window

- The Dashboard reads one page: events of the last 24 hours, newest first, at most
  500 (`/events?since=…&limit=500`). The hub applies the cap per request.
- When more events exist, Timeline and Events say "500+ events in the last 24
  hours" and link to the Activity Center, which keeps the full history. The notice
  also shows when the shown hosts have no rows in the window.
- One owner per connection issues the read. It waits at most 1 s for the stream's
  initial event frame: a read after that frame already contains what the frame
  signals. Later frames, sequence gaps and unreadable envelopes refresh once
  (coalesced). Hub maintenance end refreshes through the same owner, so a snapshot
  from before a restore is not reused. A load waiting for the frame is cancelled
  by an identity reset.
- The Timeline draws at most one marker per 15-minute slot and lane, with a count
  and the slot's newest entries in its tooltip.

## Security counts

The Dashboard reads active vulnerability counts per host in one request
(`POST /environments/{id}/images/vulnerabilities/active-summaries`) instead of
image catalogues, summaries and findings. Images are named by the containers that
use them (inventory image ids join the route's raw ids per host); an unused image
shows its short id. An image without a completed scan counts as not scanned. A
failed host keeps its last counts, marked unavailable, and a transient failure of
an online host schedules the 15 s recovery refetch.

## Inventory scope retention (#428)

The inventory's capability and host catalogue stage has its own 30 s budget. After
a successful load, a transient failure of that stage (timeout, network, 5xx,
`agent.unavailable`, a transient capability read) keeps the last catalogue of the
same principal and marks the result as stale scope; the inventory notice says the
catalogue could not be refreshed. Denial and session loss never retain; an
identity change discards; a first load without catalogue is an error. Hub
maintenance is not a scope failure: it propagates so the query keeps its rows.

Observations carry a visible `reason` (a persistent refusal such as
`agent.update-required` wins) and a separate `recovery` fact that schedules the
recovery refetch; every merge site keeps both.

## Metrics lifetime

The container CPU read shares one lifetime: the last unsubscribe, an identity
reset or a hidden document aborts it and stops its timer; becoming visible
refreshes once. Hiding the document also closes the host stats sockets; they
reopen when it is visible again. An online host whose stats socket fails, sends an
unreadable first message, or stays silent for 10 s before a first sample is
reported as metrics unavailable; a later sample clears that.

## Readiness markers

For measurement the page root carries `data-dashboard-core-ready` once hosts,
stacks and containers have their first complete results, and
`data-dashboard-settled` once the shown widgets are known (service summaries
answered) and every shown widget reported its first result or error. The load
widget needs a first sample or explicit metrics unavailability per online host. A
core inventory error settles with its error shown. Both markers stay set for the
page visit. Optional widgets read their data only while shown.
