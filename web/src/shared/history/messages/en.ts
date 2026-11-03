// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original English runtime history messages for issues #335 and #336.

// American English texts of the runtime history sections on container and stack pages.

export const enHistory = {
  availability: "Availability",
  resources: "Resources",
  rangeLabel: "Time range",
  ranges: { r24h: "24 h", r7d: "7 days", r30d: "30 days" },
  sameRange: "Same time range",
  uptime: "Uptime",
  noData: "No data yet",
  now: "now",
  stripLabel: "Availability per time slot, oldest first",
  states: { running: "Running", unhealthy: "Unhealthy", down: "Down", stopped: "Stopped", nodata: "No data" },
  minutes: "{state}: {minutes} min",
  stoppedNote: "Stopped on purpose – not counted as downtime",
  exitCode: "Exit code {code}",
  partial: "Still being recorded",
  cpu: "CPU",
  memory: "Memory",
  cpuSummary: "Avg {avg} · peak {max} · now {now}",
  memSummary: "Avg {avg} · peak {max} · limit {limit}",
  memSummaryNoLimit: "Avg {avg} · peak {max}",
  noValues: "No values",
  bucketCpu: "CPU avg {avg}, peak {max}",
  bucketMem: "Memory avg {avg}, peak {max}",
  totals: { netRx: "Received", netTx: "Sent", diskRead: "Disk read", diskWrite: "Disk written" },
};
