// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import { REQUEST_CODE_KEYS, IMAGE_ERROR_KEYS, CONTAINER_CODE_KEYS, HOST_CODE_KEYS, ACCESS_CODE_KEYS, NETWORK_CODE_KEYS, WIDGET_CODE_KEYS } from "./wire-catalogue-errors";
import { BACKUP_CODE_KEYS, BACKUP_REASON_KEYS, VOLUME_COMMAND_KEYS } from "./wire-catalogue-backup";
import { NOTIFICATION_CODE_KEYS, AUTOHEAL_CODE_KEYS, NOTIFICATION_TEST_KEYS, DELIVERY_REASON_KEYS, SETTINGS_CODE_KEYS } from "./wire-catalogue-settings";
import { IMPORT_PROBLEM_KEYS } from "./wire-catalogue-import";
import { MESSAGE_CODE_KEYS } from "./wire-catalogue-messages";
import { BACKUP_STATUS_KEYS, BACKUP_TARGET_KIND_KEYS, MAINTENANCE_PHASE_KEYS, HUB_BACKUP_STATUS_KEYS, BACKUP_DELIVERY_STATE_KEYS, RESTART_STATE_KEYS, HISTORY_STATE_KEYS, PRUNE_FAILURE_KEYS, PRUNE_CANDIDATE_KEYS, PRUNE_SKIP_KEYS, SCAN_STATUS_KEYS, SYSTEM_HINT_STATE_KEYS, SYSTEM_HINT_KIND_KEYS } from "./wire-catalogue-states";

