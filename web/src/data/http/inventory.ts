// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original multi-host inventory reader using existing agent forwarding. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). D4 #271: fair physical host reads and result-local recovery. C1 #383: target container reads with exact ownership fallback. C2 #271: recover transient stale-success host batches without discarding snapshot rows. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { mergeInventoryObservations } from "./inventory-observations";
import { inventoryRecovery } from "../inventory-recovery";
import { inventoryReadBudget } from "./host-read-deadline";

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation } from "@/domain/model";
import { withoutSystem } from "@/domain/system";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { createConditionReader, withConditions, type ConditionReader } from "./runtime-conditions";
import { normalizeProjectName } from "./project-ownership";
import { inventoryId, mapContainer, mapStack, splitInventoryId, type ContainerRow, type ProjectRow } from "./inventory-mapping";

export function createInventoryReader(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (hostId: string, state: InventoryObservation) => void, readConditions: ConditionReader = createConditionReader(baseUrl, fetcher).read): Pick<DockletAdapter, "listStacks" | "listContainers" | "getStack"> {
  const { read, list } = createInventoryTransport(baseUrl, fetcher, onState);
  function fail(code: string): never { throw new AdapterOutcomeError({ kind: "error", code }); }
  async function hostInventory(host: Host, signal?: AbortSignal) {
    const [projects, containers, conditions] = await Promise.all([list("list-projects", host.id, "projects", undefined, signal), list("list-containers", host.id, "containers", undefined, signal), readConditions(host.id, signal)]);
    signal?.throwIfAborted();
    const observation: InventoryObservation = mergeInventoryObservations([projects.observation, containers.observation]);
    onState(host.id, observation);
    const projectRows = projects.rows as ProjectRow[];
    const containerRows = containers.rows as ContainerRow[];
    const allContainers = containerRows.map((row) => mapContainer(row, host.id, projectRows, observation));
    const stacks = withoutSystem(projectRows.map((row) => mapStack(row, host.id, allContainers, observation)));
    return { ...withConditions({ stacks, containers: withoutSystem(allContainers) }, conditions), observation };
  }

  async function stackInventory(host: Host, projectId: string) {
    // The project list owns stored updates and the visible label-owner tie rules.
    const [projects, conditions] = await Promise.all([list("list-projects", host.id, "projects"), readConditions(host.id)]);
    const projectRows = projects.rows as ProjectRow[];
    const project = projectRows.find((row) => row.id === projectId);
    if (!project) return { stacks: [], containers: [] };
    const label = normalizeProjectName(project.composeProjectName || project.name);
    let containers = await list("list-containers", host.id, "containers", { label: `com.docker.compose.project=${label}` });
    // Directory fallback and stale runtime IDs can point outside this label. Only
    // that ambiguous case needs all containers to preserve concrete conflicting owners.
    if (project.runtimeServices?.some((service) => service.containerId && !containers.rows.some((row) => row.id === service.containerId))) {
      containers = await list("list-containers", host.id, "containers");
    }
    const observation: InventoryObservation = mergeInventoryObservations([projects.observation, containers.observation]);
    onState(host.id, observation);
    const allContainers = (containers.rows as ContainerRow[]).map((row) => mapContainer(row, host.id, projectRows, observation));
    return withConditions({ stacks: withoutSystem([mapStack(project, host.id, allContainers, observation)]), containers: withoutSystem(allContainers) }, conditions);
  }

  async function aggregate(operation: string, onPartial?: (batches: Awaited<ReturnType<typeof hostInventory>>[]) => void, parent?: AbortSignal) {
    const budget = inventoryReadBudget(parent);
    const scope = await budget(async () => { await requireCapability(operation); return hosts(); });
    const recovery = inventoryRecovery();
    const batches: Awaited<ReturnType<typeof hostInventory>>[] = [];
    return { recovery, batches: await Promise.all(scope.map(async (host) => {
      const batch = await budget((signal) => hostInventory(host, signal)).then((batch) => {
        recovery.observed(host, batch.observation); return batch;
      }).catch((error) => {
        parent?.throwIfAborted(); recovery.failed(host, error);
        const observation = inventoryFailure(host.id, error); onState(host.id, observation);
        return { stacks: [], containers: [], observation };
      });
      batches.push(batch);
      onPartial?.(batches);
      return batch;
    })) };
  }
  return {
    async listStacks(onPartial, signal) { const { recovery, batches } = await aggregate("listStacks", (batches) => onPartial?.(batches.flatMap((b) => b.stacks)), signal); return recovery.result(batches.flatMap((batch) => batch.stacks)); },
    async listContainers(onPartial, signal) { const { recovery, batches } = await aggregate("listContainers", (batches) => onPartial?.(batches.flatMap((b) => b.containers)), signal); return recovery.result(batches.flatMap((batch) => batch.containers)); },
    async getStack(id) {
      await requireCapability("getStack");
      const [hostId, projectId] = splitInventoryId(id);
      const host = (await hosts()).find((item) => item.id === hostId);
      if (!host) return fail("get-project.host-not-found");
      // Plain details keep the inventory observation and retained snapshot; they omit
      // file content, which only the compose read carries and which is never cached.
      const [detail, batch, files] = await Promise.all([
        read("get-project", hostId, `projects/${encodeURIComponent(projectId)}`),
        stackInventory(host, projectId).catch((error) => { onState(host.id, inventoryFailure(host.id, error)); return { stacks: [], containers: [] }; }),
        read("get-project-compose", hostId, `projects/${encodeURIComponent(projectId)}/compose`).then((result) => result.body.data, () => undefined),
      ]);
      if (!detail.body.data || detail.body.data.id !== projectId) return fail("get-project.invalid-detail");
      const listed = batch.stacks.find((item) => item.id === inventoryId(hostId, projectId));
      if (!listed) return fail("get-project.not-found");
      const loaded = files?.id === projectId && typeof files.composeContent === "string" ? files : undefined;
      // Plain detail omits stored checks; the already-read list owns service/update projections.
      return { ...mapStack(detail.body.data, hostId, batch.containers, detail.observation), services: listed.services, compose: loaded?.composeContent ?? "", env: loaded?.envContent ?? "", ...(loaded ? {} : { filesUnavailable: true }), deployments: [], restorePoints: [] };
    },
  };
}
