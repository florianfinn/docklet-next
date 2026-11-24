// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original multi-host inventory reader using existing agent forwarding. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). D4 #271: fair physical host reads and result-local recovery. C1 #383: target container reads with exact ownership fallback. C2 #271: recover transient stale-success host batches without discarding snapshot rows. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags. M35 #556: results name the hosts that answered.

import { mergeInventoryObservations } from "./inventory-observations";
import { inventoryRecovery } from "../inventory-recovery";
import { inventoryReadBudget } from "./host-read-deadline";

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation, Stack } from "@/domain/model";
import { withoutSystem } from "@/domain/system";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { createConditionReader, withConditions, type ConditionReader } from "./runtime-conditions";
import { normalizeProjectName, readableProject } from "./project-ownership";
import { inventoryId, mapContainer, mapStack, splitInventoryId, type ContainerRow, type ProjectRow } from "./inventory-mapping";
import { isMaintenanceError } from "../maintenance-retry";
import { rowCollector, withUnreadable } from "./row-isolation";

/** A successful host answer whose body or pages cannot be read, as opposed to a transport or host failure. */
function malformedSuccess(error: unknown): boolean {
  if (error instanceof DOMException && error.name === "AbortError") return false;
  if (!(error instanceof AdapterOutcomeError)) return true;
  if (error.outcome.kind !== "error") return false;
  const { code, status } = error.outcome;
  // A non-2xx answer is a host or transport failure whatever its code says; it keeps its outcome.
  if (status !== undefined && (status < 200 || status >= 300)) return false;
  return /\.(invalid-page|short-page|invalid-observation|invalid-response)$/.test(code);
}

function isUnreadable(error: unknown): boolean {
  return error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "get-project.unreadable";
}

