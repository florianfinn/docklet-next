// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original multi-host inventory reader using existing agent forwarding.

import type { operations } from "@/api/generated/schema";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation } from "@/domain/model";
import { withoutSystem } from "@/domain/system";
import { inventoryId, mapContainer, mapStack, splitInventoryId, type ContainerRow, type ProjectRow } from "./inventory-mapping";

type ListOperation = "list-projects" | "list-containers";
type Body<I extends ListOperation | "get-project"> = operations[I]["responses"][200]["content"]["application/json"];
const reasons = new Set(["agent.unavailable", "agent.invalid-token", "agent.update-required"]);

export function createInventoryReader(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (hostId: string, state: InventoryObservation) => void): Pick<DockletAdapter, "listStacks" | "listContainers" | "getStack"> {
  function fail(code: string, status?: number): never { throw new AdapterOutcomeError({ kind: "error", code, status }); }
  async function read<I extends ListOperation | "get-project">(operation: I, hostId: string, suffix: string, start?: number): Promise<{ body: Body<I>; observation: InventoryObservation }> {
    const query = start === undefined ? "" : `?start=${start}&limit=100&order=asc`;
    let response: Response;
    try { response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/${suffix}${query}`, { credentials: "same-origin", headers: { Accept: "application/json" } }); }
    catch { return fail("http.error"); }
    let body: Body<I> & { code?: string };
    try { body = await response.json(); } catch { return fail(`${operation}.invalid-response`, response.status); }
    if (!response.ok) {
      if (body.code && reasons.has(body.code)) onState(hostId, { stale: true, reason: body.code as InventoryObservation["reason"] });
      return fail(body.code ?? "http.error", response.status);
    }
    if (body.success !== true || !("data" in body)) return fail(`${operation}.invalid-response`, response.status);
    const observedAt = response.headers.get("X-Docklet-Observed-At") ?? undefined;
    const reason = response.headers.get("X-Docklet-Host-Reason") ?? undefined;
    if (observedAt && !Number.isFinite(Date.parse(observedAt)) || reason && !reasons.has(reason)) return fail(`${operation}.invalid-observation`);
    const observation: InventoryObservation = { observedAt, stale: !!reason, ...(reason ? { reason: reason as InventoryObservation["reason"] } : {}) };
    return { body, observation };
  }
  async function list<I extends ListOperation>(operation: I, hostId: string, suffix: string) {
    const rows: (ProjectRow | ContainerRow)[] = [];
    let observation: InventoryObservation = { stale: false };
    for (;;) {
      const result = await read(operation, hostId, suffix, rows.length);
      const { body } = result;
      if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) return fail(`${operation}.invalid-page`);
      rows.push(...body.data);
      // Keep the oldest page timestamp and any stale reason across a paginated read.
      observation = { observedAt: observation.observedAt && result.observation.observedAt ? [observation.observedAt, result.observation.observedAt].sort()[0] : result.observation.observedAt, stale: observation.stale || result.observation.stale, reason: observation.reason ?? result.observation.reason };
      if (rows.length >= body.pagination.totalItems) return { rows, observation };
      if (!body.data.length) return fail(`${operation}.short-page`);
    }
  }
  async function hostInventory(host: Host) {
    try {
      const [projects, containers] = await Promise.all([list("list-projects", host.id, "projects"), list("list-containers", host.id, "containers")]);
      const observation: InventoryObservation = { ...projects.observation, ...(containers.observation.stale ? containers.observation : {}), stale: projects.observation.stale || containers.observation.stale };
      onState(host.id, observation);
      const projectRows = projects.rows as ProjectRow[];
      const containerRows = containers.rows as ContainerRow[];
      const allContainers = containerRows.map((row) => mapContainer(row, host.id, projectRows, observation));
      const stacks = withoutSystem(projectRows.map((row) => mapStack(row, host.id, allContainers, observation)));
      return { stacks, containers: withoutSystem(allContainers) };
    } catch (error) {
      if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && reasons.has(error.outcome.code)) return { stacks: [], containers: [] };
      if (error instanceof AdapterOutcomeError) throw error;
      return fail("inventory.mapping-error");
    }
  }
  async function aggregate(operation: string) {
    await requireCapability(operation);
    return Promise.all((await hosts()).map(hostInventory));
  }
  return {
    async listStacks() { return (await aggregate("listStacks")).flatMap((batch) => batch.stacks); },
    async listContainers() { return (await aggregate("listContainers")).flatMap((batch) => batch.containers); },
    async getStack(id) {
      await requireCapability("getStack");
      const [hostId, projectId] = splitInventoryId(id);
      const host = (await hosts()).find((item) => item.id === hostId);
      if (!host) return fail("get-project.host-not-found");
      const [detail, batch] = await Promise.all([read("get-project", hostId, `projects/${encodeURIComponent(projectId)}`), hostInventory(host)]);
      if (!detail.body.data || detail.body.data.id !== projectId) return fail("get-project.invalid-detail");
      if (!batch.stacks.some((item) => item.id === inventoryId(hostId, projectId))) return fail("get-project.not-found");
      return { ...mapStack(detail.body.data, hostId, batch.containers, detail.observation), compose: detail.body.data.composeContent ?? "", env: detail.body.data.envContent ?? "", deployments: [], restorePoints: [] };
    },
  };
}
