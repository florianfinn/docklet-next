// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums; M21 #438 candidate and manifest-refusal render proofs. M21 #438: candidate checks and verified updates.

import { renderWithApp } from "@/test/render";
import { CandidateState, CandidateRunStatus } from "@/features/security/CandidateChecks";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { BACKUP_KIND_KEY_PREFIX } from "./wire-catalogue-states";
import { setAppLocale } from "@/app/i18n/locale-store";
import { widgetErrorKey } from "@/features/dashboard/services/widget-meta";
import { BackupError, backupErrorMessage } from "./backup-error";
import { AdapterOutcomeError } from "./capability";
import { SettingsProblemError, settingsErrorMessage } from "./settings-error";
import { importProblemMessage } from "./wire-catalogue-import";
import { mapActivityDetail } from "./http/map-dto";
import { renderMessage, renderRun } from "./message-code";
import { simulationStore } from "./demo/simulation";
import { GUARDED_WIRE_ENUMS } from "./wire-catalogue.test-support";
import { de, en, at, advisoryUsage, imageError, commandError, volumeOrNetworkError, backupStatus, backupTargets, hubBackup, restartPhase, restartFailure, historyState, prune, scanStatus, systemHint, notificationResult } from "./wire-catalogue-render.test-support";

beforeEach(() => { setAppLocale("de"); simulationStore.set((state) => ({ ...state, latency: "instant", live: false })); });
afterEach(() => { vi.restoreAllMocks(); simulationStore.reset(); });

function activityMessage(code: "runtime.wait-timeout" | "runtime.waiter-disconnected" | "runtime.target-removed" | "run.fresh") {
  const run = mapActivityDetail({ activity: { id: "example", type: "health", environmentId: "example", status: "running", startedAt: at, createdAt: at }, messages: [{ id: "example-message", activityId: "example", code, message: "English producer fallback", level: "info", createdAt: at }], output: [], stepTrace: [], outputTruncated: false });
  return renderMessage(run.log[0]!, "de");
}

