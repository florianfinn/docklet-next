// Source: docklet-next/issues/636
// Revision: 7a44b4b5c32ccc64b68e027a5496fab1d26e90e2
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 stack Health tab and reusable health details. M42 #637: container section and hover link labels. M42 #637: shared hint text and one-sentence probe line.

export const enHealth = {
  latestFailure: "Latest failing probe", exitCode: "Exit code: {code}", restarts: "Restarts in 10 minutes: {count}",
  lastLines: "Last log lines", logsUnavailable: "Logs not available for this container", logsCutOff: "Long lines were cut off after 64 KiB.", logsInterrupted: "Reading the logs was interrupted; lines may be missing.", retry: "Retry", openLogs: "Open in Logs tab",
  conditionReasons: { oom: "Killed due to insufficient memory (OOM).", "restart-loop": "Container keeps restarting.", exit: "Container exited.", health: "Healthcheck failed.", paused: "Container paused by the operator.", intent: "Container stopped as intended.", "never-started": "Container has never started.", "operator-stop": "Container stopped by the operator." },
  title: "Health", showHealth: "Show health",
  noContainer: "No container",
  unavailable: "Health details unavailable",
  historyUnavailable: "Availability unavailable",
  raw: "Raw health data",
  loading: "Loading health details…",
  streak: "Failing streak: {count}",
  command: "Probe command", interval: "Interval", timeout: "Timeout", retries: "Retries", startPeriod: "Start period",
  default: "default", seconds: "{value} s", minutes: "{value} min", hours: "{value} h",
  probeLine: "Start: {time} · Exit code {code}", noProbes: "No probes yet",
  reasons: {
    "not-running": "Container is not running.",
    "no-healthcheck": "No healthcheck defined.",
    disabled: "Healthcheck disabled.",
    pending: "Waiting for the first probe.",
    "pending-overdue": "First probe is probably overdue.",
    timeout: "The probe exceeded its timeout.",
    "tool-missing": "The probe tool \"{tool}\" is missing in the image.",
    "http-status": "The probe got HTTP {status}.",
    http404: "The probe got HTTP 404 – the checked path does not exist.",
    "connection-refused": "The probe connection was refused.",
    failing: "The probe failed; see raw output.",
  },
};
