// Volumes and bind mounts as one storage model. Bind mounts are browsed with
// the same UI; `kind` keeps the difference visible.

import type { Retention } from "./retention";
import { nextOccurrence, type PlanSchedule } from "./schedule";

export type StorageKind = "volume" | "bind";

export interface VolumeConsumer {
  containerId: string;
  containerName: string;
  stackId?: string;
  service?: string;
  target: string;
  readOnly: boolean;
  running: boolean;
}

export type { Weekday } from "./schedule";
export type BackupSchedule = PlanSchedule;

export interface BackupPolicy {
  enabled: boolean;
  targetId: string;
  schedule: BackupSchedule;
  /** Archive count and/or total size kept on the target. */
  retention: Retention;
  /** Stop consumers during the archive for a consistent file state. */
  stopContainers: boolean;
}

export type BackupRunStatus = "running" | "succeeded" | "failed";

export interface BackupRun {
  id: string;
  volumeId: string;
  targetId: string;
  startedAt: string;
  finishedAt?: string;
  status: BackupRunStatus;
  trigger: "manual" | "scheduled";
  sizeBytes?: number;
  archive?: string;
  error?: string;
  actionId?: string;
  stoppedContainers: boolean;
}

export interface Volume {
  /** Stable id used in routes: vol-<host>-<docker name> or bind-<host>-<path slug>. */
  id: string;
  /** Docker volume name or the absolute host path of a bind mount. */
  name: string;
  hostId: string;
  kind: StorageKind;
  driver?: "local";
  /** Mountpoint on the host (volumes) or source path (bind mounts). */
  source: string;
  /** Key in compose.yaml when the volume belongs to a stack. */
  composeName?: string;
  stackId?: string;
  /** Bind mount of a single file instead of a directory. */
  fileBind?: boolean;
  sizeBytes: number;
  createdAt: string;
  consumers: VolumeConsumer[];
  labels: Record<string, string>;
  backup?: BackupPolicy;
  lastBackup?: BackupRun;
  /** Created in this demo session; can be deleted when unused. */
  demoCreated?: boolean;
}

export interface VolumeDetail extends Volume {
  backups: BackupRun[];
}

export interface ParsedMount {
  source: string;
  target: string;
  readOnly: boolean;
  kind: StorageKind;
}

/** Compose/docker short syntax "source:target[:mode]"; anonymous volumes return undefined. */
export function parseMount(spec: string): ParsedMount | undefined {
  const parts = spec.split(":");
  if (parts.length < 2) return undefined;
  const [source = "", target = "", mode = ""] = parts;
  if (!source || !target) return undefined;
  const kind: StorageKind = /^[./~]/.test(source) ? "bind" : "volume";
  return { source, target, readOnly: mode.split(",").includes("ro"), kind };
}

/** Relative bind sources resolve against the stack directory inside the project directory. */
export function resolveBindSource(source: string, stackDir: string | undefined): string {
  if (source.startsWith("./") && stackDir) return `${stackDir.replace(/\/+$/, "")}/${source.slice(2)}`;
  if (source.startsWith("~/")) return `/home/user/${source.slice(2)}`;
  return source.replace(/\/+$/, "") || "/";
}

export function dockerVolumeName(name: string, stackId?: string): string {
  return stackId ? `${stackId}_${name}` : name;
}

function pathSlug(path: string): string {
  return path.replace(/^\/+/, "").replace(/[^a-zA-Z0-9]+/g, "-").replace(/-+$/, "").toLowerCase() || "root";
}

export function volumeId(hostId: string, kind: StorageKind, key: string): string {
  return kind === "volume" ? `vol-${hostId}-${key}` : `bind-${hostId}-${pathSlug(key)}`;
}

export function isFileBind(path: string): boolean {
  return /\.[a-z0-9]{1,6}$/i.test(path.split("/").pop() ?? "");
}

export interface MountRef extends ParsedMount {
  id: string;
  /** Docker volume name or resolved host path. */
  name: string;
  composeName?: string;
}

/** Resolves a container mount to the storage entry that the volumes page shows. */
export function mountRef(container: { hostId: string; stackId?: string }, spec: string, projectDir?: string): MountRef | undefined {
  const m = parseMount(spec);
  if (!m) return undefined;
  if (m.kind === "volume") {
    const name = dockerVolumeName(m.source, container.stackId);
    return { ...m, id: volumeId(container.hostId, "volume", name), name, composeName: container.stackId ? m.source : undefined };
  }
  const stackDir = projectDir && container.stackId ? `${projectDir.replace(/\/+$/, "")}/${container.stackId}` : undefined;
  const path = resolveBindSource(m.source, stackDir);
  return { ...m, id: volumeId(container.hostId, "bind", path), name: path };
}

export interface MountUsage {
  id: string;
  name: string;
  kind: StorageKind;
  hostId: string;
  stackId?: string;
  composeName?: string;
  fileBind: boolean;
  consumers: VolumeConsumer[];
}

interface MountingContainer {
  id: string;
  name: string;
  hostId: string;
  stackId?: string;
  serviceName?: string;
  state: string;
  mounts: string[];
}

/** Groups container mounts into storage entries with their consumers. */
export function collectMountUsage(containers: readonly MountingContainer[], projectDirOf: (hostId: string) => string | undefined): MountUsage[] {
  const out = new Map<string, MountUsage>();
  for (const c of containers) {
    for (const spec of c.mounts) {
      const ref = mountRef(c, spec, projectDirOf(c.hostId));
      if (!ref) continue;
      let entry = out.get(ref.id);
      if (!entry) {
        entry = {
          id: ref.id,
          name: ref.name,
          kind: ref.kind,
          hostId: c.hostId,
          stackId: ref.kind === "volume" ? c.stackId : undefined,
          composeName: ref.composeName,
          fileBind: ref.kind === "bind" && isFileBind(ref.name),
          consumers: [],
        };
        out.set(ref.id, entry);
      }
      entry.consumers.push({ containerId: c.id, containerName: c.name, stackId: c.stackId, service: c.serviceName, target: ref.target, readOnly: ref.readOnly, running: c.state === "running" });
    }
  }
  return [...out.values()];
}

/** Image names whose data directories must not be edited file by file. */
export function holdsDatabase(images: readonly string[]): boolean {
  return images.some((i) => /postgres|mariadb|mysql|mongo|redis|influx|sqlite/i.test(i));
}

/** Short display name: compose key for stack volumes, path or Docker name otherwise. */
export function volumeLabel(v: Pick<Volume, "composeName" | "name">): string {
  return v.composeName ?? v.name;
}

/** Next scheduled run in the hub time zone; manual plans and past one-time runs have none. */
export function nextBackupRun(schedule: BackupSchedule, from: Date = new Date(), timeZone?: string): Date | undefined {
  return schedule.mode === "manual" ? undefined : nextOccurrence(schedule, from, timeZone);
}
