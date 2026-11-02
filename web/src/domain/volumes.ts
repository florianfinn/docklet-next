// Volumes and bind mounts as one storage model. Bind mounts are browsed with
// the same UI; `kind` keeps the difference visible.

import type { InventoryObservation } from "./model";
import type { components } from "@/api/generated/schema";
import { nextOccurrence, type PlanSchedule } from "./schedule";

export type StorageKind = "volume" | "bind";

export interface VolumeConsumer {
  containerId: string;
  containerName: string;
  stackId?: string;
  stackName?: string;
  service?: string;
  target: string;
  readOnly: boolean;
  running: boolean;
}

export type { Weekday } from "./schedule";
/** A manual plan keeps its stored cron, so switching back or saving never replaces it. */
export type BackupSchedule = Exclude<PlanSchedule, { mode: "once" | "manual" }> | { mode: "manual"; cron?: string } | { mode: "custom"; cron: string };

export interface BackupPolicy {
  enabled: boolean;
  targetId: string;
  schedule: BackupSchedule;
  /** Zero keeps all scheduled snapshots; manual snapshots are kept until deleted. */
  retentionCount: number;
  id?: string;
  nextRunAt?: string;
  /** Stop consumers during the archive for a consistent file state. */
  stopContainers: boolean;
}

export type BackupRunStatus = "running" | "succeeded" | "failed" | "partial" | "unavailable";

export interface BackupRun {
  id: string;
  volumeId: string;
  targetId: string;
  startedAt: string;
  finishedAt?: string;
  status: BackupRunStatus;
  trigger: "manual" | "scheduled" | "safety";
  sizeBytes?: number;
  archive?: string;
  errorCode?: string;
  actionId?: string;
  stoppedContainers: boolean;
}

/** Agent view of a bind source: kind, path-rule verdict and the latest known sizes. */
export interface BindInfo {
  kind: "directory" | "file" | "unknown";
  eligible: boolean;
  /** Refusal code of the first failing path rule, e.g. `volume.bind.path-denied`. */
  reason?: string;
  measuredSize?: number;
  measuredAt?: string;
  measuredPartial?: boolean;
  lastSnapshotSize?: number;
  lastSnapshotAt?: string;
}

/** Backend bind identity: `bind:` plus the lowercase hex UTF-8 bytes of the source path. */
export function bindKey(path: string): string {
  return `bind:${Array.from(new TextEncoder().encode(path), (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

/** Source path of a bind key, or undefined for volume names and malformed keys. */
export function bindPath(key: string): string | undefined {
  const hex = key.startsWith("bind:") ? key.slice(5) : "";
  if (!hex || hex.length % 2 || !/^[0-9a-f]+$/.test(hex)) return undefined;
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(Uint8Array.from(hex.match(/../g)!, (pair) => parseInt(pair, 16)));
  } catch {
    return undefined;
  }
}

/** Newest known size of a bind: an on-demand scan or the latest snapshot, whichever is newer. */
export function bindSize(info: BindInfo | undefined): { bytes: number; at: string; source: "measured" | "snapshot"; partial: boolean } | undefined {
  const measured = info?.measuredSize != null && info.measuredAt ? { bytes: info.measuredSize, at: info.measuredAt, source: "measured" as const, partial: !!info.measuredPartial } : undefined;
  const snapshot = info?.lastSnapshotSize != null && info.lastSnapshotAt ? { bytes: info.lastSnapshotSize, at: info.lastSnapshotAt, source: "snapshot" as const, partial: false } : undefined;
  if (!measured || !snapshot) return measured ?? snapshot;
  return Date.parse(measured.at) >= Date.parse(snapshot.at) ? measured : snapshot;
}

/** Whether files and backups are offered: binds need an eligible agent verdict. */
export function storageUsable(v: Pick<Volume, "kind" | "bind">): boolean {
  return v.kind === "volume" || !!v.bind?.eligible;
}

export interface Volume {
  inventory?: InventoryObservation;
  sizeUnavailable?: boolean;
  /** Stable id used in routes: vol-<host>-<docker name> or bind-<host>-<path slug>. */
  id: string;
  /** Docker volume name or the absolute host path of a bind mount. */
  name: string;
  hostId: string;
  kind: StorageKind;
  driver?: string;
  /** Mountpoint on the host (volumes) or source path (bind mounts). */
  source: string;
  /** Key in compose.yaml when the volume belongs to a stack. */
  composeName?: string;
  stackId?: string;
  stackName?: string;
  /** Bind mount of a single file instead of a directory. */
  fileBind?: boolean;
  /** Bind list verdict; absent for volumes and when the agent did not answer the bind list. */
  bind?: BindInfo;
  sizeBytes: number;
  createdAt: string;
  consumers: VolumeConsumer[];
  labels: Record<string, string>;
  backup?: BackupPolicy;
  backupUnavailable?: boolean;
  timeZone?: string;
  volumeExists?: boolean;
  running?: BackupRunning;
  lastRestore?: RestoreRun;
  /** Every restore still owing stopped containers, newest first. */
  pendingRestores?: RestoreRun[];
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

/** Inventory entry of a container mount, matched by the consumer the daemon reports, never by a guessed path. */
export function storageOf(volumes: readonly Volume[] | undefined, containerId: string, target: string): Volume | undefined {
  return volumes?.find((v) => v.consumers.some((c) => c.containerId === containerId && c.target === target));
}

/** Inventory entry of a Compose mount of a deployed stack, matched by the stack's consumers and the mount target. */
export function stackStorage(volumes: readonly Volume[] | undefined, stackId: string, mount: { source: string; target: string }, service?: string): Volume | undefined {
  const bind = /^[./~]/.test(mount.source);
  return volumes?.find((v) => (v.kind === "bind") === bind && v.consumers.some((c) => c.stackId === stackId && c.target === mount.target && (service === undefined || c.service === service)));
}

export interface MountUsage {
  id: string;
  name: string;
  kind: StorageKind;
  hostId: string;
  stackId?: string;
  stackName?: string;
  composeName?: string;
  fileBind: boolean;
  consumers: VolumeConsumer[];
}

interface MountingContainer {
  id: string;
  name: string;
  hostId: string;
  stackId?: string;
  stackName?: string;
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
  return schedule.mode === "manual" || schedule.mode === "custom" ? undefined : nextOccurrence(schedule, from, timeZone);
}

export type RestoreRun = components["schemas"]["VolumeRestoreRun"];
export type BackupRunning = components["schemas"]["VolumeBackupRunning"];
export type BackupRepository = components["schemas"]["VolumeBackupRepository"];
export type UsableBackupTarget = components["schemas"]["VolumeUsableBackupTarget"];
export type BackupSelection = components["schemas"]["BackupRestoreSelection"];
export type BackupBrowse = components["schemas"]["BasePaginatedBackupBackupFileEntry"];
export interface BackupOverview {
  hostId: string;
  unavailable?: boolean;
  timeZone: string;
  authority: string;
  setVersion: number;
  repository?: components["schemas"]["VolumeBackupRepositorySummary"];
  targets: UsableBackupTarget[];
  volumes: Volume[];
}