/** Exact property identities are pinned, including equivalent unions on different DTOs. */
export const GUARDED_WIRE_ENUMS = [
  { name: "REQUEST_CODE_KEYS", keys: REQUEST_CODE_KEYS, properties: ["RequestProblem.code"] },
  { name: "IMAGE_ERROR_KEYS", keys: IMAGE_ERROR_KEYS, properties: ["JobscheduleJobStatus.code", "ProjectUpdateInfo.policyCode", "SchedulerOutcome.code", "SchedulerTargetOutcome.code", "UpdaterAutoUpdatePolicy.code", "UpdaterBaselineStatus.code", "UpdaterLifecycleResult.baselineCode", "UpdaterLifecycleResult.code", "UpdaterLifecycleResult.pointCode", "UpdaterRestorePoint.code", "UpdaterSystemImageHint.code", "ImageProblem.code", "ImagePruneItem.code", "VulnerabilityProblem.code", "VulnerabilityScanResult.errorCode", "VulnerabilityScanRun.errorCode", "VulnerabilityScanSummary.errorCode", "HistoryProblem.code", "PreferencesProblem.code"] },
  { name: "BACKUP_CODE_KEYS", keys: BACKUP_CODE_KEYS, properties: ["BackupProblem.code"] },
  { name: "BACKUP_REASON_KEYS", keys: BACKUP_REASON_KEYS, properties: ["BackupProblem.reason", "VolumeRestoreRun.reason"] },
  { name: "CONTAINER_CODE_KEYS", keys: CONTAINER_CODE_KEYS, properties: ["ContainerProblem.code"] },
  { name: "HOST_CODE_KEYS", keys: HOST_CODE_KEYS, properties: ["EnvironmentHostProblem.code", "EnvironmentAgentRestartRun.code"] },
  { name: "ACCESS_CODE_KEYS", keys: ACCESS_CODE_KEYS, properties: ["UserAccessProblem.code"] },
  { name: "NETWORK_CODE_KEYS", keys: NETWORK_CODE_KEYS, properties: ["NetworkError.code"] },
  { name: "WIDGET_CODE_KEYS", keys: WIDGET_CODE_KEYS, properties: ["WidgetProblem.code"] },
  { name: "NOTIFICATION_CODE_KEYS", keys: NOTIFICATION_CODE_KEYS, properties: ["NotificationProblem.code"] },
  { name: "AUTOHEAL_CODE_KEYS", keys: AUTOHEAL_CODE_KEYS, properties: ["AutohealProblem.code"] },
  { name: "SETTINGS_CODE_KEYS", keys: SETTINGS_CODE_KEYS, properties: ["SettingsProblem.code"] },
  { name: "IMPORT_PROBLEM_KEYS", keys: IMPORT_PROBLEM_KEYS, properties: ["StackimportProblem.code"] },
  { name: "NOTIFICATION_TEST_KEYS", keys: NOTIFICATION_TEST_KEYS, properties: ["NotificationTestResponse.code"] },
  { name: "DELIVERY_REASON_KEYS", keys: DELIVERY_REASON_KEYS, properties: ["NotificationTestResponse.reason"] },
  { name: "MESSAGE_CODE_KEYS", keys: MESSAGE_CODE_KEYS, properties: ["ActivityActivity.messageCode", "ActivityActivitySummary.messageCode", "ActivityMessage.code", "ActivityRuntimeAdmission.code", "ActivityRuntimeConflict.code", "ActivityRuntimeExecutionRefusal.code", "ActivityRuntimeHTTPError.code", "ActivityRuntimeSnapshot.messageCode", "ActivityStepTraceEntry.code", "EventEvent.messageCode"] },
  { name: "BACKUP_STATUS_KEYS", keys: BACKUP_STATUS_KEYS, properties: ["DockerVolumeVolumeBackup.status", "VolumeBackupEntry.status", "VolumeRestoreRun.status"] },
  { name: "BACKUP_TARGET_KIND_KEYS", keys: BACKUP_TARGET_KIND_KEYS, properties: ["BackupSyncedTarget.kind", "BackupTarget.kind", "BackupTargetInput.kind", "VolumeUsableBackupTarget.kind"] },
  { name: "MAINTENANCE_PHASE_KEYS", keys: MAINTENANCE_PHASE_KEYS, properties: ["BackupSystemBackupMaintenance.phase"] },
  { name: "HUB_BACKUP_STATUS_KEYS", keys: HUB_BACKUP_STATUS_KEYS, properties: ["BackupSystemBackupRun.status"] },
  { name: "BACKUP_DELIVERY_STATE_KEYS", keys: BACKUP_DELIVERY_STATE_KEYS, properties: ["BackupTargetHostDelivery.state"] },
  { name: "RESTART_STATE_KEYS", keys: RESTART_STATE_KEYS, properties: ["EnvironmentAgentRestartRun.state"] },
  { name: "HISTORY_STATE_KEYS", keys: HISTORY_STATE_KEYS, properties: ["HistoryBucket.state"] },
  { name: "PRUNE_FAILURE_KEYS", keys: PRUNE_FAILURE_KEYS, properties: ["VolumePruneFailed.code"] },
  { name: "PRUNE_CANDIDATE_KEYS", keys: PRUNE_CANDIDATE_KEYS, properties: ["VolumePrunePlanCandidate.reason"] },
  { name: "PRUNE_SKIP_KEYS", keys: PRUNE_SKIP_KEYS, properties: ["VolumePruneSkipped.reason"] },
  { name: "SCAN_STATUS_KEYS", keys: SCAN_STATUS_KEYS, properties: ["VulnerabilityScanRun.status"] },
  { name: "SYSTEM_HINT_STATE_KEYS", keys: SYSTEM_HINT_STATE_KEYS, properties: ["UpdaterSystemImageHint.state"] },
  { name: "SYSTEM_HINT_KIND_KEYS", keys: SYSTEM_HINT_KIND_KEYS, properties: ["UpdaterSystemImageHint.kind"] },
  { name: "VOLUME_COMMAND_KEYS", keys: VOLUME_COMMAND_KEYS, properties: ["VolumeExistsError.code", "VolumeHelperIncompatibleError.code", "VolumeHelperUnavailableError.code"] },
] as const;

