import { isHostAvailable } from "@/domain/hosts";
import { createDemoBackups } from "./demo-backups";
import { collectMountUsage, volumeId, type BackupRun, type MountUsage, type Volume, type VolumeDetail } from "@/domain/volumes";
import { planVolumePrune, pruneBlock } from "@/domain/volume-prune";
import { volumeName } from "@/domain/validation";

import type { VolumePruneResult, VolumesApi } from "../api";
import { DemoError, wait, type DemoContext } from "./demo-context";
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
  const { db } = ctx;
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
    if (!isHostAvailable(db.host(v.hostId))) throw new DemoError("host-offline");
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
      if (!isHostAvailable(db.host(input.hostId))) throw new DemoError("host-offline");
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
      if (!isHostAvailable(db.host(v.hostId))) throw new DemoError("host-offline");
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

    ...createDemoBackups(ctx, all, find),
  };
}

function latest(runs: BackupRun[]): BackupRun | undefined {
  return runs.reduce<BackupRun | undefined>((best, r) => (!best || r.startedAt > best.startedAt ? r : best), undefined);
}
