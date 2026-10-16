// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original synthetic Hub backup scenarios for issue #32.

import type { HubBackupApi, HubBackupOperation, HubBackupPolicy, HubBackupRun } from "@/domain/hub-backup";
import { MAX_HUB_BACKUP_BYTES } from "@/domain/hub-backup";

import { AdapterOutcomeError } from "../capability";
import type { DemoAdapterOptions } from "./options";

const CREATED = "2026-10-14T09:30:00Z";
const sample: HubBackupRun = {
  id: "system-demo-1", activityId: "activity-demo-1", archiveName: "system-demo-1.dump",
  createdAt: CREATED, completedAt: CREATED, size: 33_554_432, status: "succeeded",
  trigger: "manual", copyState: "none",
};

function denied(): never {
  throw new AdapterOutcomeError({ kind: "error", code: "backup.permission_denied", status: 403 });
}

function invalid(code: string): never {
  throw new AdapterOutcomeError({ kind: "error", code, status: 422 });
}

export function createDemoHubBackups(options: DemoAdapterOptions = {}): HubBackupApi {
  const scenario = options.hubBackupScenario ?? "list";
  let signedOut = scenario === "signed-out";
  let maintenance = scenario === "maintenance";
  let phase: "draining" | "applying" | "verifying" = "draining";
  const runs: HubBackupRun[] = scenario === "empty" ? [] : [{ ...sample, status: scenario === "creating" || scenario === "queued" ? "running" : scenario === "cancelled" ? "failed" : "succeeded" }];
  const states = new Map<string, string>([["activity-demo-1", scenario === "queued" ? "queued" : scenario === "cancelled" ? "cancelled" : scenario === "creating" ? "running" : "success"]]);
  let policies: HubBackupPolicy[] = [{ id: "hub-daily", enabled: true, schedule: "0 0 3 * * *", retentionCount: 7 }];
  const access = () => !signedOut && scenario !== "no-access";
  const requireAccess = () => { if (!access()) denied(); };
  const requireStorage = () => {
    if (scenario === "storage-unavailable") invalid("backup.storage_unavailable");
    if (scenario === "tools-unavailable") invalid("backup.tools_unavailable");
  };

  return {
    async getHubBackupAccess() { return { read: access(), manage: access(), restore: access() }; },
    async listHubBackups() { requireAccess(); return structuredClone(runs); },
    async getHubBackupPolicies() { requireAccess(); return structuredClone(policies); },
    async saveHubBackupPolicies(next) {
      requireAccess(); requireStorage();
      if (next.some((entry) => entry.retentionCount < 1 || !entry.schedule.trim())) invalid("backup.invalid_policy");
      policies = structuredClone(next);
      return structuredClone(policies);
    },
    async getHubBackupStorage() {
      requireAccess();
      const reasonCode = scenario === "storage-unavailable" ? "backup.storage_unavailable" : scenario === "tools-unavailable" ? "backup.tools_unavailable" : undefined;
      return { available: !reasonCode, freeBytes: reasonCode ? 0 : 9_663_676_416, usedBytes: runs.filter((run) => run.status === "succeeded").reduce((sum, run) => sum + run.size, 0), maxArchiveBytes: MAX_HUB_BACKUP_BYTES, ...(reasonCode ? { reasonCode } : {}) };
    },
    async getHubBackupMaintenance() { return { active: maintenance, ...(maintenance ? { phase, activityId: "activity-restore-1" } : {}) }; },
    async getHubBackupActivityStatus(id) { requireAccess(); return states.get(id) ?? "failed"; },
    async createHubBackup() {
      requireAccess(); requireStorage();
      const id = `system-demo-${runs.length + 1}`;
      const activityId = `activity-demo-${runs.length + 1}`;
      const run: HubBackupRun = { id, activityId, createdAt: new Date().toISOString(), size: 0, status: "running", trigger: "manual", copyState: "none" };
      runs.unshift(run); states.set(activityId, "running");
      setTimeout(() => { run.status = "succeeded"; run.size = 18_432_000; run.completedAt = new Date().toISOString(); states.set(activityId, "success"); }, 1300);
      return structuredClone(run);
    },
    async uploadHubBackup(file, onProgress): Promise<HubBackupOperation> {
      requireAccess(); requireStorage();
      if (file.size > MAX_HUB_BACKUP_BYTES) invalid("backup.size_limit");
      if (file.type && file.type !== "application/octet-stream") invalid("backup.unsupported_media_type");
      onProgress(45);
      await new Promise((resolve) => setTimeout(resolve, 350));
      const id = `system-upload-${runs.length + 1}`;
      const activityId = `activity-upload-${runs.length + 1}`;
      runs.unshift({ id, activityId, createdAt: new Date().toISOString(), completedAt: new Date().toISOString(), size: file.size, status: "succeeded", trigger: "uploaded", copyState: "none" });
      states.set(activityId, "success"); onProgress(100);
      return { backupId: id, activityId, status: "success" };
    },
    async restoreHubBackup(id) {
      requireAccess(); requireStorage();
      if (!runs.some((run) => run.id === id && run.status === "succeeded")) invalid("backup.not_found");
      maintenance = true; phase = "draining";
      states.set("activity-restore-1", "running");
      setTimeout(() => { phase = "applying"; }, 900);
      setTimeout(() => { phase = "verifying"; }, 1800);
      setTimeout(() => { maintenance = false; signedOut = true; states.set("activity-restore-1", "success"); }, 2900);
      return { backupId: id, activityId: "activity-restore-1", status: "running" };
    },
    async deleteHubBackup(id) {
      requireAccess(); requireStorage();
      const index = runs.findIndex((run) => run.id === id);
      if (index === -1) invalid("backup.not_found");
      runs.splice(index, 1);
      const activityId = `activity-delete-${id}`;
      states.set(activityId, "success");
      return { backupId: id, activityId, status: "success" };
    },
    async downloadHubBackup(id) {
      requireAccess();
      if (!runs.some((run) => run.id === id && run.status === "succeeded")) invalid("backup.not_found");
      const blob = new Blob(["PGDMPdemo"], { type: "application/octet-stream" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url; link.download = `${id}.dump`; document.body.append(link); link.click(); link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 0);
    },
  };
}
