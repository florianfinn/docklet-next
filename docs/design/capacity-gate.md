<!--
Source: docklet-next/issues/431
Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
Retrieved: 2026-11-11
License: Apache-2.0
Deviations: Original M23 decision for the capacity widget source and time axis, run gate confirmation and fleet density (#400, #401, #431, #430).
-->

# Capacity, run gate confirmation and density

## One disk source (#401)

Live host metrics and the history sampler measure the same disk. When the
configured disk path fails or reports a total of 0, both measure `/` instead; the
history bucket records `diskSource` `/`. The sampler logs the failing configured
path once per host and process, and again only after a success in between. If `/`
fails too, the sample carries no disk values and the card marks the meter
unavailable.

The capacity trend and growth use only buckets measured at the current source,
including an earlier period with the same source. The card shows "Measured at
{path}" from the same response. A switch becomes visible within one sample minute
on the next capacity read: a Dashboard visit refetches after the app's 30 s stale
time, and the card polls every 15 minutes.

## Time axis (#400)

The capacity line spans the 30-day bucket domain (first bucket start to last bucket
end) and places each kept 8-hour bucket by its time. Where buckets are missing or
were measured at another source, the line breaks into separate segments.

## Run gate confirmation (#431)

The run gate refuses a mutation while a conflicting run on the same target is
active. A local run row can lag behind the hub when an event was missed. Before
refusing, the gate reads the blocking run's hub record once
(`GET /environments/{host}/runtime-runs/{runId}`, without `refresh=true` and
without following it), then repeats the complete admission. The command is sent
only when that record is terminal with a known outcome. A running, queued or
unknown-outcome record, a failed read, or an answer for another run or host
refuses as before; a new blocker that appears meanwhile gets its own single
confirmation. Concurrent attempts share one read per blocker; an identity reset
during the read cancels the attempt. Adapters without the confirmation port (demo)
refuse at once.

## Fleet density (#430)

Docklet is designed and tested for about 50 containers per host. 100 containers
per host is the documented upper bound; it is not enforced. The supported values
are listed in [Compatibility](../compatibility.md). Measurements at the bound are
release evidence, not budgets: the Dashboard budgets stay defined for the
reference fleet.
