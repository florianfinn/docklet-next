// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original disruption timeline and bounded combined log window for #693.

export const enDisruptions = {
  "title": "Disruptions",
  "description": "Overlapping container problems across hosts. Select a bucket to read logs for that window.",
  "range": "Time range",
  "loading": "Loading…",
  "combined": "Combined logs",
  "missing": "Container no longer exists",
  "unavailable": "Logs not available for this container",
  "emptyLogs": "No lines in this window. The log may predate a container recreation.",
  "capped": "Logs cut off at the read limit.",
  "retry": "Read again",
  "truncated": "Only the first 500 affected containers are shown.",
  "empty": "No disruptions in this range.",
  "omitted": "{count} further participants omitted (limit: 20).",
  "bucket": "{container}: {state}, {start} – {end}",
  "ranges": {
    "24h": "24 h",
    "7d": "7 d",
    "30d": "30 d"
  },
  "states": {
    "down": "Down",
    "unhealthy": "Unhealthy",
    "running": "Running",
    "stopped": "Stopped",
    "nodata": "No data"
  }
};
