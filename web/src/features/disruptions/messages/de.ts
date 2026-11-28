// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original disruption timeline and bounded combined log window for #693.

export const deDisruptions = {
  "title": "Störungen",
  "description": "Gleichzeitige Containerprobleme auf mehreren Hosts. Einen Zeitabschnitt auswählen, um passende Logs zu lesen.",
  "range": "Zeitraum",
  "loading": "Wird geladen…",
  "combined": "Gemeinsame Logs",
  "missing": "Container existiert nicht mehr",
  "unavailable": "Logs für diesen Container nicht verfügbar",
  "emptyLogs": "Keine Zeilen in diesem Zeitfenster. Das Log könnte vor einer Container-Neuerstellung liegen.",
  "capped": "Logs am Leselimit abgeschnitten.",
  "retry": "Erneut lesen",
  "truncated": "Nur die ersten 500 betroffenen Container werden angezeigt.",
  "empty": "Keine Störungen in diesem Zeitraum.",
  "omitted": "{count} weitere Teilnehmer ausgelassen (Limit: 20).",
  "bucket": "{container}: {state}, {start} – {end}",
  "ranges": {
    "24h": "24 Std.",
    "7d": "7 Tage",
    "30d": "30 Tage"
  },
  "states": {
    "down": "Ausgefallen",
    "unhealthy": "Ungesund",
    "running": "Läuft",
    "stopped": "Gestoppt",
    "nodata": "Keine Daten"
  }
};
