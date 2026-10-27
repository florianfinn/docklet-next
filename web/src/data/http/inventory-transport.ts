// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original multi-host inventory reader using existing agent forwarding.

import type { operations } from "@/api/generated/schema";
import { AdapterOutcomeError } from "@/data/capability";
import type { InventoryObservation } from "@/domain/model";

type ListOperation = "list-projects" | "list-containers" | "list-images" | "list-volumes" | "list-networks";
type ReadOperation = ListOperation | "get-project" | "get-project-compose" | "get-network" | "get-dashboard";
type Body<I extends ReadOperation> = operations[I]["responses"][200]["content"]["application/json"];
const reasons = new Set(["agent.unavailable", "agent.invalid-token", "agent.update-required"]);

export function createInventoryTransport(baseUrl: string, fetcher: typeof fetch, onState: (hostId: string, state: InventoryObservation) => void) {
  function fail(code: string, status?: number): never { throw new AdapterOutcomeError({ kind: "error", code, status }); }
  async function read<I extends ReadOperation>(operation: I, hostId: string, suffix: string, start?: number, extra?: Record<string, string>, signal?: AbortSignal): Promise<{ body: Body<I>; observation: InventoryObservation }> {
    signal?.throwIfAborted();
    const params = new URLSearchParams(extra);
    if (start !== undefined) { params.set("start", String(start)); params.set("limit", "100"); params.set("order", "asc"); }
    const query = params.size ? `?${params}` : "";
    let response: Response;
    try { response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/${suffix}${query}`, { credentials: "same-origin", headers: { Accept: "application/json" }, signal }); }
    catch { return fail("http.error"); }
    let body: Body<I> & { code?: string };
    try { body = await response.json(); } catch { return fail(`${operation}.invalid-response`, response.status); }
    signal?.throwIfAborted();
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
  async function list<I extends ListOperation>(operation: I, hostId: string, suffix: string, extra?: Record<string, string>, signal?: AbortSignal) {
    const rows: unknown[] = [];
    let observation: InventoryObservation = { stale: false };
    for (;;) {
      const result = await read(operation, hostId, suffix, rows.length, extra, signal);
      const { body } = result;
      if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) return fail(`${operation}.invalid-page`);
      rows.push(...body.data);
      // Keep the oldest page timestamp and any stale reason across a paginated read.
      observation = { observedAt: observation.observedAt && result.observation.observedAt ? [observation.observedAt, result.observation.observedAt].sort()[0] : result.observation.observedAt, stale: observation.stale || result.observation.stale, reason: observation.reason ?? result.observation.reason };
      if (rows.length >= body.pagination.totalItems) return { rows: rows as NonNullable<Body<I>["data"]>, observation };
      if (!body.data.length) return fail(`${operation}.short-page`);
    }
  }
  return { read, list };
}

export { reasons };

/** Host-local failures never discard another host's inventory. */
export function inventoryFailure(hostId: string, error: unknown): InventoryObservation {
  const code = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : undefined;
  const reason = code && reasons.has(code) ? code as InventoryObservation["reason"] : code === "http.error" || error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status && error.outcome.status >= 400 ? hostId === "0" ? "agent.unavailable" : "inventory.read-error" : "inventory.mapping-error";
  return { stale: true, reason };
}