export const EXEMPT_WIRE_ENUMS = [
  ...["SystemSelfUpdateEligibility.state", "SystemSelfUpdateEligibility.code", "SystemSelfUpdateOperation.phase", "SystemSelfUpdateOperation.outcome", "SystemSelfUpdateResponse.phase", "SystemSelfUpdateResponse.code"].map((property) => ({ property, reason: "Backend recovery contract without a rendered consumer yet; the separately owned self-update UI step adds render proofs. Frozen typed maps: wire-catalogue-self-update.ts." })),
  { property: "ActivityStepTraceEntry.status", reason: "Mapped to a domain step state before display: web/src/data/http/map-dto.ts." },
  { property: "HistoryCondition.kind", reason: "Transport selector for condition projection, never rendered: web/src/data/http/runtime-conditions.ts." },
  { property: "HistoryCondition.reason", reason: "Transport evidence for runtime conditions, never rendered: web/src/data/http/runtime-conditions.ts." },
  { property: "HistoryContainerSample.state", reason: "Transport sample input for hub aggregation, never rendered." },
  { property: "ImagePullProgressFrame.status", reason: "Mapped to a domain progress phase before display: web/src/data/http/image-mapping.ts." },
  { property: "ImageUpdateInfo.state", reason: "Mapped to a domain update variant before display: web/src/data/http/update-mapping.ts." },
  { property: "ImageupdateResponse.state", reason: "Mapped to a domain update variant before display: web/src/data/http/update-mapping.ts." },
  { property: "NotificationDispatchAutoHeal.outcome", reason: "Outbound notification transport only, never rendered by the web." },
  { property: "NotificationDispatchRequest.kind", reason: "Outbound notification transport only, never rendered by the web." },
  { property: "NotificationDispatchVulnerabilityScan.outcome", reason: "Outbound notification transport only, never rendered by the web." },
  { property: "ProjectRuntimeService.status", reason: "Mapped to a domain container state before display: web/src/data/http/inventory-mapping.ts." },
  { property: "RoleAccessSurface.kind", reason: "Transport selector for role access checks, never rendered." },
  { property: "UpdaterImageSelection.kind", reason: "Transport selector for update admission, never rendered." },
  { property: "UpdaterUpdateWindow.kind", reason: "Mapped to a domain schedule before display: web/src/shared/components/updates/policy-control.tsx." },
  { property: "VolumeBackupRunning.kind", reason: "Transport selector for activity following, never rendered: web/src/features/volumes/backups/VolumeBackupsTab.tsx." },
  { property: "VolumeBind.kind", reason: "Mapped to domain bind flags before display: web/src/data/http/inventory-mapping.ts." },
  { property: "VolumeRestoreRun.kind", reason: "Transport selector for restore following, never rendered: web/src/features/volumes/backups/VolumeBackupsTab.tsx." },
  { property: "VolumeRestoreRun.phase", reason: "Transport restore progress evidence, never rendered by the web." },
  { property: "VulnerabilityScanResult.status", reason: "Mapped to a domain image scan state before display: web/src/data/http/vulnerabilities.ts." },
  { property: "VulnerabilityScanSummary.status", reason: "Mapped to a domain image scan state before display: web/src/data/http/vulnerabilities.ts." },
  { property: "WidgetConfig.kind", reason: "Mapped to a domain service kind before display: web/src/data/http/service-widgets.ts." },
  { property: "WidgetResult.kind", reason: "Mapped to a domain service kind before display: web/src/data/http/service-widgets.ts." },
  { property: "WidgetSummary.kind", reason: "Mapped to a domain service kind before display: web/src/data/http/service-widgets.ts." },
  { property: "StackimportRequest.kind", reason: "Request input chosen by the import dialog tab, never rendered from the wire." },
  { property: "StackimportSource.kind", reason: "Provenance echo of the request; the import dialog labels sources from its own tab selection." },
  { property: "BackupSystemBackupOperation.status", reason: "Transport receipt for activity following, never rendered: web/src/data/http/hub-backups.ts." },
] as const;
