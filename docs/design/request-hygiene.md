<!--
Source: docklet-next/issues/389
Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
Retrieved: 2026-11-11
License: Apache-2.0
Deviations: Original M23 decision for browser request hygiene: session discovery, hub maintenance holds and unavailable-host skips (#425, #389, #391, #398).
-->

# Request hygiene

The web client sends each browser request through one chain: CSRF client, identity
reads, host admission (five concurrent reads per HTTP/1.1 host origin) and a
**dispatch gate** directly before the network. This note records what the gate and
its neighbours decide. Rationale: one place sees every request, so maintenance and
offline hosts are handled once instead of per page.

## Session discovery (#425)

The hub sets a non-secret presence cookie `docklet_session=1` (`__Host-` prefixed
when served securely) together with the HttpOnly session cookie, renews and clears
it with that cookie. In local login mode the web skips the anonymous `/auth/me`
probe when the hint is absent. Current user and auth mode stay fresh for 60 s.
There is no new auth route.

## Hub maintenance hold (#391)

A hub backup or restore answers ordinary requests with 503 `backup.maintenance`.

- The first such answer, or an active state from `/api/backups/maintenance`, starts
  a **hold**. Replay-safe reads (GET/HEAD and read projections) wait in the gate
  instead of failing; at most 64 waiters are counted, more get the maintenance
  answer at once. Writes are never held or replayed.
- A held read is replayed at most twice after the hold ends. After 120 s a hold is
  overdue: requests proceed and surface the hub's answer.
- Every maintenance answer leaving the gate has one shape: status 503, `code`,
  `data.code` and `detail` set to `backup.maintenance`, plus the header
  `X-Docklet-Maintenance: hub`. Every decoder keeps that code (the marker lets
  status-only readers recognise it without reading the body).
- **Time spent in a hold never counts against a request timer.** One pause clock
  per browser excludes held time from read deadlines, request abort timers, the
  run lineage and admission's held-host mark.
- The query layer retries only `backup.maintenance` errors, every 4 s up to 30
  times. During those retries a query keeps its cached data and success state, so
  no page replaces content with an error; inventory, capability, schedule and
  settings reads propagate the outcome instead of turning it into empty rows or
  `http.error`. Readers that keep per-host rows (security, volume backups) flag
  them as unavailable instead.
- The shell header shows a compact maintenance status next to the live indicator.
  When maintenance ends, every topic refreshes once (a restore may replace data).
- The native event stream and hub backup downloads wait for the hold's end without
  taking a waiter slot; a download re-checks the identity after the wait.
- A maintenance answer of a request cancelled by sign-in or sign-out never starts
  a hold for the next identity.

## Unavailable-host skips (#389, #398)

Automatic reads carry the internal header `X-Docklet-Read` with `background`
(first load, mount, invalidation, recovery) or `poll` (interval refresh); the gate
removes it before sending. For a host that the catalogue or the environment
stream reports offline, disabled, pending, update-required or token-rejected, the
gate answers tagged reads without a request: status 502, `code: agent.unavailable`,
`data.reason: host.skipped`, header `X-Docklet-Skipped: host`. Readers keep the
host's previous rows as new objects marked unavailable, with the catalogue's
refusal as the visible reason. Standby Edge hosts skip only `poll` reads.

First loads still read, because the hub serves retained snapshots of offline
hosts. Deliberate user reads (detail pages, dialogs) are never tagged.

## Limits

- Uploads are single sends and are not held.
- A hold does not survive a page reload; the next status read re-establishes it.
