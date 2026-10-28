// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import type { ScanSchedule } from "./scan-schedule";

export interface ScanSettingsInput {
  enabled: boolean;
  automatic: boolean;
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
