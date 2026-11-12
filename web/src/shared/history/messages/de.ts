// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original German runtime history messages for issues #335 and #336.

// German texts of the runtime history sections on container and stack pages.
import type { enHistory } from "./en";

export const deHistory: typeof enHistory = {
  availability: "Verfügbarkeit",
  resources: "Ressourcen",
  rangeLabel: "Zeitraum",
  ranges: { r24h: "24 Std.", r7d: "7 Tage", r30d: "30 Tage" },
  sameRange: "Gleicher Zeitraum",
  uptime: "Verfügbarkeit",
  noData: "Noch keine Daten",
  now: "jetzt",
  stripLabel: "Verfügbarkeit je Zeitabschnitt, älteste zuerst",
  states: { running: "Läuft", unhealthy: "Ungesund", down: "Ausgefallen", stopped: "Gestoppt", nodata: "Keine Daten" },
  minutes: "{state}: {minutes} Min.",
  stoppedNote: "Bewusst gestoppt – zählt nicht als Ausfall",
  exitCode: "Exit-Code {code}",
  partial: "Wird noch aufgezeichnet",
  cpu: "CPU",
  memory: "Arbeitsspeicher",
  cpuSummary: "Ø {avg} · Spitze {max} · jetzt {now}",
  memSummary: "Ø {avg} · Spitze {max} · Limit {limit}",
  memSummaryNoLimit: "Ø {avg} · Spitze {max}",
  noValues: "Keine Werte",
  bucketCpu: "CPU Ø {avg}, Spitze {max}",
  bucketMem: "Arbeitsspeicher Ø {avg}, Spitze {max}",
  totals: { netRx: "Empfangen", netTx: "Gesendet", diskRead: "Datenträger gelesen", diskWrite: "Datenträger geschrieben" },
};
