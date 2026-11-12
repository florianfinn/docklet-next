// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original phase 2a stack update consumer for issue #231. M21 #438: candidate checks and verified updates.

export interface UpdateBaseline {
  targetIdentity: string; savedRevision: string; observedRevision: string;
  available: boolean; autoExcluded: boolean; code?: string; sourceRunId?: string;
}
export interface LifecycleInput { expectedRevision: string; pointId?: string; replaceConfiguration?: boolean; services?: string[]; service?: string; expectedManifest?: string }
export interface UpdatePoint {
  id: string; registrationId: string; targetId: string; targetIdentity: string;
  sourceRunId: string; createdAt: string; sourceState: "settled" | "partial-source";
  available: boolean; orphaned: boolean; volumeDataIncluded: false; code?: string;
  protectionReason: "restore-target" | "healthy-baseline" | null;
  holdState: "none" | "active" | "unknown"; failedAttempt: boolean;
  images: { service: string; reference: string; imageId: string; digest?: string; present: boolean; running: boolean }[];
}
export type PolicyMode = "check" | "scheduled";
export interface StackUpdatePolicy {
  mode: PolicyMode; revision: number; environmentId: string; registrationId: string;
  targetKind: "stack" | "container"; targetName: string; targetMissing: boolean; window?: UpdateWindow; targetId: string; targetIdentity: string; jobId: string;
  stale: boolean; checkedAt?: string; code?: string; applyCheckedAt?: string; applyCode?: string;
  bookkeepingAt?: string; bookkeepingCode?: string;
  candidates: { service: string; imageRef: string; version: string; digest: string; platform: string; entryId?: string;
    notificationState: "pending" | "sending" | "sent" | "unconfigured" | "failed" | "not-requested" }[];
}
export interface UpdateWindow { kind: "daily" | "weekly"; day?: number; time: string }
export type UpdateRunStatus = "queued" | "waiting" | "running" | "retrying" | "succeeded" | "partial" | "skipped" | "failed" | "needs_attention" | "canceled";
export interface UpdateJob {
  id: string; kind: string; enabled: boolean; schedule: string; timeZone?: string; nextRun?: string; code?: string;
  currentRun?: { id: string; status: UpdateRunStatus }; lastRun?: { id: string; status: UpdateRunStatus }; lastSuccess?: string;
}
export interface SystemHints {
  hubVersion: string;
  images: { kind: "agent" | "tools"; reference: string; currentVersion?: string; candidateVersion?: string;
    compatibleReference?: string; state: "current" | "available" | "pinned" | "unknown";
    requiresHubUpdate: boolean; manualOnly: true; code?: string }[];
}
export interface UpdatesApi {
  getUpdateBaseline?(stackId: string): Promise<UpdateBaseline>;
  listHostRestorePoints?(hostId: string): Promise<UpdatePoint[]>;
  listRestorePoints?(stackId: string): Promise<UpdatePoint[]>;
  deleteRestorePoint?(stackId: string, pointId: string): Promise<void>;
  deleteHostRestorePoint?(hostId: string, pointId: string): Promise<void>;
  getAutoUpdatePolicy?(stackId: string): Promise<StackUpdatePolicy>;
  saveAutoUpdatePolicy?(stackId: string, mode: PolicyMode, expectedRevision: number, window?: UpdateWindow): Promise<StackUpdatePolicy>;
  listUpdatePolicies?(hostId: string): Promise<StackUpdatePolicy[]>;
  getContainerUpdatePolicy?(id: string): Promise<StackUpdatePolicy>;
  saveContainerUpdatePolicy?(id: string, mode: PolicyMode, expectedRevision: number, window?: UpdateWindow): Promise<StackUpdatePolicy>;
  getUpdateSettings?(): Promise<import("./settings").UpdateSettings & { timeZone: string; nextRun?: string; projectionUnavailable?: boolean }>;
  checkAllUpdates?(): Promise<string>;
  restoreContainerImage?(id: string): Promise<string>;
  getContainerPreviousImage?(id: string): Promise<{ imageId: string; reference: string; createdAt: string } | undefined>;
  getSystemImageHints?(hostId: string): Promise<SystemHints>;
  checkStackUpdates?(stackId: string): Promise<void>;
}
