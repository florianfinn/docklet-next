// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums. M21 #437: advisory detail.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

const CREDENTIAL_REJECTED_KEY = "common.commandErrors.credentialRejected" satisfies CatalogueKey;

export const REQUEST_CODE_KEYS = {
  "request.path-encoded": "runMessages.pathEncoded",
} satisfies Record<Schema["RequestProblem"]["code"], CatalogueKey>;

export const IMAGE_ERROR_KEYS = {
  "agent.unavailable": "images.errors.agent_unavailable",
  "container.origin-locked": "images.errors.container_origin-locked",
  "container.recreate-ambiguous": "images.errors.container_recreate-ambiguous",
  "container.recreate-recovered": "images.errors.container_recreate-recovered",
  "autoheal.interrupted": "images.errors.autoheal_interrupted",
  "autoheal.limit-reached": "images.errors.autoheal_limit-reached",
  "autoheal.restart-failed": "images.errors.autoheal_restart-failed",
  "autoheal.restarted": "images.errors.autoheal_restarted",
  "backup.copy.insufficient-space": "images.errors.backup_copy_insufficient-space",
  "backup.copy.mount-failed": "images.errors.backup_copy_mount-failed",
  "backup.copy.write-failed": "images.errors.backup_copy_write-failed",
  "deploy.point-unavailable": "images.errors.deploy_point-unavailable",
  "feature-disabled": "images.errors.feature-disabled",
  "history.invalid-range": "images.errors.history_invalid-range",
  "history.sample-invalid": "images.errors.history_sample-invalid",
  "history.sample-too-large": "images.errors.history_sample-too-large",
  "history.unavailable": "images.errors.history_unavailable",
  "host.restarting": "images.errors.host_restarting",
  "image.active-update-protected": "images.errors.image_active-update-protected",
  "image.busy": "images.errors.image_busy",
  "image.changed": "images.errors.image_changed",
  "image.in-use": "images.errors.image_in-use",
  "image.invalid-archive": "images.errors.image_invalid-archive",
  "image.invalid-prune": "images.errors.image_invalid-prune",
  "image.invalid-reference": "images.errors.image_invalid-reference",
  "image.load-failed": "images.errors.image_load-failed",
  "image.not-found": "images.errors.image_not-found",
  "image.prune-failed": "images.errors.image_prune-failed",
  "image.pull-failed": "images.errors.image_pull-failed",
  "image.remove-failed": "images.errors.image_remove-failed",
  "image.reserved-reference": "images.errors.image_reserved-reference",
  "image.restore-point-protected": "images.errors.image_restore-point-protected",
  "image.system-protected": "images.errors.image_system-protected",
  "image.tag-failed": "images.errors.image_tag-failed",
  "image.usage-unavailable": "images.errors.image_usage-unavailable",
  "job.skipped": "images.errors.job_skipped",
  "preferences.invalid": "images.errors.preferences_invalid",
  "preferences.no-user": "images.errors.preferences_no-user",
  "preferences.too-large": "images.errors.preferences_too-large",
  "preferences.unknown-section": "images.errors.preferences_unknown-section",
  "registry.authentication-failed": "images.errors.registry_authentication-failed",
  "registry.authentication-required": "images.errors.registry_authentication-required",
  "registry.credential-unavailable": "images.errors.registry_credential-unavailable",
  "registry.exists": "images.errors.registry_exists",
  "registry.inline-credentials-not-allowed": "images.errors.registry_inline-credentials-not-allowed",
  "registry.invalid-config": "images.errors.registry_invalid-config",
  "registry.not-found": "images.errors.registry_not-found",
  "registry.persistence-failed": "images.errors.registry_persistence-failed",
  "registry.rate-limited": "images.errors.registry_rate-limited",
  "registry.sync-failed": "images.errors.registry_sync-failed",
  "registry.unavailable": "images.errors.registry_unavailable",
  "restore.deploy-failed": "images.errors.restore_deploy-failed",
  "restore.image-mismatch": "images.errors.restore_image-mismatch",
  "restore.in-use": "images.errors.restore_in-use",
  "restore.not-found": "images.errors.restore_not-found",
  "restore.shared-reference": "images.errors.restore_shared-reference",
  "restore.unavailable": "images.errors.restore_unavailable",
  "settings.invalid-time-zone": "images.errors.settings_invalid-time-zone",
  "run.busy": "images.errors.run_busy",
  "run.cancelling": "images.errors.run_cancelling",
  "runtime.interrupted": "images.errors.runtime_interrupted",
  "runtime.outcome-unknown": "images.errors.runtime_outcome-unknown",
  "runtime.parent-unresolved": "images.errors.runtime_parent-unresolved",
  "runtime.target-changed": "images.errors.runtime_target-changed",
  "runtime.target-unavailable": "images.errors.runtime_target-unavailable",
  "update.agent-local-job-disabled": "images.errors.update_agent-local-job-disabled",
  "update.auto-excluded": "images.errors.update_auto-excluded",
  "update.baseline-unavailable": "images.errors.update_baseline-unavailable",
  "update.check-unavailable": "images.errors.update_check-unavailable",
  "update.config-drift": "images.errors.update_config-drift",
  "update.deploy-failed": "images.errors.update_deploy-failed",
  "update.image-binding-unsupported": "images.errors.update_image-binding-unsupported",
  "update.invalid-policy": "images.errors.update_invalid-policy",
  "update.invalid-target": "images.errors.update_invalid-target",
  "update.path-retired": "images.errors.update_path-retired",
  "update.permission-denied": "images.errors.update_permission-denied",
  "update.persistence-failed": "images.errors.update_persistence-failed",
  "update.policy-changed": "images.errors.update_policy-changed",
  "update.requires-hub-update": "images.errors.update_requires-hub-update",
  "update.schedule-hub-owned": "images.errors.update_schedule-hub-owned",
  "update.schedule-owner-unset": "images.errors.update_schedule-owner-unset",
  "update.service-coupled": "images.errors.update_service-coupled",
  "update.snapshot-unavailable": "images.errors.update_snapshot-unavailable",
  "update.system-version-unknown": "images.errors.update_system-version-unknown",
  "update.target-changed": "images.errors.update_target-changed",
  "update.target-not-found": "images.errors.update_target-not-found",
  "volume.workspace-busy": "images.errors.volume_workspace-busy",
  "vulnerability.agent-job-retired": "images.errors.vulnerability_agent-job-retired",
  "vulnerability.busy": "images.errors.vulnerability_busy",
  "vulnerability.database-unavailable": "images.errors.vulnerability_database-unavailable",
  "vulnerability.dispatch-interrupted": "images.errors.vulnerability_dispatch-interrupted",
  "vulnerability.dispatch-unknown": "images.errors.vulnerability_dispatch-unknown",
  "vulnerability.host-incompatible": "images.errors.vulnerability_host-incompatible",
  "vulnerability.host-offline": "images.errors.vulnerability_host-offline",
  "vulnerability.host-pending": "images.errors.vulnerability_host-pending",
  "vulnerability.image-not-found": "images.errors.vulnerability_image-not-found",
  "vulnerability.image-removed": "images.errors.vulnerability_image-removed",
  "vulnerability.no-eligible-hosts": "images.errors.vulnerability_no-eligible-hosts",
  "vulnerability.platform-unavailable": "images.errors.vulnerability_platform-unavailable",
  "vulnerability.report-invalid": "images.errors.vulnerability_report-invalid",
  "vulnerability.request-too-large": "images.errors.vulnerability_request-too-large",
  "vulnerability.response-too-large": "images.errors.vulnerability_response-too-large",
  "vulnerability.run-not-found": "images.errors.vulnerability_run-not-found",
  "vulnerability.scan-cancelled": "images.errors.vulnerability_scan-cancelled",
  "vulnerability.scan-failed": "images.errors.vulnerability_scan-failed",
  "vulnerability.scan-interrupted": "images.errors.vulnerability_scan-interrupted",
  "vulnerability.scan-timeout": "images.errors.vulnerability_scan-timeout",
  "vulnerability.scanner-unavailable": "images.errors.vulnerability_scanner-unavailable",
  "vulnerability.storage-full": "images.errors.vulnerability_storage-full",
  "vulnerability.storage-unavailable": "images.errors.vulnerability_storage-unavailable",
  "agent.invalid-token": CREDENTIAL_REJECTED_KEY,
} satisfies Record<NonNullable<
  | Schema["JobscheduleJobStatus"]["code"]
  | Schema["ProjectUpdateInfo"]["policyCode"]
  | Schema["SchedulerOutcome"]["code"]
  | Schema["SchedulerTargetOutcome"]["code"]
  | Schema["UpdaterAutoUpdatePolicy"]["code"]
  | Schema["UpdaterBaselineStatus"]["code"]
  | Schema["UpdaterLifecycleResult"]["baselineCode"]
  | Schema["UpdaterLifecycleResult"]["code"]
  | Schema["UpdaterLifecycleResult"]["pointCode"]
  | Schema["UpdaterRestorePoint"]["code"]
  | Schema["UpdaterSystemImageHint"]["code"]
  | Schema["ImageProblem"]["code"]
  | Schema["ImagePruneItem"]["code"]
  | Schema["VulnerabilityProblem"]["code"]
  | Schema["VulnerabilityScanResult"]["errorCode"]
  | Schema["VulnerabilityScanRun"]["errorCode"]
  | Schema["VulnerabilityScanSummary"]["errorCode"]
  | Schema["HistoryProblem"]["code"]
  | Schema["PreferencesProblem"]["code"]
