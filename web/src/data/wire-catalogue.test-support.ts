// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums. M21 #437: advisory detail. M21 #438: candidate checks and verified updates. M24 recovery vocabulary classification. M24 S6: observed recovery and update UI follow-ups; optional-module foundation (#472). M27 proxy contract foundation (#473/#474). M36 tailnet route vocabulary (#573).

import {PROXY_ADOPTION_REASON_KEYS,PROXY_ADOPTION_OUTCOME_KEYS,PROXY_STATE_KEYS,PROXY_REASON_KEYS} from "./wire-catalogue-proxy";
import { MODULE_CODE_KEYS } from "./wire-catalogue-modules";
import { TAILNET_CODE_KEYS, TAILNET_CONNECTION_KEYS, TAILNET_PROVISIONING_KEYS, TAILNET_PEER_KEYS, TAILNET_TEST_STATE_KEYS } from "./wire-catalogue-tailnet";
import { IMAGE_BINDING_KEYS, RECOVERY_OUTCOME_KEYS } from "./wire-catalogue-update";
import { ADVISORY_USAGE_CODE_KEYS, REQUEST_CODE_KEYS, IMAGE_ERROR_KEYS, CONTAINER_CODE_KEYS, HOST_CODE_KEYS, ACCESS_CODE_KEYS, NETWORK_CODE_KEYS, WIDGET_CODE_KEYS } from "./wire-catalogue-errors";
import { BACKUP_CODE_KEYS, BACKUP_REASON_KEYS, VOLUME_COMMAND_KEYS } from "./wire-catalogue-backup";
import { NOTIFICATION_CODE_KEYS, AUTOHEAL_CODE_KEYS, NOTIFICATION_TEST_KEYS, DELIVERY_REASON_KEYS, SETTINGS_CODE_KEYS } from "./wire-catalogue-settings";
import { IMPORT_PROBLEM_KEYS } from "./wire-catalogue-import";
import { MESSAGE_CODE_KEYS } from "./wire-catalogue-messages";
import { CANDIDATE_STATE_KEYS, CANDIDATE_RUN_STATUS_KEYS, BACKUP_STATUS_KEYS, BACKUP_TARGET_KIND_KEYS, MAINTENANCE_PHASE_KEYS, HUB_BACKUP_STATUS_KEYS, BACKUP_DELIVERY_STATE_KEYS, RESTART_STATE_KEYS, HISTORY_STATE_KEYS, PRUNE_FAILURE_KEYS, PRUNE_CANDIDATE_KEYS, PRUNE_SKIP_KEYS, SCAN_STATUS_KEYS, SYSTEM_HINT_STATE_KEYS, SYSTEM_HINT_KIND_KEYS } from "./wire-catalogue-states";

