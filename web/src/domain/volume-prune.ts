// Which volumes a prune removes. The same rule builds the preview and is
// applied again per volume when the prune runs.

import type { Host } from "./model";
import type { Volume } from "./volumes";

/** Why an entry is not removed. */
export type PruneSkipReason = "in-use" | "bind" | "host-offline" | "not-found";

export interface VolumePrunePlan {
  /** Unused Docker volumes on connected hosts of the scope. */
  candidates: Volume[];
  /** Entries of connected hosts that stay: bind mounts and volumes with (also stopped) consumers. */
  kept: { volume: Volume; reason: PruneSkipReason }[];
  /** Hosts of the scope that are not connected; nothing on them is read or touched. */
  skippedHosts: string[];
}

/** Bind mounts belong to the host; consumers count whether their container runs or not. */
export function pruneBlock(v: Pick<Volume, "kind" | "consumers">, host: Pick<Host, "status"> | undefined): PruneSkipReason | undefined {
  if (host?.status !== "online") return "host-offline";
  if (v.kind === "bind") return "bind";
  if (v.consumers.length > 0) return "in-use";
  return undefined;
}

export function planVolumePrune(volumes: readonly Volume[], hosts: readonly Pick<Host, "id" | "status">[], hostIds: readonly string[]): VolumePrunePlan {
  const scope = hosts.filter((h) => hostIds.includes(h.id));
  const online = new Set(scope.filter((h) => h.status === "online").map((h) => h.id));
  const candidates: Volume[] = [];
  const kept: VolumePrunePlan["kept"] = [];
  for (const v of volumes) {
    if (!online.has(v.hostId)) continue;
    const reason = pruneBlock(v, { status: "online" });
    if (reason) kept.push({ volume: v, reason });
    else candidates.push(v);
  }
  return { candidates, kept, skippedHosts: scope.filter((h) => !online.has(h.id)).map((h) => h.id) };
}