>, CatalogueKey>;

export const CONTAINER_CODE_KEYS = {
  "container.origin-locked": "runMessages.codes.container.origin-locked",
  "container.recreate-recovered": "runMessages.codes.container.recreate-recovered",
  "container.recreate-ambiguous": "runMessages.codes.container.recreate-ambiguous",
  "runtime.idempotency-mismatch": "runMessages.codes.runtime.idempotency-mismatch",
  "update.path-retired": "runMessages.codes.update.path-retired",
  "container.shell-invalid": "runMessages.codes.container.shell-invalid",
  "container.shell-protocol": "runMessages.codes.container.shell-protocol",
  "container.shell-unavailable": "runMessages.codes.container.shell-unavailable",
  "container.shell-orphaned": "runMessages.codes.container.shell-orphaned",
  "container.shell-not-running": "runMessages.codes.container.shell-not-running",
} satisfies Record<NonNullable<
  | Schema["ContainerProblem"]["code"]
>, CatalogueKey>;

export const HOST_CODE_KEYS = {
  "agent.restart-unsupported": "runMessages.codes.agent.restart-unsupported",
  "agent.restart-superseded": "runMessages.codes.agent.restart-superseded",
  "agent.restart-timeout": "runMessages.codes.agent.restart-timeout",
  "agent.restart-unconfirmed": "runMessages.codes.agent.restart-unconfirmed",
  "agent.restart-interrupted": "runMessages.codes.agent.restart-interrupted",
  "host.busy": "runMessages.codes.host.busy",
  "host.restarting": "runMessages.codes.host.restarting",
  "host.name-invalid": "runMessages.codes.host.name-invalid",
  "host.name-taken": "runMessages.codes.host.name-taken",
  "host.unreachable": "runMessages.codes.host.unreachable",
  "edge.heartbeat-stale": "runMessages.codes.edge.heartbeat-stale",
  "agent.ui-configuration-disabled": "runMessages.codes.agent.ui-configuration-disabled",
  "settings.ui-configuration-disabled": "runMessages.codes.settings.ui-configuration-disabled",
} satisfies Record<NonNullable<
  | Schema["EnvironmentHostProblem"]["code"]