export function createInventoryReader(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (hostId: string, state: InventoryObservation) => void, readConditions: ConditionReader = createConditionReader(baseUrl, fetcher).read): Pick<DockletAdapter, "listStacks" | "listContainers" | "getStack"> {
  const { read, list } = createInventoryTransport(baseUrl, fetcher, onState);
  function fail(code: string): never { throw new AdapterOutcomeError({ kind: "error", code }); }
  async function hostInventory(host: Host, signal?: AbortSignal) {
    const [projects, containers, conditions] = await Promise.all([list("list-projects", host.id, "projects", undefined, signal), list("list-containers", host.id, "containers", undefined, signal), readConditions(host.id, signal)]);
    signal?.throwIfAborted();
    const merged: InventoryObservation = mergeInventoryObservations([projects.observation, containers.observation]);
    const projectRows = projects.rows as ProjectRow[];
    const containerRows = containers.rows as ContainerRow[];
    // One row that cannot be mapped never empties the host (#600).
    const stackRows = rowCollector("stacks"), containerRowsRead = rowCollector("containers");
    const allContainers = containerRowsRead.each(containerRows, (row) => mapContainer(row, host.id, projectRows, merged), (row) => row?.names?.[0]?.replace(/^\//, ""));
    const stacks = withoutSystem(stackRows.each(projectRows, (row) => mapStack(row, host.id, allContainers, merged), (row) => row?.name));
    const observation = withUnreadable(merged, stackRows, containerRowsRead);
    onState(host.id, observation);
    return { ...withConditions({ stacks, containers: withoutSystem(allContainers) }, conditions), observation };
  }

  async function stackInventory(host: Host, projectId: string) {
    // The project list owns stored updates and the visible label-owner tie rules.
    const [projects, conditions] = await Promise.all([list("list-projects", host.id, "projects"), readConditions(host.id)]);
    const projectRows = projects.rows as ProjectRow[];
    // Unrelated rows never affect the requested one; the requested row that cannot be read is unreadable, not missing (#600).
    const project = projectRows.find((row) => row?.id === projectId);
    if (!project) return { stacks: [], containers: [] };
    if (!readableProject(project)) return fail("get-project.unreadable");
    const label = normalizeProjectName(project.composeProjectName || project.name);
    const runtimeIds = Array.isArray(project.runtimeServices) ? project.runtimeServices.flatMap((service) => typeof service?.containerId === "string" && service.containerId ? [service.containerId] : []) : [];
    let containers = await list("list-containers", host.id, "containers", { label: `com.docker.compose.project=${label}` });
    // Directory fallback and stale runtime IDs can point outside this label. Only
    // that ambiguous case needs all containers to preserve concrete conflicting owners.
    if (runtimeIds.some((id) => !containers.rows.some((row) => row?.id === id))) {
      containers = await list("list-containers", host.id, "containers");
    }
    const observation: InventoryObservation = mergeInventoryObservations([projects.observation, containers.observation]);
    onState(host.id, observation);
    // A label-filtered container list is partial: it isolates rows but never publishes the host's containers count.
    const allContainers = rowCollector("containers").each(containers.rows as ContainerRow[], (row) => mapContainer(row, host.id, projectRows, observation));
    // The requested project itself is the page: a row that cannot be mapped is reported as unreadable, not as missing.
    let stack: Stack;
    try { stack = mapStack(project, host.id, allContainers, observation); }
    catch { return fail("get-project.unreadable"); }
    return withConditions({ stacks: withoutSystem([stack]), containers: withoutSystem(allContainers) }, conditions);
  }

  async function aggregate(operation: string, onPartial?: (batches: Awaited<ReturnType<typeof hostInventory>>[], tag: <T extends object>(data: T) => T) => void, parent?: AbortSignal) {
    const budget = inventoryReadBudget(parent);
    const scope = await budget(async () => { await requireCapability(operation); return hosts(); });
    const recovery = inventoryRecovery();
    const batches: Awaited<ReturnType<typeof hostInventory>>[] = [];
    return { recovery, batches: await Promise.all(scope.map(async (host) => {
      const batch = await budget((signal) => hostInventory(host, signal)).then((batch) => {
        recovery.observed(host, batch.observation); recovery.answered(host.id); return batch;
      }).catch((error) => {
        // A hub maintenance refusal fails the whole read, so the query keeps its rows while it retries.
        parent?.throwIfAborted(); if (isMaintenanceError(error)) throw error; recovery.failed(host, error); recovery.answered(host.id);
        const observation = inventoryFailure(host.id, error); onState(host.id, observation);
        return { stacks: [], containers: [], observation };
      });
      batches.push(batch);
      onPartial?.(batches, recovery.partial);
      return batch;
    })) };
  }
  return {
    async listStacks(onPartial, signal) { const { recovery, batches } = await aggregate("listStacks", (batches, tag) => onPartial?.(tag(batches.flatMap((b) => b.stacks))), signal); return recovery.result(batches.flatMap((batch) => batch.stacks)); },
    async listContainers(onPartial, signal) { const { recovery, batches } = await aggregate("listContainers", (batches, tag) => onPartial?.(tag(batches.flatMap((b) => b.containers))), signal); return recovery.result(batches.flatMap((batch) => batch.containers)); },
    async getStack(id) {
      await requireCapability("getStack");
      const [hostId, projectId] = splitInventoryId(id);
      const host = (await hosts()).find((item) => item.id === hostId);
      if (!host) return fail("get-project.host-not-found");
      // Plain details keep the inventory observation and retained snapshot; they omit
      // file content, which only the compose read carries and which is never cached.
      const [detail, batch, files] = await Promise.all([
        read("get-project", hostId, `projects/${encodeURIComponent(projectId)}`),
        // The list read belongs to the detail boundary: a malformed successful answer is unreadable; a transport
        // or host failure keeps its own outcome. Only a requested row that is really absent becomes not-found (#600).
        stackInventory(host, projectId).catch((error) => {
          if (isMaintenanceError(error) || isUnreadable(error)) throw error;
          onState(host.id, inventoryFailure(host.id, error));
          throw malformedSuccess(error) ? new AdapterOutcomeError({ kind: "error", code: "get-project.unreadable" }) : error;
        }),
        read("get-project-compose", hostId, `projects/${encodeURIComponent(projectId)}/compose`).then((result) => result.body.data, () => undefined),
      ]);
      // Detail boundary (#600): a missing requested list row is absence; any other failure after the host answered is unreadable.
      const listed = batch.stacks.find((item) => item.id === inventoryId(hostId, projectId));
      if (!listed) return fail("get-project.not-found");
      try {
        const data = detail.body.data;
        if (!data || data.id !== projectId) throw new Error("get-project.invalid-detail");
        const loaded = files?.id === projectId && typeof files.composeContent === "string" ? files : undefined;
        // Plain detail omits stored checks; the already-read list owns service/update projections.
        return { ...mapStack(data, hostId, batch.containers, detail.observation), services: listed.services, compose: loaded?.composeContent ?? "", env: loaded?.envContent ?? "", ...(loaded ? {} : { filesUnavailable: true }), deployments: [], restorePoints: [] };
      } catch { return fail("get-project.unreadable"); }
    },
  };
}
