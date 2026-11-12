// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded bulk admission and captured result ownership. M19 web consumer integration.

import type { DockletAdapter, RuntimeCommandRequest } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { isRunBusy } from "@/data/run-gate";

export interface BulkItem { id: string; name: string; host: string; kind: "stack" | "container"; stale?: boolean }
export type BulkAction = "start" | "stop" | "restart" | "redeploy" | "update";
export interface BulkResult extends BulkItem { outcome: "started" | "joined" | "skipped" | "failed"; code?: string; error?: unknown; runId?: string }
export const bulkOperation = (kind: BulkItem["kind"], action: BulkAction) => kind === "container" ? "runContainerCommand" : action === "update" ? "installStackUpdates" : action === "redeploy" ? "redeployStack" : "runStackCommand";

export async function mapFour<T, R>(items: readonly T[], visit: (item: T, index: number) => Promise<R>): Promise<R[]> {
  const values: R[] = new Array(items.length);
  let next = 0;
  await Promise.all(Array.from({ length: Math.min(4, items.length) }, async () => {
    for (;;) { const index = next++; if (index >= items.length) return; values[index] = await visit(items[index]!, index); }
  }));
  return values;
}
export function bulkErrorCode(error: unknown) {
  return isRunBusy(error) ? "run.busy" : error instanceof AdapterOutcomeError ? error.outcome.kind === "not-available" ? error.outcome.reasonCode : error.outcome.code : "http.error";
}
/** Every request owns its copied label and reaches the same adapter admission as a single action. */
export async function runBulk(adapter: DockletAdapter, selection: readonly BulkItem[], action: BulkAction, revisions?: ReadonlyMap<string, string>): Promise<BulkResult[]> {
  const snapshot = selection.map((item) => ({ ...item }));
  const lifetime = identityLifetime(adapter), version = lifetime.version();
  const ended = () => !lifetime.active() || version !== lifetime.version();
  return mapFour(snapshot, async (item): Promise<BulkResult> => {
    const fail = (code: string, error?: unknown): BulkResult => ({ ...item, outcome: "failed", code, error });
    if (ended()) return fail("session.ended");
    if (item.stale) return { ...item, outcome: "skipped", code: "inventory.stale" };
    try {
      const capability = await adapter.getCapability(bulkOperation(item.kind, action), { kind: item.kind, id: item.id });
      if (ended()) return fail("session.ended");
      if (!capability.available) return { ...item, outcome: "skipped", code: capability.disabledReasonCode };
      const request: RuntimeCommandRequest = { kind: item.kind, id: item.id, command: action, ...(action === "update" ? { input: { expectedRevision: revisions?.get(item.id) ?? "" } } : {}) };
      const result = adapter.admitRuntimeCommand ? await adapter.admitRuntimeCommand(request, { quiet: true }) : { outcome: "started" as const, runId: item.kind === "container" ? await adapter.runContainerCommand(item.id, action as "start" | "stop" | "restart") : action === "update" ? await adapter.installStackUpdates(item.id, request.input) : await adapter.runStackCommand(item.id, action) };
      if (ended()) return fail("session.ended");
      return { ...item, ...result };
    } catch (error) { return fail(ended() ? "session.ended" : bulkErrorCode(error), error); }
  });
}
export interface BulkUpdateItem { item: BulkItem; count: number; revision?: string; code?: string; error?: unknown }
export async function readBulkUpdates(adapter: DockletAdapter, items: readonly BulkItem[]): Promise<BulkUpdateItem[]> {
  return mapFour(items.map((item) => ({ ...item })), async (item) => {
    try {
      const capability = await adapter.getCapability("installStackUpdates", { kind: "stack", id: item.id });
      if (!capability.available || item.stale) return { item, count: 0, code: item.stale ? "inventory.stale" : capability.disabledReasonCode };
      if (!adapter.getAutoUpdatePolicy || !adapter.getUpdateBaseline) return { item, count: 0, code: "bulk.not-checked" };
      const policy = await adapter.getAutoUpdatePolicy(item.id);
      if (policy.stale || policy.code) return { item, count: 0, code: policy.code ?? "bulk.not-checked" };
      if (!policy.candidates.length) return { item, count: 0, code: "bulk.up-to-date" };
      const baseline = await adapter.getUpdateBaseline(item.id);
      return baseline.available ? { item, count: policy.candidates.length, revision: baseline.savedRevision } : { item, count: 0, code: baseline.code ?? "bulk.not-checked" };
    } catch (error) { return { item, count: 0, code: bulkErrorCode(error), error }; }
  });
}
