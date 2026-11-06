// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";
import { IMAGE_ERROR_KEYS } from "./wire-catalogue-errors";

type Schema = components["schemas"];

// Update admission accepts this contract family, independently of other image-error consumers.
export const UPDATE_REASON_KEYS = {
  "agent.unavailable": IMAGE_ERROR_KEYS["agent.unavailable"],
  "container.origin-locked": IMAGE_ERROR_KEYS["container.origin-locked"],
  "container.recreate-ambiguous": IMAGE_ERROR_KEYS["container.recreate-ambiguous"],
  "container.recreate-recovered": IMAGE_ERROR_KEYS["container.recreate-recovered"],
  "autoheal.interrupted": IMAGE_ERROR_KEYS["autoheal.interrupted"],
  "autoheal.limit-reached": IMAGE_ERROR_KEYS["autoheal.limit-reached"],
  "autoheal.restart-failed": IMAGE_ERROR_KEYS["autoheal.restart-failed"],
  "autoheal.restarted": IMAGE_ERROR_KEYS["autoheal.restarted"],
  "backup.copy.insufficient-space": IMAGE_ERROR_KEYS["backup.copy.insufficient-space"],
  "backup.copy.mount-failed": IMAGE_ERROR_KEYS["backup.copy.mount-failed"],
  "backup.copy.write-failed": IMAGE_ERROR_KEYS["backup.copy.write-failed"],
  "deploy.point-unavailable": IMAGE_ERROR_KEYS["deploy.point-unavailable"],
  "feature-disabled": IMAGE_ERROR_KEYS["feature-disabled"],
  "history.invalid-range": IMAGE_ERROR_KEYS["history.invalid-range"],
  "history.sample-invalid": IMAGE_ERROR_KEYS["history.sample-invalid"],
  "history.sample-too-large": IMAGE_ERROR_KEYS["history.sample-too-large"],
  "history.unavailable": IMAGE_ERROR_KEYS["history.unavailable"],
  "host.restarting": IMAGE_ERROR_KEYS["host.restarting"],
  "image.active-update-protected": IMAGE_ERROR_KEYS["image.active-update-protected"],
  "image.busy": IMAGE_ERROR_KEYS["image.busy"],
  "image.changed": IMAGE_ERROR_KEYS["image.changed"],
  "image.in-use": IMAGE_ERROR_KEYS["image.in-use"],
  "image.invalid-archive": IMAGE_ERROR_KEYS["image.invalid-archive"],
  "image.invalid-prune": IMAGE_ERROR_KEYS["image.invalid-prune"],
  "image.invalid-reference": IMAGE_ERROR_KEYS["image.invalid-reference"],
  "image.load-failed": IMAGE_ERROR_KEYS["image.load-failed"],
  "image.not-found": IMAGE_ERROR_KEYS["image.not-found"],
  "image.prune-failed": IMAGE_ERROR_KEYS["image.prune-failed"],
  "image.pull-failed": IMAGE_ERROR_KEYS["image.pull-failed"],
  "image.remove-failed": IMAGE_ERROR_KEYS["image.remove-failed"],
  "image.reserved-reference": IMAGE_ERROR_KEYS["image.reserved-reference"],
  "image.restore-point-protected": IMAGE_ERROR_KEYS["image.restore-point-protected"],
  "image.system-protected": IMAGE_ERROR_KEYS["image.system-protected"],
  "image.tag-failed": IMAGE_ERROR_KEYS["image.tag-failed"],
  "image.usage-unavailable": IMAGE_ERROR_KEYS["image.usage-unavailable"],
  "job.skipped": IMAGE_ERROR_KEYS["job.skipped"],
  "preferences.invalid": IMAGE_ERROR_KEYS["preferences.invalid"],
  "preferences.no-user": IMAGE_ERROR_KEYS["preferences.no-user"],
  "preferences.too-large": IMAGE_ERROR_KEYS["preferences.too-large"],
  "preferences.unknown-section": IMAGE_ERROR_KEYS["preferences.unknown-section"],
  "registry.authentication-failed": IMAGE_ERROR_KEYS["registry.authentication-failed"],
  "registry.authentication-required": IMAGE_ERROR_KEYS["registry.authentication-required"],
  "registry.credential-unavailable": IMAGE_ERROR_KEYS["registry.credential-unavailable"],
  "registry.exists": IMAGE_ERROR_KEYS["registry.exists"],
  "registry.inline-credentials-not-allowed": IMAGE_ERROR_KEYS["registry.inline-credentials-not-allowed"],
  "registry.invalid-config": IMAGE_ERROR_KEYS["registry.invalid-config"],
  "registry.not-found": IMAGE_ERROR_KEYS["registry.not-found"],
  "registry.persistence-failed": IMAGE_ERROR_KEYS["registry.persistence-failed"],
  "registry.rate-limited": IMAGE_ERROR_KEYS["registry.rate-limited"],
  "registry.sync-failed": IMAGE_ERROR_KEYS["registry.sync-failed"],
  "registry.unavailable": IMAGE_ERROR_KEYS["registry.unavailable"],
  "restore.deploy-failed": IMAGE_ERROR_KEYS["restore.deploy-failed"],
  "restore.image-mismatch": IMAGE_ERROR_KEYS["restore.image-mismatch"],
  "restore.in-use": IMAGE_ERROR_KEYS["restore.in-use"],
  "restore.not-found": IMAGE_ERROR_KEYS["restore.not-found"],
  "restore.shared-reference": IMAGE_ERROR_KEYS["restore.shared-reference"],
  "restore.unavailable": IMAGE_ERROR_KEYS["restore.unavailable"],
  "settings.invalid-time-zone": IMAGE_ERROR_KEYS["settings.invalid-time-zone"],
  "run.busy": IMAGE_ERROR_KEYS["run.busy"],
  "run.cancelling": IMAGE_ERROR_KEYS["run.cancelling"],
  "runtime.interrupted": IMAGE_ERROR_KEYS["runtime.interrupted"],
  "runtime.outcome-unknown": IMAGE_ERROR_KEYS["runtime.outcome-unknown"],
  "runtime.parent-unresolved": IMAGE_ERROR_KEYS["runtime.parent-unresolved"],
  "runtime.target-changed": IMAGE_ERROR_KEYS["runtime.target-changed"],
  "runtime.target-unavailable": IMAGE_ERROR_KEYS["runtime.target-unavailable"],
  "update.agent-local-job-disabled": IMAGE_ERROR_KEYS["update.agent-local-job-disabled"],
  "update.auto-excluded": IMAGE_ERROR_KEYS["update.auto-excluded"],
  "update.baseline-unavailable": IMAGE_ERROR_KEYS["update.baseline-unavailable"],
  "update.check-unavailable": IMAGE_ERROR_KEYS["update.check-unavailable"],
  "update.config-drift": IMAGE_ERROR_KEYS["update.config-drift"],
  "update.deploy-failed": IMAGE_ERROR_KEYS["update.deploy-failed"],
  "update.image-binding-unsupported": IMAGE_ERROR_KEYS["update.image-binding-unsupported"],
  "update.invalid-policy": IMAGE_ERROR_KEYS["update.invalid-policy"],
  "update.invalid-target": IMAGE_ERROR_KEYS["update.invalid-target"],
  "update.path-retired": IMAGE_ERROR_KEYS["update.path-retired"],
  "update.permission-denied": IMAGE_ERROR_KEYS["update.permission-denied"],
  "update.persistence-failed": IMAGE_ERROR_KEYS["update.persistence-failed"],
  "update.policy-changed": IMAGE_ERROR_KEYS["update.policy-changed"],
  "update.requires-hub-update": IMAGE_ERROR_KEYS["update.requires-hub-update"],
  "update.schedule-hub-owned": IMAGE_ERROR_KEYS["update.schedule-hub-owned"],
  "update.schedule-owner-unset": IMAGE_ERROR_KEYS["update.schedule-owner-unset"],
  "update.service-coupled": IMAGE_ERROR_KEYS["update.service-coupled"],
  "update.snapshot-unavailable": IMAGE_ERROR_KEYS["update.snapshot-unavailable"],
  "update.system-version-unknown": IMAGE_ERROR_KEYS["update.system-version-unknown"],
  "update.target-changed": IMAGE_ERROR_KEYS["update.target-changed"],
  "update.target-not-found": IMAGE_ERROR_KEYS["update.target-not-found"],
  "volume.workspace-busy": IMAGE_ERROR_KEYS["volume.workspace-busy"],
  "vulnerability.agent-job-retired": IMAGE_ERROR_KEYS["vulnerability.agent-job-retired"],
  "vulnerability.busy": IMAGE_ERROR_KEYS["vulnerability.busy"],
  "vulnerability.database-unavailable": IMAGE_ERROR_KEYS["vulnerability.database-unavailable"],
  "vulnerability.dispatch-interrupted": IMAGE_ERROR_KEYS["vulnerability.dispatch-interrupted"],
  "vulnerability.dispatch-unknown": IMAGE_ERROR_KEYS["vulnerability.dispatch-unknown"],
  "vulnerability.host-incompatible": IMAGE_ERROR_KEYS["vulnerability.host-incompatible"],
  "vulnerability.host-offline": IMAGE_ERROR_KEYS["vulnerability.host-offline"],
  "vulnerability.host-pending": IMAGE_ERROR_KEYS["vulnerability.host-pending"],
  "vulnerability.image-not-found": IMAGE_ERROR_KEYS["vulnerability.image-not-found"],
  "vulnerability.image-removed": IMAGE_ERROR_KEYS["vulnerability.image-removed"],
  "vulnerability.no-eligible-hosts": IMAGE_ERROR_KEYS["vulnerability.no-eligible-hosts"],
  "vulnerability.platform-unavailable": IMAGE_ERROR_KEYS["vulnerability.platform-unavailable"],
  "vulnerability.report-invalid": IMAGE_ERROR_KEYS["vulnerability.report-invalid"],
  "vulnerability.request-too-large": IMAGE_ERROR_KEYS["vulnerability.request-too-large"],
  "vulnerability.response-too-large": IMAGE_ERROR_KEYS["vulnerability.response-too-large"],
  "vulnerability.run-not-found": IMAGE_ERROR_KEYS["vulnerability.run-not-found"],
  "vulnerability.scan-cancelled": IMAGE_ERROR_KEYS["vulnerability.scan-cancelled"],
  "vulnerability.scan-failed": IMAGE_ERROR_KEYS["vulnerability.scan-failed"],
  "vulnerability.scan-interrupted": IMAGE_ERROR_KEYS["vulnerability.scan-interrupted"],
  "vulnerability.scan-timeout": IMAGE_ERROR_KEYS["vulnerability.scan-timeout"],
  "vulnerability.scanner-unavailable": IMAGE_ERROR_KEYS["vulnerability.scanner-unavailable"],
  "vulnerability.storage-full": IMAGE_ERROR_KEYS["vulnerability.storage-full"],
  "vulnerability.storage-unavailable": IMAGE_ERROR_KEYS["vulnerability.storage-unavailable"],
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
>, CatalogueKey>;
