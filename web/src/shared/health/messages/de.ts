// Source: docklet-next/issues/636
// Revision: 7a44b4b5c32ccc64b68e027a5496fab1d26e90e2
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 stack Health tab and reusable health details. M42 #637: container section and hover link labels. M42 #637: shared hint text and one-sentence probe line.

export const deHealth = {
  title: "Health", showHealth: "Health anzeigen",
  noContainer: "Kein Container",
  unavailable: "Health-Details nicht verfügbar",
  historyUnavailable: "Verfügbarkeit nicht verfügbar",
  raw: "Health-Rohdaten",
  loading: "Health-Details werden geladen…",
  streak: "Fehlversuche in Folge: {count}",
  command: "Prüfbefehl", interval: "Intervall", timeout: "Zeitlimit", retries: "Wiederholungen", startPeriod: "Startphase",
  default: "Standard", seconds: "{value} s", minutes: "{value} Min.", hours: "{value} Std.",
  probeLine: "Start: {time} · Exit-Code {code}", noProbes: "Noch keine Prüfungen",
  reasons: {
    "not-running": "Container läuft nicht.",
    "no-healthcheck": "Kein Healthcheck definiert.",
    disabled: "Healthcheck deaktiviert.",
    pending: "Warten auf die erste Prüfung.",
    "pending-overdue": "Die erste Prüfung ist vermutlich überfällig.",
    timeout: "Die Prüfung hat ihr Zeitlimit überschritten.",
    "tool-missing": "Das Prüfwerkzeug \"{tool}\" fehlt im Image.",
    "http-status": "Die Prüfung erhielt HTTP {status}.",
    http404: "Die Prüfung erhielt HTTP 404 – der geprüfte Pfad existiert nicht.",
    "connection-refused": "Die Verbindung der Prüfung wurde abgelehnt.",
    failing: "Die Prüfung ist fehlgeschlagen; siehe Rohausgabe.",
  },
};
