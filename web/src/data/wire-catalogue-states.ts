// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums. M21 #438: candidate checks and verified updates.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

export const BACKUP_STATUS_KEYS = {
  "running": "volumes.backups.status.running",
  "succeeded": "volumes.backups.status.succeeded",
  "failed": "volumes.backups.status.failed",
  "partial": "volumes.backups.status.partial",
  "unavailable": "volumes.backups.status.unavailable",
} satisfies Record<NonNullable<
  | Schema["DockerVolumeVolumeBackup"]["status"]
  | Schema["VolumeBackupEntry"]["status"]
  | Schema["VolumeRestoreRun"]["status"]
>, CatalogueKey>;

export const BACKUP_KIND_KEY_PREFIX = "settings.backups.kinds";

export const BACKUP_TARGET_KIND_KEYS = {
  "local": `${BACKUP_KIND_KEY_PREFIX}.local`,
  "mount": `${BACKUP_KIND_KEY_PREFIX}.mount`,
  "smb": `${BACKUP_KIND_KEY_PREFIX}.smb`,
  "nfs": `${BACKUP_KIND_KEY_PREFIX}.nfs`,
} satisfies Record<NonNullable<
  | Schema["BackupSyncedTarget"]["kind"]
  | Schema["BackupTarget"]["kind"]
  | Schema["BackupTargetInput"]["kind"]
  | Schema["VolumeUsableBackupTarget"]["kind"]
>, CatalogueKey>;

export const MAINTENANCE_PHASE_KEYS = {
  "draining": "hubBackups.maintenancePhase.draining",
  "applying": "hubBackups.maintenancePhase.applying",
  "verifying": "hubBackups.maintenancePhase.verifying",
  "blocked": "hubBackups.maintenancePhase.blocked",
} satisfies Record<NonNullable<
  | Schema["BackupSystemBackupMaintenance"]["phase"]
>, CatalogueKey>;

export const HUB_BACKUP_STATUS_KEYS = {
  "running": "hubBackups.statusValues.running",
  "succeeded": "hubBackups.statusValues.succeeded",
  "failed": "hubBackups.statusValues.failed",
} satisfies Record<NonNullable<
  | Schema["BackupSystemBackupRun"]["status"]
>, CatalogueKey>;


export const BACKUP_DELIVERY_STATE_KEYS = {
  "current": "settings.backups.delivered",
  "conflict": "settings.backups.deliveryConflict",
  "pending": "settings.backups.deliveryPending",
} satisfies Record<NonNullable<
  | Schema["BackupTargetHostDelivery"]["state"]
>, CatalogueKey>;

export const RESTART_STATE_KEYS = {
  "reserved": "hosts.restart.phases.reserved",
  "sent": "hosts.restart.phases.sent",
  "awaiting": "hosts.restart.phases.awaiting",
  "connected": "hosts.restart.phases.connected",
  "failed": "activity.status.failed",
  "unknown": "activity.runtime.unknown",
} satisfies Record<NonNullable<
  | Schema["EnvironmentAgentRestartRun"]["state"]
>, CatalogueKey>;

export const HISTORY_STATE_KEYS = {
  "running": "history.states.running",
  "unhealthy": "history.states.unhealthy",
  "down": "history.states.down",
  "stopped": "history.states.stopped",
  "nodata": "history.states.nodata",
} satisfies Record<NonNullable<
  | Schema["HistoryBucket"]["state"]
>, CatalogueKey>;

export const PRUNE_FAILURE_KEYS = {
  "volume.inspect-failed": "volumes.prune.failedReasons.inspect-failed",
  "volume.usage-failed": "volumes.prune.failedReasons.usage-failed",
  "volume.remove-failed": "volumes.prune.failedReasons.remove-failed",
  "volume.protected": "volumes.prune.failedReasons.protected",
} satisfies Record<NonNullable<
  | Schema["VolumePruneFailed"]["code"]
>, CatalogueKey>;

export const PRUNE_CANDIDATE_KEYS = {
  "unused": "volumes.prune.candidateReasons.unused",
  "unused-anonymous": "volumes.prune.candidateReasons.unused-anonymous",
} satisfies Record<NonNullable<
  | Schema["VolumePrunePlanCandidate"]["reason"]
>, CatalogueKey>;

export const PRUNE_SKIP_KEYS = {
  "in-use": "volumes.prune.skipReasons.in-use",
  "not-found": "volumes.prune.skipReasons.not-found",
  "changed": "volumes.prune.skipReasons.changed",
} satisfies Record<NonNullable<
  | Schema["VolumePruneSkipped"]["reason"]
>, CatalogueKey>;

export const SCAN_STATUS_KEYS = {
  "running": "security.scanning",
  "completed": "security.historyStatus.completed",
  "partial": "security.historyStatus.partial",
  "cancelled": "security.historyStatus.cancelled",
  "failed": "security.historyStatus.failed",
  "interrupted": "security.historyStatus.interrupted",
} satisfies Record<NonNullable<
  | Schema["VulnerabilityScanRun"]["status"]
>, CatalogueKey>;

export const SYSTEM_HINT_STATE_KEYS = {
  "current": "common.stackUpdates.hint.current",
  "available": "common.stackUpdates.hint.available",
  "pinned": "common.stackUpdates.hint.pinned",
  "unknown": "common.stackUpdates.hint.unknown",
} satisfies Record<NonNullable<
  | Schema["UpdaterSystemImageHint"]["state"]
>, CatalogueKey>;

export const SYSTEM_HINT_KIND_KEYS = {
  "agent": "hosts.card.systemComponent.agent",
  "tools": "hosts.components.tools",
} satisfies Record<NonNullable<
  | Schema["UpdaterSystemImageHint"]["kind"]
>, CatalogueKey>;

export const CANDIDATE_STATE_KEYS = {
  unchecked: "security.candidates.states.unchecked", queued: "security.candidates.states.queued",
  running: "security.candidates.states.running", completed: "security.candidates.states.completed",
  failed: "security.candidates.states.failed", cancelled: "security.candidates.states.cancelled",
  interrupted: "security.candidates.states.interrupted", deferred: "security.candidates.states.deferred",
} satisfies Record<Schema["VulnerabilityCandidateResult"]["state"], CatalogueKey>;
export const CANDIDATE_RUN_STATUS_KEYS = {
  running: "security.historyStatus.running", completed: "security.historyStatus.completed",
  partial: "security.historyStatus.partial", failed: "security.historyStatus.failed",
  cancelled: "security.historyStatus.cancelled", interrupted: "security.historyStatus.interrupted",
} satisfies Record<Schema["VulnerabilityCandidateRun"]["status"], CatalogueKey>;
