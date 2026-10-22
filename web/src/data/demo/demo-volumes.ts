import { collectMountUsage, holdsDatabase, volumeId, type BackupRun, type MountUsage, type Volume, type VolumeDetail } from "@/domain/volumes";
import { applyRetention, validateRetention } from "@/domain/retention";
import { validateSchedule } from "@/domain/schedule";
import { planVolumePrune, pruneBlock } from "@/domain/volume-prune";
import { volumeName } from "@/domain/validation";

import type { VolumePruneResult, VolumesApi } from "../api";
import type { StepPlan } from "./action-runner";
import { DemoError, failMessage, wait, type DemoContext } from "./demo-context";
import { FileStoreError } from "./file-store";
import { UNUSED_VOLUMES } from "./fixtures/unused-volumes";
import { seedFiles } from "./fixtures/volume-files";
import { simulateRead } from "./simulation";

function hash(s: string): number {
  let h = 7;
  for (const ch of s) h = (h * 31 + ch.charCodeAt(0)) % 100_003;
  return h;
}

const DAY = 86_400_000;

export function createVolumesApi(ctx: DemoContext): VolumesApi {
  const { db, runner } = ctx;
  const usageOf = (): MountUsage[] => collectMountUsage(db.containers(), (h) => db.host(h)?.projectDir);
  for (const u of UNUSED_VOLUMES) {
    const id = volumeId(u.hostId, "volume", u.name);
    if (db.createdVolumes.has(id)) continue;
    db.createdVolumes.set(id, { name: u.name, hostId: u.hostId, createdAt: new Date(Date.now() - u.ageDays * DAY).toISOString(), labels: u.labels, seeded: true });
    db.files.ensure(id, () => u.files);
  }

  function toVolume(u: MountUsage): Volume {
    db.files.ensure(u.id, () => seedFiles(u));
    const runs = db.backupRuns.filter((r) => r.volumeId === u.id);
    return {
      id: u.id,
      name: u.name,
      hostId: u.hostId,
      kind: u.kind,
      driver: u.kind === "volume" ? "local" : undefined,
      source: u.kind === "volume" ? `/var/lib/docker/volumes/${u.name}/_data` : u.name,
      composeName: u.composeName,
      stackId: u.stackId,
      fileBind: u.fileBind,
      sizeBytes: db.files.size(u.id),
      createdAt: new Date(Date.now() - (20 + (hash(u.id) % 300)) * DAY).toISOString(),
      consumers: u.consumers,
      labels: u.stackId && u.composeName ? { "com.docker.compose.project": u.stackId, "com.docker.compose.volume": u.composeName } : {},
      backup: db.backupPolicies.get(u.id),
      lastBackup: latest(runs),
    };
  }

  function all(): Volume[] {
    const derived = usageOf().map(toVolume);
    const mounted = new Map(derived.map((v) => [v.id, v]));
    const created = [...db.createdVolumes].map(([id, v]): Volume => {
      db.files.ensure(id, () => []);
      // A known volume that a container mounts meanwhile keeps its consumers.
      const used = mounted.get(id);
      return {
        id,
        name: v.name,
        hostId: v.hostId,
        kind: "volume",
        driver: "local",
        source: `/var/lib/docker/volumes/${v.name}/_data`,
        composeName: used?.composeName,
        stackId: used?.stackId,
        sizeBytes: db.files.size(id),
        createdAt: v.createdAt,
        consumers: used?.consumers ?? [],
        labels: { ...used?.labels, ...v.labels },
        backup: db.backupPolicies.get(id),
        lastBackup: latest(db.backupRuns.filter((r) => r.volumeId === id)),
        demoCreated: v.seeded ? undefined : true,
      };
    });
    return [...derived.filter((v) => !db.createdVolumes.has(v.id)), ...created];
  }

  function find(id: string): Volume {
    const v = all().find((x) => x.id === id);
    if (!v) throw new DemoError("not-found");
    return v;
  }

  /** File access goes through the host agent; disconnected hosts cannot be browsed. */
  function online(v: Volume): Volume {
    if (db.host(v.hostId)?.status !== "online") throw new DemoError("host-offline");
    return v;
  }

  async function fileOp<T>(id: string, fn: () => T, mutates = true): Promise<T> {
    await wait(180);
    online(find(id));
    try {
      const out = fn();
      if (mutates) db.changed("volumes");
      return out;
    } catch (e) {
      if (e instanceof FileStoreError) throw new DemoError(e.code);
      throw e;
    }
  }

  function consumerImages(v: Volume): string[] {
    return db
      .containers()
      .filter((c) => v.consumers.some((x) => x.containerId === c.id))
      .map((c) => c.image.repository);
  }

  function consumerNames(v: Volume) {
    return v.consumers.filter((c) => c.running).map((c) => c.containerName);
  }

  return {
    listVolumes: () => simulateRead("volumes", () => (db.hosts.length ? structuredClone(all()) : [])),

    getVolume: (id) =>
      simulateRead("volumes", (): VolumeDetail => {
        const v = find(id);
        const backups = db.backupRuns.filter((r) => r.volumeId === id).toSorted((a, b) => b.startedAt.localeCompare(a.startedAt));
        return structuredClone({ ...v, backups });
      }),

    async createVolume(input) {
      await wait(300);
      if (db.host(input.hostId)?.status !== "online") throw new DemoError("host-offline");
      const taken = all()
        .filter((v) => v.hostId === input.hostId && v.kind === "volume")
        .map((v) => v.name);
      const invalid = volumeName(input.name, taken);
      if (invalid === "duplicate") throw new DemoError("exists");
      if (invalid) throw new DemoError("invalid-name");
      const id = volumeId(input.hostId, "volume", input.name);
      db.createdVolumes.set(id, { name: input.name, hostId: input.hostId, createdAt: new Date().toISOString(), labels: input.labels });
      db.files.ensure(id, () => []);
      db.pushEvent({ kind: "config", level: "info", title: `Volume ${input.name} angelegt`, hostId: input.hostId });
      db.changed("volumes");
      return structuredClone(find(id));
    },

    async renameVolume(id, name) {
      const v = online(find(id));
      if (v.kind !== "volume" || v.consumers.length) throw new DemoError("in-use");
      if (volumeName(name, all().filter((row) => row.hostId === v.hostId).map((row) => row.name))) throw new DemoError("invalid-name");
      const nextId = volumeId(v.hostId, "volume", name);
      db.files.clone(id, nextId);
      db.createdVolumes.set(nextId, { name, hostId: v.hostId, createdAt: new Date().toISOString(), labels: v.labels });
      db.createdVolumes.delete(id);
      db.files.drop(id);
      db.changed("volumes");
      return structuredClone(find(nextId));
    },

    async deleteVolume(id) {
      await wait(300);
      const v = find(id);
      if (v.kind === "bind") throw new DemoError("bind-not-deletable");
      if (v.consumers.length) throw new DemoError("in-use");
      if (db.host(v.hostId)?.status !== "online") throw new DemoError("host-offline");
      db.createdVolumes.delete(id);
      db.files.drop(id);
      db.backupPolicies.delete(id);
      db.pushEvent({ kind: "config", level: "info", title: `Volume ${v.name} gelöscht`, hostId: v.hostId });
      db.changed("volumes");
    },

    planVolumePrune: ({ hostIds, all: includeNamed = true }) => simulateRead("volumes", () => {
      const plan = planVolumePrune(all(), db.hosts, hostIds);
      if (!includeNamed) plan.candidates = plan.candidates.filter((v) => v.labels["com.docker.volume.anonymous"] !== undefined);
      return structuredClone({ ...plan, all: includeNamed });
    }),

    async pruneVolumes({ hostIds, volumeIds, candidates }) {
      await wait(400);
      // One synchronous pass after the wait: each confirmed volume is checked against the current state.
      const current = new Map(all().map((v) => [v.id, v]));
      const removed: { id: string; name: string; hostId: string; sizeBytes: number }[] = [];
      const skipped: VolumePruneResult["skipped"] = [];
      for (const id of volumeIds) {
        const v = current.get(id);
        const preview = candidates?.find((c) => c.id === id);
        const reason = !v || !hostIds.includes(v.hostId) ? "not-found" : preview && preview.createdAt !== v.createdAt ? "changed" : pruneBlock(v, db.host(v.hostId));
        if (reason || !v) {
          skipped.push({ id, reason: reason ?? "not-found" });
          continue;
        }
        db.createdVolumes.delete(id);
        db.files.drop(id);
        db.backupPolicies.delete(id);
        removed.push({ id, name: v.name, hostId: v.hostId, sizeBytes: v.sizeBytes });
      }
      if (removed.length) {
        const hosts = [...new Set(removed.map((r) => r.hostId))];
        db.pushEvent({ kind: "config", level: "ok", title: `${removed.length} ungenutzte Volumes entfernt (${hosts.map((h) => db.host(h)?.name ?? h).join(", ")})`, hostId: hosts.length === 1 ? hosts[0] : undefined });
        db.changed("volumes", "schedule");
      }
      return { removed, skipped, failed: [], spaceReclaimed: removed.reduce((total, v) => total + v.sizeBytes, 0) };
    },

    listFiles: async (id, path) => {
      const listing = await simulateRead("volumes", () => {
        online(find(id));
        try {
          return db.files.list(id, path);
        } catch (e) {
          if (e instanceof FileStoreError) throw new DemoError(e.code);
          throw e;
        }
      }, 0.3);
      return structuredClone(listing);
    },
    readFile: async (id, path) => structuredClone(await fileOp(id, () => db.files.read(id, path), false)),
    writeFile: (id, path, content) => fileOp(id, () => db.files.write(id, path, content)),
    createEntry: (id, dir, name, type) => fileOp(id, () => db.files.create(id, dir, name.trim(), type)),
    renameEntry: (id, path, name) => fileOp(id, () => db.files.rename(id, path, name.trim())),
    deleteEntry: (id, path) => fileOp(id, () => db.files.remove(id, path)),
    async uploadFile(id, dir, file) {
      const text = file.content ?? (file.blob && file.size <= 256 * 1024 ? await file.blob.text() : undefined);
      await fileOp(id, () => db.files.create(id, dir, file.name, "file", text ?? "", text === undefined ? file.size : undefined));
    },
    async downloadFile(id, path) {
      const file = await fileOp(id, () => db.files.read(id, path), false);
      if (!file.editable) throw new DemoError("not-editable");
      return new Blob([file.content], { type: "text/plain" });
    },

    async setBackupPolicy(id, policy) {
      await wait(300);
      find(id);
      if (policy && !db.settings.backups.targets.some((t) => t.id === policy.targetId)) throw new DemoError("target-missing");
      // The adapter checks the plan itself instead of trusting the form.
      if (policy && (validateRetention(policy.retention) || (policy.schedule.mode !== "manual" && validateSchedule(policy.schedule, new Date(), db.settings.general.timeZone)))) throw new DemoError("invalid-plan");
      if (policy) db.backupPolicies.set(id, structuredClone(policy));
      else db.backupPolicies.delete(id);
      db.changed("volumes", "schedule");
    },

    async runBackup(id) {
      const v = online(find(id));
      const policy = db.backupPolicies.get(id);
      if (!policy) throw new DemoError("no-policy");
      const target = db.settings.backups.targets.find((t) => t.id === policy.targetId);
      if (!target) throw new DemoError("target-missing");
      const stop = policy.stopContainers ? consumerNames(v) : [];
      const archive = `${v.composeName ?? v.name.split("/").pop()}-${new Date().toISOString().slice(0, 16).replace(/[:T]/g, "")}.tar.zst`;
      const steps: StepPlan[] = [
        { key: "validate", label: "Ziel prüfen", detail: target.name },
        ...(stop.length ? [{ key: "stop" as const, label: "Container stoppen", detail: stop.join(", ") }] : []),
        { key: "collect", label: "Archiv erstellen", detail: `${v.kind === "bind" ? "Bind-Mount" : "Volume"} ${v.name}` },
        { key: "save", label: "Auf Ziel schreiben", detail: archive },
        ...(stop.length ? [{ key: "start" as const, label: "Container starten", detail: stop.join(", ") }] : []),
      ];
      const unreachable = target.lastTest?.ok === false;
      const forced = ctx.forcedFailure();
      const runId = db.nextId("b");
      const record: BackupRun = { id: runId, volumeId: id, targetId: target.id, startedAt: new Date().toISOString(), status: "running", trigger: "manual", stoppedContainers: stop.length > 0 };
      db.backupRuns = [record, ...db.backupRuns];
      const patch = (fn: (r: BackupRun) => BackupRun) => {
        db.backupRuns = db.backupRuns.map((r) => (r.id === runId ? fn(r) : r));
        db.changed("volumes");
      };
      const actionId = runner.start({
        kind: "backup",
        title: `Backup · ${v.composeName ?? v.name}`,
        target: `${target.name} · ${db.host(v.hostId)?.name ?? v.hostId}`,
        hostId: v.hostId,
        stackId: v.stackId,
        services: stop,
        steps,
        failAt: unreachable ? "validate" : forced ? "save" : undefined,
        failMessage: unreachable ? `${target.name} nicht erreichbar (letzter Test: ${target.lastTest?.message})` : failMessage,
        onSuccess: () => {
          patch((r) => ({ ...r, status: "succeeded", finishedAt: new Date().toISOString(), sizeBytes: Math.round(db.files.size(id) * (holdsDatabase(consumerImages(v)) ? 0.32 : 0.6)), archive }));
          const done = db.backupRuns.filter((r) => r.volumeId === id && r.status === "succeeded").toSorted((a, b) => b.startedAt.localeCompare(a.startedAt));
          const drop = new Set(applyRetention(done, policy.retention).drop.map((r) => r.id));
          if (drop.size) db.backupRuns = db.backupRuns.filter((r) => !drop.has(r.id));
          db.pushEvent({ kind: "backup", level: "ok", title: `Backup von ${v.composeName ?? v.name} abgeschlossen${drop.size ? ` · ${drop.size} ältere(s) Archiv(e) gemäß Aufbewahrung entfernt` : ""}`, hostId: v.hostId, stackId: v.stackId, actionId });
          db.changed("volumes", "events");
        },
        onFailure: (run) => {
          patch((r) => ({ ...r, status: "failed", finishedAt: new Date().toISOString(), error: run.error }));
          db.pushEvent({ kind: "backup", level: "error", title: `Backup von ${v.composeName ?? v.name} fehlgeschlagen`, hostId: v.hostId, stackId: v.stackId, actionId: run.id });
        },
      });
      patch((r) => ({ ...r, actionId }));
      return actionId;
    },

    async restoreBackup(id, runId, mode) {
      const v = online(find(id));
      const run = db.backupRuns.find((r) => r.id === runId && r.volumeId === id && r.status === "succeeded");
      if (!run) throw new DemoError("not-found");
      const stop = mode === "replace" ? consumerNames(v) : [];
      const newName = `${v.name}-restore-${new Date().toISOString().slice(0, 10).replace(/-/g, "")}`;
      return runner.start({
        kind: "volume-restore",
        title: `Wiederherstellung · ${v.composeName ?? v.name}`,
        target: run.archive ?? runId,
        hostId: v.hostId,
        stackId: v.stackId,
        services: stop,
        steps: [
          { key: "validate", label: "Archiv prüfen", detail: run.archive },
          ...(stop.length ? [{ key: "stop" as const, label: "Container stoppen", detail: stop.join(", ") }] : []),
          { key: "create", label: mode === "replace" ? "Inhalt ersetzen" : "Neues Volume anlegen", detail: mode === "replace" ? v.name : newName },
          ...(stop.length ? [{ key: "start" as const, label: "Container starten", detail: stop.join(", ") }] : []),
        ],
        failAt: ctx.forcedFailure() ? "create" : undefined,
        failMessage,
        onSuccess: () => {
          if (mode === "new" && v.kind === "volume") {
            const nid = volumeId(v.hostId, "volume", newName);
            db.createdVolumes.set(nid, { name: newName, hostId: v.hostId, createdAt: new Date().toISOString(), labels: { "docklet.restore-of": v.name } });
            db.files.clone(id, nid);
          }
          db.pushEvent({ kind: "backup", level: "ok", title: `Archiv ${run.archive} wiederhergestellt${mode === "new" ? ` als ${newName}` : ""}`, hostId: v.hostId, stackId: v.stackId });
          db.changed("volumes", "events");
        },
      });
    },
  };
}

function latest(runs: BackupRun[]): BackupRun | undefined {
  return runs.reduce<BackupRun | undefined>((best, r) => (!best || r.startedAt > best.startedAt ? r : best), undefined);
}