>, CatalogueKey>;

export const ACCESS_CODE_KEYS = {
  "access.not-found": "containers.operationReasons.accessNotFound",
  "access.admin-required": "containers.operationReasons.accessAdminRequired",
  "access.full-required": "containers.operationReasons.accessFullRequired",
  "access.disabled": "containers.operationReasons.accessDisabled",
  "access.not-registered": "containers.operationReasons.accessNotRegistered",
  "access.unavailable": "containers.operationReasons.accessUnavailable",
} satisfies Record<NonNullable<
  | Schema["UserAccessProblem"]["code"]
>, CatalogueKey>;

export const NETWORK_CODE_KEYS = {
  "network.not-found": "networks.errors.not-found",
  "network.exists": "networks.errors.exists",
  "network.invalid-network": "networks.errors.invalid-network",
  "network.builtin-network": "networks.errors.builtin-network",
  "network.compose-managed": "networks.errors.compose-managed",
  "network.in-use": "networks.errors.in-use",
  "network.network-mode": "networks.errors.network-mode",
  "network.already-connected": "networks.errors.already-connected",
  "network.not-connected": "networks.errors.not-connected",
  "network.invalid-ip": "networks.errors.invalid-ip",
  "network.overlap": "networks.errors.overlap",
} satisfies Record<NonNullable<
  | Schema["NetworkError"]["code"]
>, CatalogueKey>;

export const WIDGET_CODE_KEYS = {
  "widget.not-configured": "dashboard.services.errors.not-configured",
  "widget.disabled": "dashboard.services.errors.disabled",
  "widget.unreachable": "dashboard.services.errors.unreachable",
  "widget.timeout": "dashboard.services.errors.timeout",
  "widget.auth-failed": "dashboard.services.errors.auth-failed",
  "widget.bad-response": "dashboard.services.errors.bad-response",
  "widget.redirect-refused": "dashboard.services.errors.redirect-refused",
  "widget.tls-failed": "dashboard.services.errors.tls-failed",
  "widget.credential-unavailable": "dashboard.services.errors.credential-unavailable",
  "widget.invalid-config": "dashboard.services.errors.invalid-config",
  "widget.persistence-failed": "dashboard.services.errors.persistence-failed",
} satisfies Record<NonNullable<
  | Schema["WidgetProblem"]["code"]
>, CatalogueKey>;

export const ADVISORY_USAGE_CODE_KEYS = {
  "image.usage-unavailable": "security.detail.usageUnavailable",
} satisfies Record<NonNullable<Schema["VulnerabilityAdvisoryUsage"]["code"]>, CatalogueKey>;