/** Exact property identities are pinned, including equivalent unions on different DTOs. */
export const GUARDED_WIRE_ENUMS = [
 { name: "IMAGE_BINDING_KEYS", keys: IMAGE_BINDING_KEYS, properties: ["ImageBinding.reason"] },
{name:"PROXY_ADOPTION_REASON_KEYS",keys:PROXY_ADOPTION_REASON_KEYS,properties:["ProxyAdoption.reason"]},
{name:"PROXY_ADOPTION_OUTCOME_KEYS",keys:PROXY_ADOPTION_OUTCOME_KEYS,properties:["ProxyAdoption.outcome"]},
{name:"PROXY_STATE_KEYS",keys:PROXY_STATE_KEYS,properties:["ProxyOperation.state"]},
{name:"PROXY_REASON_KEYS",keys:PROXY_REASON_KEYS,properties:["ProxyProblem.code"]},

 { name: "TAILNET_CODE_KEYS", keys: TAILNET_CODE_KEYS, properties: ["TailnetProblem.code", "TailnetConnection.code", "TailnetProvisioning.code", "TailnetTestCheck.code"] },
  { name: "TAILNET_TEST_STATE_KEYS", keys: TAILNET_TEST_STATE_KEYS, properties: ["TailnetTestCheck.state"] },
  { name: "TAILNET_CONNECTION_KEYS", keys: TAILNET_CONNECTION_KEYS, properties: ["TailnetConnection.state"] },
  { name: "TAILNET_PROVISIONING_KEYS", keys: TAILNET_PROVISIONING_KEYS, properties: ["TailnetProvisioning.state"] },
  { name: "TAILNET_PEER_KEYS", keys: TAILNET_PEER_KEYS, properties: ["TailnetHostPeer.state"] },
 {name:"MODULE_CODE_KEYS",keys:MODULE_CODE_KEYS,properties:["ModulesModuleProblem.code"]},
  { name: "RECOVERY_OUTCOME_KEYS", keys: RECOVERY_OUTCOME_KEYS, properties: ["UpdaterRecoveryResult.outcome"] },
  { name: "CANDIDATE_STATE_KEYS", keys: CANDIDATE_STATE_KEYS, properties: ["VulnerabilityCandidateResult.state"] },
  { name: "CANDIDATE_RUN_STATUS_KEYS", keys: CANDIDATE_RUN_STATUS_KEYS, properties: ["VulnerabilityCandidateRun.status"] },
  { name: "ADVISORY_USAGE_CODE_KEYS", keys: ADVISORY_USAGE_CODE_KEYS, properties: ["VulnerabilityAdvisoryUsage.code"] },
  { name: "REQUEST_CODE_KEYS", keys: REQUEST_CODE_KEYS, properties: ["RequestProblem.code"] },
  { name: "IMAGE_ERROR_KEYS", keys: IMAGE_ERROR_KEYS, properties: ["JobscheduleJobStatus.code", "ProjectUpdateInfo.policyCode", "SchedulerOutcome.code", "SchedulerTargetOutcome.code", "UpdaterAutoUpdatePolicy.code", "UpdaterBaselineStatus.code", "UpdaterLifecycleResult.baselineCode", "UpdaterLifecycleResult.code", "UpdaterLifecycleResult.pointCode", "UpdaterRecoveryResult.code", "UpdaterRestorePoint.code", "UpdaterSystemImageHint.code", "ImageProblem.code", "ImagePruneItem.code", "VulnerabilityProblem.code", "VulnerabilityScanResult.errorCode", "VulnerabilityScanRun.errorCode", "VulnerabilityScanSummary.errorCode", "HistoryProblem.code", "PreferencesProblem.code"] },
  { name: "BACKUP_CODE_KEYS", keys: BACKUP_CODE_KEYS, properties: ["BackupProblem.code"] },
  { name: "BACKUP_REASON_KEYS", keys: BACKUP_REASON_KEYS, properties: ["BackupProblem.reason", "VolumeRestoreRun.reason"] },
  { name: "CONTAINER_CODE_KEYS", keys: CONTAINER_CODE_KEYS, properties: ["ContainerProblem.code", "ContainerLogsDownloadProblem.code"] },
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

 { property: "ImageRuntimeIdentityPeer.reason", reason: "Validated by runtime-blocker.ts and rendered through common.runtimePeers reason texts by the confirmation dialog and audit mapper; runtime-peer-confirmation.test.tsx exercises both locales." },
 { property: "ProjectDetails.definitionReason", reason: "Rendered by use-stack-definition-hint.ts through common.runtimePeers reason texts; runtime identity marker and repair proofs retain the existing coverage limit." },
 ...["ProxyOperation.phase", "ProxyInstance.kind", "ProxyDiscovery.kind", "ProxyCreateInstanceRequest.kind", "ProxySource.kind", "ProxyReference.kind"].map((property) => ({ property, reason: "S2 contract foundation has no production proxy page; S5 owns these rendered consumers and their render proofs. Total typed bilingual maps are frozen in wire-catalogue-proxy.ts. Outcome state/reason consumers already have guarded render proofs." })),
{property:"EnvironmentHubAddressCandidate.kind",reason:"Address classification (ip or domain) read only by the discovery ranker and prefill; candidate labels come from their source, never from this wire value."},
...["ProxyApplyRequest.kind", "ProxyResolverEnvelope.kind", "ProxyResolverFile.kind", "ProxyResolverReplacement.kind"].map((property) => ({ property, reason: "M28 transport and journal discriminator between route and resolver payloads; no screen renders this selector." })),
...["ProxyResolverCA.kind", "ProxyResolverChallenge.kind", "ProxyResolverDiagnostic.code", "ProxyResolverReceipt.phase"].map((property) => ({ property, reason: "M28 S1 contract foundation has no resolver page; S5 owns the resolver dialog and receipt consumers and their render proofs." })),
{property:"ProxyChange.kind",reason:"Request action discriminator selected by the proxy editor; action captions come from feature labels and never display this transport selector."},
 {property:"ModulesState.kind",reason:"Static registry discriminator validated by the module decoder; the module settings UI renders translated module names, never this wire selector."},
  { property: "VulnerabilityAdvisoryUpdate.state", reason: "Consumer update states are translated with security.detail.updateAvailable, noUpdate and updateUnknown in AdvisoryContent." },
  { property: "VulnerabilityAdvisoryUsage.state", reason: "Usage availability controls the explicit security.detail.usageUnavailable message in AdvisoryContent." },
  ...["SystemSelfUpdateEligibility.state", "SystemSelfUpdateEligibility.code", "SystemSelfUpdateOperation.phase", "SystemSelfUpdateOperation.outcome", "SystemSelfUpdateResponse.phase", "SystemSelfUpdateResponse.code"].map((property) => ({ property, reason: "Backend recovery contract without a rendered consumer yet; the separately owned self-update UI step adds render proofs. Frozen typed maps: wire-catalogue-self-update.ts." })),
  { property: "ActivityStepTraceEntry.status", reason: "Mapped to a domain step state before display: web/src/data/http/map-dto.ts." },
  { property: "HistoryCondition.kind", reason: "Transport selector for condition projection, never rendered: web/src/data/http/runtime-conditions.ts." },
  { property: "HistoryCondition.reason", reason: "Transport evidence for runtime conditions, never rendered: web/src/data/http/runtime-conditions.ts." },
  { property: "HistoryContainerSample.state", reason: "Transport sample input for hub aggregation, never rendered." },
  { property: "ImagePullProgressFrame.status", reason: "Mapped to a domain progress phase before display: web/src/data/http/image-mapping.ts." },
  { property: "ImageRuntimeIdentityBlocker.reason", reason: "Validated by web/src/data/runtime-blocker.ts and mapped to localized peer confirmation, duplicate, and legacy refusal text." },
  { property: "ProjectWorkspaceProblem.code", reason: "Mapped to the workspace permission text in web/src/data/http/workspace.ts; the code itself is never rendered." },
  { property: "VolumeWorkspaceProblem.code", reason: "Mapped to the workspace permission text in web/src/data/http/workspace.ts; the code itself is never rendered." },
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
