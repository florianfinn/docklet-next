// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original demo mirror of the agent bind verdict (R1 system paths, special files) and size scans.

import type { BindInfo } from "@/domain/volumes";

const SYSTEM = ["/proc", "/sys", "/dev", "/etc", "/run", "/var/run", "/boot"];
const DOCKER = ["/var/lib/docker", "/var/lib/containerd"];

/** Segment-wise overlap: equal, below or above (ancestors are refused too). */
function overlaps(path: string, root: string): boolean {
  return path === root || path.startsWith(`${root}/`) || root.startsWith(`${path}/`) || path === "/";
}

/** Demo verdict for a bind source; sockets stand in for special files. */
export function demoBind(path: string, fileBind: boolean, measured?: { size: number; at: string }, snapshot?: { size: number; at: string }): BindInfo {
  const special = path.endsWith(".sock");
  const denied = SYSTEM.some((root) => overlaps(path, root)) || DOCKER.some((root) => overlaps(path, root));
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
