// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original assigned-stack reads without host-wide container inventory.

import type { CurrentUser } from "../api";
import type { DockletAdapter } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import type { Host, InventoryObservation } from "@/domain/model";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { mapStack, splitInventoryId, type ProjectRow } from "./inventory-mapping";

export function createUserInventory(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, principal: () => CurrentUser | undefined, onState: (host: string, state: InventoryObservation) => void): Pick<DockletAdapter, "listStacks" | "getStack"> {
  const { list, read } = createInventoryTransport(baseUrl, fetcher, onState);
  return {
    async listStacks(onPartial) {
      const batches: Awaited<ReturnType<DockletAdapter["listStacks"]>>[] = [];
      const result = await Promise.all((await hosts()).map(async (host) => {
        let rows: Awaited<ReturnType<DockletAdapter["listStacks"]>> = [];
        try {
          const projects = await list("list-projects", host.id, "projects");
          rows = (projects.rows as ProjectRow[]).map((p) => mapStack(p, host.id, [], projects.observation));
        } catch (error) {
          onState(host.id, inventoryFailure(host.id, error));
        }
        batches.push(rows); onPartial?.(batches.flat()); return rows;
      }));
      return result.flat();
    },
    async getStack(id) {
      const [host, project] = splitInventoryId(id);
      const assignment = principal()?.assignments?.find((a) => a.environmentId === host && a.projectId === project);
      if (!assignment) throw new Error("not-found");
      try {
        const full = assignment.level === "full";
        const path = `projects/${encodeURIComponent(project)}`;
        const [runtime, files, updates] = await Promise.all([
          read("get-project-runtime", host, `${path}/runtime`),
          full ? read("get-project-compose", host, `${path}/compose`).then((r) => r.body.data, () => undefined) : undefined,
          full ? read("get-project-updates", host, `${path}/updates`).then((r) => r.body.data, () => undefined) : undefined,
        ]);
        if (runtime.body.data.id !== project) throw new Error("get-project.invalid-detail");
        const loaded = files?.id === project && typeof files.composeContent === "string" ? files : undefined;
        const row = { ...runtime.body.data, ...(updates?.id === project ? { updateInfo: updates.updateInfo } : {}) };
        return { ...mapStack(row, host, [], runtime.observation), compose: loaded?.composeContent ?? "", env: loaded?.envContent ?? "", ...(loaded ? {} : { filesUnavailable: true }), deployments: [], restorePoints: [] };
      } catch (error) {
        if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 404) throw new Error("not-found", { cause: error });
        throw error;
      }
    },
  };
}
