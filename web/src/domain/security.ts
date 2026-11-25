// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262. M39 #612: durable host scan run summaries.

import type { ScanSchedule } from "./scan-schedule";
import type { SecurityCounts } from "./model";

export interface ScanSettingsInput {
  enabled: boolean;
  schedule: ScanSchedule;
  ignoreUnfixed: boolean;
  notifyCritical: boolean;
  notifyHigh: boolean;
  notifyFailed: boolean;
}
export interface ScanSettings extends ScanSettingsInput {
  timeZone: string;
  nextRun?: string;
  projectionUnavailable?: boolean;
}
export interface ScanAdmission { hostId: string; activityId?: string; busy?: boolean; code?: string }

/** An all-host command preserves each independent admission, including refusals. */
export class ScanAdmissionError extends Error {
  readonly admissions: ScanAdmission[];
  constructor(admissions: ScanAdmission[]) { super("vulnerability.scan-admission"); this.admissions = admissions; }
}

/** A terminal host scan run as the hub stores it; single-image scans have no such record. */
export interface ScanRunSummary {
  status: "completed" | "partial" | "failed" | "cancelled" | "interrupted";
  targets: number;
  scanned: number;
  failed: number;
  skipped: number;
  counts: SecurityCounts;
  startedAt: string;
  finishedAt?: string;
}
/** The newest terminal host scan runs by activity id; `unavailable` when the host cannot answer this read. */
export interface ScanRunSummaries { runs: ReadonlyMap<string, ScanRunSummary>; unavailable?: boolean }
