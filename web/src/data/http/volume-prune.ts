import { isHostAvailable } from "@/domain/hosts";
// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Stateless plan and exact-candidate execution mapping for #181. M14 host maintenance and availability.

import type { components } from "@/api/generated/schema";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterError, type VolumePrunePlan, type VolumePruneResult } from "@/data/api";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host } from "@/domain/model";
import { inventoryId } from "./inventory-mapping";
import { createVolumeTransport, type VolumeTransportOptions } from "./volume-commands";

type Schema = components["schemas"];
function failureCode(error: unknown) {
  return error instanceof AdapterOutcomeError ? error.outcome.kind === "error" ? error.outcome.code : "capability.unavailable" : "http.error";
}

export function createVolumePrune(options: VolumeTransportOptions, hosts: () => Promise<Host[]>): Pick<DockletAdapter, "planVolumePrune" | "pruneVolumes"> {
  const { request, require } = createVolumeTransport(options);
  return {
    async planVolumePrune({ hostIds, all = true }) {
      await require("planVolumePrune", { kind: "hub", id: "0" });
      const available = await hosts();
      const plan: VolumePrunePlan = { candidates: [], kept: [], skippedHosts: [], failedHosts: [], candidateReasons: {}, all };
      await Promise.all([...new Set(hostIds)].map(async (hostId) => {
        if (!available.some((h) => h.id === hostId && isHostAvailable(h))) { plan.skippedHosts.push(hostId); return; }
        try {
          await require("planVolumePrune", { kind: "host", id: hostId });
          const data = await request<Schema["VolumePrunePlan"]>(hostId, `/prune?all=${all}`);
          if (data.candidates !== null && !Array.isArray(data.candidates)) throw new Error("volume.invalid-plan");
          const candidates = (data.candidates ?? []).map((row) => {
            if (!row.name || !row.createdAt || !row.driver || !["unused", "unused-anonymous"].includes(row.reason)) throw new Error("volume.invalid-plan");
            const id = inventoryId(hostId, row.name);
            return { id, hostId, name: row.name, createdAt: row.createdAt, driver: row.driver, kind: "volume" as const, source: "", labels: {}, consumers: [], sizeBytes: row.sizeBytes ?? 0, sizeUnavailable: row.sizeBytes === undefined, reason: row.reason };
          });
          plan.candidates.push(...candidates);
          for (const row of candidates) plan.candidateReasons![row.id] = row.reason;
        } catch (error) { plan.failedHosts!.push({ hostId, code: failureCode(error) }); }
      }));
      plan.candidates.sort((a, b) => a.hostId.localeCompare(b.hostId) || a.name.localeCompare(b.name));
      return plan;
    },
    async pruneVolumes({ hostIds, volumeIds, candidates, all = true }) {
      if (!candidates || candidates.some((c) => !c.createdAt || !hostIds.includes(c.hostId) || !volumeIds.includes(c.id) || inventoryId(c.hostId, c.name) !== c.id) || volumeIds.some((id) => !candidates.some((c) => c.id === id))) throw new AdapterError("invalid-plan");
      const result: VolumePruneResult = { removed: [], skipped: [], failed: [], spaceReclaimed: 0 };
      await Promise.all([...new Set(candidates.map((c) => c.hostId))].map(async (hostId) => {
        const own = candidates.filter((c) => c.hostId === hostId);
        try {
          await require("pruneVolumes", { kind: "host", id: hostId });
          const data = await request<Schema["VolumePruneResult"]>(hostId, "/prune", "POST", { all, candidates: own.map(({ name, createdAt }) => ({ name, createdAt })) });
          for (const name of data.removed ?? []) result.removed.push({ id: inventoryId(hostId, name), name, hostId, sizeBytes: 0 });
          for (const row of data.skipped ?? []) result.skipped.push({ id: inventoryId(hostId, row.name), reason: row.reason });
          for (const row of data.failed ?? []) result.failed!.push({ id: inventoryId(hostId, row.name), hostId, ...row });
          result.spaceReclaimed! += data.spaceReclaimed;
        } catch (error) { result.failed!.push(...own.map((c) => ({ ...c, code: failureCode(error) }))); }
        finally { options.changed(); }
      }));
      return result;
    },
  };
}
