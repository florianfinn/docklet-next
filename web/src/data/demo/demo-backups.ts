// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original development-only backup model matching live target and restore contracts.

import type { VolumesApi } from "../api";
import { BackupError } from "../backup-error";
import type { BackupPolicy, BackupRun, RestoreRun, Volume } from "@/domain/volumes";
import { nextBackupRun, volumeId } from "@/domain/volumes";
import type { BackupTarget } from "@/domain/settings";
import { validateSchedule } from "@/domain/schedule";
import type { DemoContext } from "./demo-context";
import { volumeName } from "@/domain/validation";

type Operations = "setBackupPolicy" | "runBackup" | "restoreBackup" | "restoreBackupFiles" | "browseBackup" | "downloadBackup" | "deleteBackup" | "uploadRestore" | "getBackupOverview" | "listBackupOverviews" | "getBackupRepository" | "checkBackupRepository" | "listBackupTargets" | "createBackupTarget" | "updateBackupTarget" | "deleteBackupTarget" | "testBackupTarget" | "getBackupTargetReferences" | "getBackupRecoveryKey" | "attachLocalBackupRecoveryKey";
export function createDemoBackups(ctx: DemoContext, all: () => Volume[], find: (id: string) => Volume): Pick<VolumesApi, Operations> {
  const { db, runner } = ctx;
  const restores = new Map<string, RestoreRun>(), keys = new Map<string, string>();
  const now = () => new Date().toISOString();
  const fail = (code: ConstructorParameters<typeof BackupError>[0]["code"]): never => { throw new BackupError({ code }); };
  function named(id: string) { const v = find(id); if (v.kind !== "volume") return fail("volume.not-found"); return v; }
  function target(id: string) { const t = db.settings.backups.targets.find((t) => t.id === id); if (!t) return fail("backup.target.unknown"); return t; }
  function snapshot(id: string, runId: string) { const row = db.backupRuns.find((run) => run.id === runId && run.volumeId === id); if (!row || !["succeeded", "partial"].includes(row.status)) return fail("volume.backup.snapshot-not-found"); const key = `snapshot-${runId}`; if (!db.files.has(key)) db.files.clone(id, key); return key; }
  function start(host: string, kind: "backup" | "volume-restore", done: () => void, failure?: () => void) {
    return runner.start({ kind, hostId: host, title: kind === "backup" ? "Backup" : "Wiederherstellung", target: db.host(host)?.name ?? host, services: [], steps: [{ key: "validate", label: "Ziel prüfen" }, { key: "collect", label: "Daten erfassen" }, { key: "save", label: "Auf Ziel schreiben" }], failAt: ctx.forcedFailure() ? "save" : undefined, failMessage: "Backup fehlgeschlagen", onSuccess: () => { done(); db.changed("volumes", "settings", "schedule"); }, onFailure: () => { failure?.(); db.changed("volumes", "settings"); } });
  }
  function usable(host: string) { return [{ id: "local", name: "Local", kind: "local" as const }, ...db.settings.backups.targets.filter((t) => !t.retiredAt && (!t.hostId || t.hostId === host)).map(({ id, name, kind }) => ({ id, name, kind }))]; }
  const api: Pick<VolumesApi, Operations> = {
    async setBackupPolicy(id, policy) {
      const v = named(id);
      if (policy) {
        if (!usable(v.hostId).some((t) => t.id === policy.targetId)) return fail("backup.target.unknown");
        if (!Number.isInteger(policy.retentionCount) || policy.retentionCount < 0 || policy.retentionCount > 3650 || policy.schedule.mode !== "manual" && policy.schedule.mode !== "custom" && validateSchedule(policy.schedule, new Date(), db.settings.general.timeZone)) return fail("volume.backup.policy-invalid");
        db.backupPolicies.set(id, structuredClone(policy));
      } else db.backupPolicies.delete(id);
      db.changed("volumes", "schedule");
    },
    async runBackup(id, options = {}) {
      const v = named(id); const policy = db.backupPolicies.get(id);
      const targetId = options.targetId ?? policy?.targetId ?? "local";
      if (!usable(v.hostId).some((t) => t.id === targetId)) return fail("backup.target.unknown");
      const run: BackupRun = { id: db.nextId("b"), volumeId: id, targetId, startedAt: now(), trigger: "manual", status: "running", stoppedContainers: options.stopContainers ?? policy?.stopContainers ?? false };
      db.backupRuns.unshift(run);
      const activity = start(v.hostId, "backup", () => { run.status = "succeeded"; run.sizeBytes = db.files.size(id); run.archive = run.id; run.finishedAt = now(); db.files.clone(id, `snapshot-${run.id}`); }, () => { run.status = "failed"; run.errorCode = "volume.backup.repository-unavailable"; });
      run.actionId = activity; db.changed("volumes"); return activity;
    },
    async restoreBackup(id, runId, mode, newVolumeName) {
      const v = named(id), source = snapshot(id, runId);
      if (mode === "new" && (!newVolumeName || volumeName(newVolumeName, all().filter((row) => row.hostId === v.hostId).map((row) => row.name)))) return fail("volume.exists");
      let safety: BackupRun | undefined;
      if (mode === "replace") { safety = { id: db.nextId("safety"), volumeId: id, targetId: "local", startedAt: now(), status: "succeeded", trigger: "safety", archive: "safety", sizeBytes: db.files.size(id), stoppedContainers: true }; db.backupRuns.unshift(safety); db.files.clone(id, `snapshot-${safety.id}`); }
      const restore: RestoreRun = { restoreId: db.nextId("restore"), activityId: "", at: now(), mode, newVolumeName, kind: "full", phase: "preparing", status: "running", pendingRestart: false, safetyBackupId: safety?.id, stoppedContainers: mode === "new" ? [] : v.consumers.filter((c) => c.running).map((c) => ({ id: c.containerId, name: c.containerName })) };
      restores.set(id, restore);
      restore.activityId = start(v.hostId, "volume-restore", () => {
        const dest = mode === "new" ? volumeId(v.hostId, "volume", newVolumeName!) : id;
        if (mode === "new") db.createdVolumes.set(dest, { name: newVolumeName!, hostId: v.hostId, createdAt: now(), labels: { "docklet.restore-id": restore.restoreId } });
        db.files.clone(source, dest); restore.status = "succeeded"; restore.phase = "done"; restore.pendingRestart = false;
      }, () => { restore.phase = "writing"; restore.status = "failed"; restore.code = "volume.restore.failed-partial"; restore.pendingRestart = mode === "replace"; });
      db.changed("volumes"); return restore.activityId;
    },
    async restoreBackupFiles(id, runId, selection) {
      const v = named(id), source = snapshot(id, runId), paths = selection.paths ?? [];
      return start(v.hostId, "volume-restore", () => {
        const src = db.files.ensure(source, () => []), dest = db.files.ensure(id, () => []);
        for (const selected of paths) { for (const path of dest.keys()) if (path === selected || path.startsWith(`${selected}/`)) dest.delete(path); for (const [path, node] of src) if (path === selected || path.startsWith(`${selected}/`)) dest.set(path, { ...node }); }
      });
    },
    async browseBackup(id, runId, path = "/", start = 0) { const entries = db.files.list(snapshot(id, runId), path).entries; if (entries.length > 5000) return fail("volume.backup.listing-too-large"); return { success: true, data: entries.slice(start, start + 100).map((e) => ({ name: e.name, path: e.path, isDirectory: e.type === "dir" })), pagination: { totalItems: entries.length, currentPage: Math.floor(start / 100) + 1, itemsPerPage: 100, totalPages: Math.ceil(entries.length / 100) } }; },
    async downloadBackup(id, runId) { snapshot(id, runId); return new Blob(["Development snapshot export"], { type: "application/gzip" }); },
    async deleteBackup(id, runId) { named(id); if (restores.get(id)?.pendingRestart && restores.get(id)?.safetyBackupId === runId) return fail("volume.backup.safety-pinned"); db.backupRuns = db.backupRuns.filter((run) => run.id !== runId || run.volumeId !== id); db.files.drop(`snapshot-${runId}`); db.changed("volumes"); },
    async uploadRestore(id, file, mode = "replace", name) { if (file.size > 4 * 1024 ** 3) return fail("volume.backup.upload-too-large"); if (!/\.(tar\.gz|tgz)$/.test(file.name)) return fail("volume.backup.upload-invalid"); const v = named(id); const run: BackupRun = { id: db.nextId("upload"), volumeId: id, targetId: "local", startedAt: now(), status: "succeeded", trigger: "manual", archive: file.name, stoppedContainers: false }; db.backupRuns.unshift(run); return api.restoreBackup(v.id, run.id, mode, name); },
    async getBackupOverview(host) {
      const online = db.host(host)?.status === "online";
      return { hostId: host, unavailable: !online, authority: "development", setVersion: 1, timeZone: db.settings.general.timeZone, repository: { available: online, mode: "volume" }, targets: usable(host), volumes: all().filter((v) => v.kind === "volume" && v.hostId === host).map((v) => ({ ...v, timeZone: db.settings.general.timeZone, backupUnavailable: !online, volumeExists: true, backup: v.backup ? { ...v.backup, nextRunAt: v.backup.enabled ? nextBackupRun(v.backup.schedule, new Date(), db.settings.general.timeZone)?.toISOString() : undefined } : undefined, lastRestore: restores.get(v.id), running: restores.get(v.id)?.status === "running" ? { kind: "restore", activityId: restores.get(v.id)!.activityId } : v.lastBackup?.status === "running" ? { kind: "backup", activityId: v.lastBackup.actionId! } : undefined })) };
    },
    async listBackupOverviews(onPartial) { const rows = await Promise.all(db.hosts.map((host) => api.getBackupOverview(host.id))); onPartial?.(rows); return rows; },
    async getBackupRepository() { return { mode: "volume", name: "arcane-backups", engineImage: "ghcr.io/rustic-rs/rustic:v0.11.2" }; },
    async checkBackupRepository(host) { return start(host, "backup", () => {}); },
    async listBackupTargets() { return structuredClone(db.settings.backups.targets); },
    async createBackupTarget(input) { const row: BackupTarget = { ...input, id: db.nextId("target"), createdAt: now(), configVersion: 1, checks: [], hasSecret: !!input.secret }; delete (row as Record<string, unknown>).secret; delete (row as Record<string, unknown>).recoveryKey; db.settings.backups.targets.push(row); keys.set(row.id, input.recoveryKey ?? "AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA"); db.changed("settings"); return structuredClone(row); },
    async updateBackupTarget(id, input) { const row = target(id); if (["kind", "path", "hostId", "server", "share"].some((key) => row[key as keyof BackupTarget] !== input[key as keyof typeof input])) return fail("backup.target.immutable"); Object.assign(row, { name: input.name, username: input.username, options: input.options, hasSecret: row.hasSecret || !!input.secret, configVersion: row.configVersion + 1 }); db.changed("settings"); return structuredClone(row); },
    async deleteBackupTarget(id) { if (db.backupRuns.some((r) => r.targetId === id) || [...db.backupPolicies.values()].some((p) => p.targetId === id)) return fail("backup.target.in-use"); if (db.hosts.some((h) => h.status !== "online" && (!target(id).hostId || h.id === target(id).hostId))) return fail("backup.target.hosts-unknown"); target(id).retiredAt = now(); db.changed("settings"); },
    async testBackupTarget(id, host) { const row = target(id); return start(host, "backup", () => { row.checks = [...(row.checks ?? []).filter((c) => c.hostId !== host), { hostId: host, activityId: "test", at: now(), configVersion: row.configVersion, setVersion: 1, earlierSettings: false, ok: true }]; }); },
    async getBackupTargetReferences(id, host) { return { backups: db.backupRuns.filter((r) => r.targetId === id && all().some((v) => v.id === r.volumeId && v.hostId === host)).map((r) => r.volumeId), policies: [...db.backupPolicies].filter(([, p]) => p.targetId === id).map(([id]) => id), running: [] }; },
    async getBackupRecoveryKey(id, host) { return keys.get(id === "local" ? `local-${host}` : id) ?? "AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA-AAAAAA"; },
    async attachLocalBackupRecoveryKey(host, key) { keys.set(`local-${host}`, key); },
  };
  return api;
}

export function validDemoBackupPolicy(policy: BackupPolicy) { return Number.isInteger(policy.retentionCount) && policy.retentionCount >= 0 && policy.retentionCount <= 3650; }
