// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original demo mirror of the agent bind verdict (R1 system paths, R2 target paths, special files), stored-key state and size scans.

import type { BackupTarget } from "@/domain/settings";
import type { BindInfo } from "@/domain/volumes";
import type { DemoDb } from "./demo-db";

const SYSTEM = ["/proc", "/sys", "/dev", "/etc", "/run", "/var/run", "/boot"];
const DOCKER = ["/var/lib/docker", "/var/lib/containerd"];

/** Go `path.Clean` for absolute paths, as the agent canonicalises target paths and compares bind sources. */
export function cleanPath(value: string): string {
  const parts: string[] = [];
  for (const part of value.split("/")) {
    if (part === "..") parts.pop();
    else if (part && part !== ".") parts.push(part);
  }
  return `/${parts.join("/")}`;
}

/** Segment-wise overlap of cleaned paths: equal, below or above (ancestors are refused too). */
function overlaps(a: string, b: string): boolean {
  const [path, root] = [cleanPath(a), cleanPath(b)];
  return path === root || path.startsWith(`${root}/`) || root.startsWith(`${path}/`) || path === "/" || root === "/";
}

/** Like the agent, a key with a policy, backup or restore row stays a stored key after its last consumer is gone (K3). */
export function demoStored(db: DemoDb, id: string): boolean {
  return db.backupPolicies.has(id) || db.backupRuns.some((r) => r.volumeId === id) || db.restoreRuns.has(id);
}

/** Local and mount target paths of the host or the hub, retired ones included, as the agent protects them. */
export function protectedPaths(targets: BackupTarget[], hostId: string): string[] {
  return targets.filter((t) => (t.kind === "local" || t.kind === "mount") && t.path && (!t.hostId || t.hostId === "0" || t.hostId === hostId)).map((t) => t.path!);
}

/** Demo verdict for a bind source; sockets stand in for special files. */
export function demoBind(path: string, fileBind: boolean, targets: string[], measured?: { size: number; at: string }, snapshot?: { size: number; at: string }): BindInfo {
  const special = path.endsWith(".sock");
  const denied = [...SYSTEM, ...DOCKER, ...targets].some((root) => overlaps(path, root));
  return {
    kind: special ? "unknown" : fileBind ? "file" : "directory",
    eligible: !special && !denied,
    reason: denied ? "volume.bind.path-denied" : special ? "volume.bind.unsupported-type" : undefined,
    measuredSize: measured?.size,
    measuredAt: measured?.at,
    measuredPartial: measured ? false : undefined,
    lastSnapshotSize: snapshot?.size,
    lastSnapshotAt: snapshot?.at,
  };
}