// Each row names a representative property of one distinct guarded union and its real consumer.
const proofs = [
  { property: "VulnerabilityCandidateResult.state", member: "queued", key: "security.candidates.states.queued", render: () => renderWithApp(<CandidateState state="queued" />).container.textContent ?? "" },
  { property: "VulnerabilityCandidateRun.status", member: "running", key: "security.historyStatus.running", render: () => renderWithApp(<CandidateRunStatus status="running" />).container.textContent ?? "" },
  { property: "VulnerabilityScanResult.errorCode", member: "candidate.unavailable", key: "images.errors.candidate_unavailable", render: () => imageError("candidate.unavailable") },
  { property: "VulnerabilityScanResult.errorCode", member: "candidate.deferred", key: "images.errors.candidate_deferred", render: () => imageError("candidate.deferred") },
  { property: "UpdaterLifecycleResult.code", member: "update.candidate-changed", key: "images.errors.update_candidate-changed", render: () => imageError("update.candidate-changed") },
  { property: "VulnerabilityAdvisoryUsage.code", member: "image.usage-unavailable", key: "security.detail.usageUnavailable", render: () => advisoryUsage() },
  { property: "ActivityMessage.code", member: "runtime.wait-timeout", key: "runMessages.codes.runtime.wait-timeout", render: () => activityMessage("runtime.wait-timeout") },
  { property: "UpdaterAutoUpdatePolicy.code", member: "registry.unavailable", key: "images.errors.registry_unavailable", render: () => imageError("registry.unavailable") },
  { property: "ImageProblem.code", member: "image.pull-failed", key: "images.errors.image_pull-failed", render: () => imageError("image.pull-failed") },
  { property: "ImagePruneItem.code", member: "image.busy", key: "images.errors.image_busy", render: () => imageError("image.busy") },
  { property: "VulnerabilityProblem.code", member: "vulnerability.scan-timeout", key: "images.errors.vulnerability_scan-timeout", render: () => imageError("vulnerability.scan-timeout") },
  { property: "VulnerabilityScanResult.errorCode", member: "vulnerability.scan-timeout", key: "images.errors.vulnerability_scan-timeout", render: () => imageError("vulnerability.scan-timeout") },
  { property: "HistoryProblem.code", member: "history.unavailable", key: "images.errors.history_unavailable", render: () => imageError("history.unavailable") },
  { property: "PreferencesProblem.code", member: "preferences.invalid", key: "images.errors.preferences_invalid", render: () => imageError("preferences.invalid") },
  { property: "BackupProblem.code", member: "volume.backup.interrupted", key: "volumes.backupCodes.volume_backup_interrupted", render: () => backupErrorMessage(new AdapterOutcomeError({ kind: "error", code: "volume.backup.interrupted" }), de) },
  { property: "VolumeExistsError.code", member: "volume.exists", key: "volumes.errors.exists", render: () => volumeOrNetworkError("volume", "volume.exists") },
  { property: "BackupProblem.reason", member: "link", key: "volumes.uploadReasons.link", render: () => backupErrorMessage(new BackupError({ code: "volume.backup.upload-invalid", reason: "link" }), de) },
  { property: "VolumeRestoreRun.reason", member: "link", key: "volumes.uploadReasons.link", render: () => backupErrorMessage(new BackupError({ code: "volume.backup.upload-invalid", reason: "link" }), de) },
  { property: "VolumeHelperIncompatibleError.code", member: "volume.helper-incompatible", key: "volumes.errors.helper-incompatible", render: () => volumeOrNetworkError("volume", "volume.helper-incompatible") },
  { property: "VolumeHelperUnavailableError.code", member: "volume.helper-unavailable", key: "volumes.errors.helper-unavailable", render: () => volumeOrNetworkError("volume", "volume.helper-unavailable") },
  { property: "ContainerProblem.code", member: "container.shell-invalid", key: "runMessages.codes.container.shell-invalid", render: () => commandError("container.shell-invalid") },
  { property: "EnvironmentHostProblem.code", member: "host.name-invalid", key: "runMessages.codes.host.name-invalid", render: () => commandError("host.name-invalid") },
  { property: "UserAccessProblem.code", member: "access.admin-required", key: "containers.operationReasons.accessAdminRequired", render: () => commandError("access.admin-required") },
  { property: "NetworkError.code", member: "network.overlap", key: "networks.errors.overlap", render: () => volumeOrNetworkError("network", "overlap") },
  { property: "WidgetProblem.code", member: "widget.disabled", key: "dashboard.services.errors.disabled", render: () => de(`dashboard.services.${widgetErrorKey({ code: "widget.disabled" })}`) },
  { property: "NotificationProblem.code", member: "notification.config-invalid", key: "settings.codes.notification.config-invalid", render: () => settingsErrorMessage(new SettingsProblemError("notification.config-invalid", 422), de) },
  { property: "SettingsProblem.code", member: "settings.activity-limit-invalid", key: "settings.codes.settings.activity-limit-invalid", render: () => settingsErrorMessage(new SettingsProblemError("settings.activity-limit-invalid", 422), de) },
  { property: "StackimportProblem.code", member: "import.registry-not-configured", key: "stacks.import.errors.registry-not-configured", render: () => importProblemMessage("import.registry-not-configured", de) },
  { property: "AutohealProblem.code", member: "autoheal.settings-invalid", key: "settings.codes.autoheal.settings-invalid", render: () => settingsErrorMessage(new SettingsProblemError("autoheal.settings-invalid", 422), de) },
  { property: "NotificationTestResponse.code", member: "notification.test-failed", key: "settings.codes.notification.test-failed", render: () => notificationResult() },
  { property: "NotificationTestResponse.reason", member: "delivery-auth", key: "settings.codes.delivery-auth", render: () => notificationResult() },
  { property: "VolumeBackupEntry.status", member: "partial", key: "volumes.backups.status.partial", render: () => backupStatus("partial") },
  { property: "VolumeRestoreRun.status", member: "partial", key: "volumes.backups.status.partial", render: () => backupStatus("partial") },
  { property: "BackupTarget.kind", member: "local", key: `${BACKUP_KIND_KEY_PREFIX}.local`, render: () => backupTargets() },
  { property: "BackupSystemBackupMaintenance.phase", member: "draining", key: "hubBackups.maintenancePhase.draining", render: () => hubBackup(true) },
  { property: "BackupSystemBackupRun.status", member: "succeeded", key: "hubBackups.statusValues.succeeded", render: () => hubBackup(false) },
  { property: "BackupTargetHostDelivery.state", member: "conflict", key: "settings.backups.deliveryConflict", render: () => backupTargets() },
  { property: "EnvironmentAgentRestartRun.state", member: "reserved", key: "hosts.restart.phases.reserved", render: () => restartPhase() },
  { property: "EnvironmentAgentRestartRun.code", member: "host.busy", key: "runMessages.restartRefused", render: () => restartFailure("host.busy") },
  { property: "RequestProblem.code", member: "request.path-encoded", key: "runMessages.pathEncoded", render: () => commandError("request.path-encoded") },
  { property: "HistoryBucket.state", member: "unhealthy", key: "history.states.unhealthy", render: () => historyState() },
  { property: "VolumePruneFailed.code", member: "volume.remove-failed", key: "volumes.prune.failedReasons.remove-failed", render: () => prune(true) },
  { property: "VolumePrunePlanCandidate.reason", member: "unused-anonymous", key: "volumes.prune.candidateReasons.unused-anonymous", render: () => prune(false) },
  { property: "VolumePruneSkipped.reason", member: "changed", key: "volumes.prune.skipReasons.changed", render: () => prune(true) },
  { property: "VulnerabilityScanRun.status", member: "interrupted", key: "security.historyStatus.interrupted", render: () => scanStatus() },
  { property: "UpdaterSystemImageHint.state", member: "available", key: "common.stackUpdates.hint.available", render: () => systemHint() },
  { property: "UpdaterSystemImageHint.kind", member: "tools", key: "hosts.components.tools", render: () => systemHint() },
 ] as const;

