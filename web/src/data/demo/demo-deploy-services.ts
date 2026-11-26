// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original simulated per-service deploy stages for the demo adapter.

import type { DeployStepKey } from "@/domain/model";
import type { DeployServiceRow, DeployServices, DeployStageState } from "@/features/activity/deploy-services";

const ORDER: Partial<Record<DeployStepKey, number>> = { pull: 0, create: 1, start: 2, health: 3 };
const clamp = (value: number) => Math.min(1, Math.max(0, value));

/** Per-service rows for a simulated deploy at `fraction` (0–1) of step `step`; no Docker calls. */
export function demoDeployServices(names: string[], step: DeployStepKey, fraction: number, failed = false): DeployServices {
  const reached = ORDER[step] ?? -1;
  const rows = names.slice(0, 64).map((name, i): DeployServiceRow => {
    const layersTotal = 3 + ((i * 2) % 5);
    const bytesTotal = (i + 1) * 48_000_000;
    const share = reached > 0 ? 1 : reached === 0 ? clamp(fraction * 1.6 - i * 0.18) : 0;
    const row: DeployServiceRow = { name, ...(i === 0 ? { check: true } : i === names.length - 1 && i > 0 ? { check: false } : {}) };
    if (reached >= 0) row.pull = { layersDone: Math.floor(share * layersTotal), layersTotal, bytesDone: Math.floor(share * bytesTotal), bytesTotal };
    // Services move through the stage one after another.
    const stage = (order: number): DeployStageState | undefined => {
      if (reached > order) return "done";
      if (reached < order) return undefined;
      const position = fraction * names.length;
      if (i < Math.floor(position)) return "done";
      if (i === Math.floor(position)) return failed ? "error" : "running";
      return undefined;
    };
    for (const [key, order] of [["create", 1], ["start", 2], ["health", 3]] as const) {
      const state = stage(order);
      if (state) row[key] = state;
    }
    return row;
  });
  return { rows, truncated: names.length > 64 };
}
