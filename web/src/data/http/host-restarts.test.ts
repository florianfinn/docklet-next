// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original durable phase, failure, identity and one-shot restart tests. M14 host maintenance and availability.

import { afterEach, expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import type { CodedActionRun } from "../message-code";
import { operationReasonKey } from "../operation-reason";
import { createHostRestarts } from "./host-restarts";
const at = "2026-10-31T00:00:00Z";
const row: components["schemas"]["EnvironmentAgentRestartRun"] = { runId: "example-run", createdAt: at, deadline: "2026-10-31T00:03:00Z", state: "reserved" };
afterEach(() => vi.useRealTimers());
function fixture(code?: string) {
  vi.useFakeTimers(); let generation = 0; let reads = 0;
  const runs: CodedActionRun[] = [];
  const fetcher = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    expect(new Headers(init?.headers).has("Idempotency-Key")).toBe(false); expect(init?.body).toBeUndefined();
    if (init?.method === "POST") { expect(String(url)).toBe("/api/environments/example/agent/restart"); return new Response(JSON.stringify({ success: true, data: row }), { status: 202 }); }
    expect(String(url)).toBe("/api/environments/example/agent/restarts/example-run");
    reads++;
    return new Response(JSON.stringify({ success: true, data: { ...row, state: reads === 1 ? "sent" : reads === 2 ? "awaiting" : code ? "unknown" : "connected", ...(reads >= 3 ? { settledAt: at, ...(code ? { code } : {}) } : {}) } }));
  });
  const capability = vi.fn(async (operationId: string, resourceRef: { kind: string; id: string }) => ({ operationId, resourceRef, available: true }));
  const api = createHostRestarts({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability, generation: () => generation, publish: run => runs.push(run), changed: () => {} });
  return { api, runs, fetcher, capability, identity: () => { generation++; api.reset(); } };
}
it("H3 refuses hub restart without a capability read or send", async () => {
  const f = fixture(); await expect(f.api.restartAgent("0")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
  expect(f.fetcher).not.toHaveBeenCalled(); expect(f.capability).not.toHaveBeenCalled(); f.api.reset();
});
it("H3 follows reserved, sent and awaiting until the producer proves a new connected instance", async () => {
  const f = fixture(); expect(await f.api.restartAgent("example")).toBe(row.runId);
  expect(f.runs.at(-1)?.status).toBe("running");
  await vi.advanceTimersByTimeAsync(1000); expect(f.runs.at(-1)?.steps[1]?.state).toBe("running");
  await vi.advanceTimersByTimeAsync(1000); expect(f.runs.at(-1)?.steps[2]?.state).toBe("running");
  await vi.advanceTimersByTimeAsync(1000); expect(f.runs.at(-1)?.status).toBe("succeeded");
  await vi.advanceTimersByTimeAsync(5000);
  expect(f.fetcher.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
  expect(f.fetcher).toHaveBeenCalledTimes(4); expect(f.api.retained().at(-1)?.status).toBe("succeeded"); f.api.reset();
});
it.each(["agent.restart-superseded", "agent.restart-timeout", "agent.restart-unsupported", "agent.restart-interrupted", "agent.restart-unconfirmed", "host.unreachable"])("H3 presents terminal %s and never resends", async code => {
  const f = fixture(code); await f.api.restartAgent("example"); await vi.advanceTimersByTimeAsync(3000);
  expect(f.runs.at(-1)).toMatchObject({ status: "failed", error: { code: `containers.operationReasons.${operationReasonKey(code)}` } });
  expect(f.fetcher.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1); f.api.reset();
});
it("H3 stops following on identity loss and retains no run or timer", async () => {
  const f = fixture(); await f.api.restartAgent("example"); f.identity(); await vi.advanceTimersByTimeAsync(3000);
  expect(f.api.retained()).toEqual([]); expect(f.fetcher).toHaveBeenCalledTimes(1);
});
it("H3 keeps host.restarting typed for runtime consumers", () => {
  expect(operationReasonKey("host.restarting")).toBe("hostRestarting");
});