it.each(proofs)("renders $property / $member through its real consumer in German", async (proof) => {
  const text = await proof.render();
  expect(text).toContain(de(proof.key));
  expect(text).not.toContain(en(proof.key));
  expect(text).not.toContain(proof.member);
  expect(text).not.toContain("English producer fallback");
});

it("has a real render proof for every distinct guarded contract union", () => {
  const schemas = JSON.parse(readFileSync(resolve(process.cwd(), "../api/openapi.json"), "utf8")).components.schemas;
  const members = (property: string): string[] => { const [schema, field] = property.split("."); return schemas[schema!].properties[field!].enum; };
  const signature = (property: string) => JSON.stringify(members(property).toSorted());
  const guarded = new Set(GUARDED_WIRE_ENUMS.flatMap((family) => family.properties.map(signature)));
  expect(new Set(proofs.map((proof) => signature(proof.property)))).toEqual(guarded);
  for (const proof of proofs) expect(members(proof.property)).toContain(proof.member);
});

it.each(["runtime.wait-timeout", "runtime.waiter-disconnected", "runtime.target-removed", "run.fresh"] as const)("never falls back to producer text for the catalogued message %s", (code) => {
  expect(activityMessage(code)).toBe(de(`runMessages.codes.${code}`));
});

it("renders a declared update reason through the actual lifecycle DTO consumer", () => {
  const run = mapActivityDetail({ activity: { id: "example", type: "project_update", environmentId: "example", status: "failed", startedAt: at, createdAt: at, error: "English producer fallback", imageLifecycle: { runId: "example", verified: false, volumeDataIncluded: false, code: "registry.unavailable" } }, messages: [], output: [], stepTrace: [], outputTruncated: false });
  expect(renderRun(run, "de").error).toBe(de("images.errors.registry_unavailable"));
});
