// Source: docklet-next/issues/370
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original tests for the latest agent restart read after a reload.

import { afterEach, expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import type { CodedActionRun } from "../message-code";
import { createHostRestarts } from "./host-restarts";

type Restart = components["schemas"]["EnvironmentAgentRestartRun"];
const at = "2026-11-13T10:00:00Z";
const settled = "2026-11-13T10:01:00Z";
const active: Restart = { runId: "run-active", createdAt: at, deadline: "2026-11-13T10:03:00Z", state: "sent" };
afterEach(() => vi.useRealTimers());

// A stub hub: `latest` answers the latest read, `detail` the per-run follow reads.
function hub(latest: () => unknown, detail: (reads: number) => Restart = () => active) {
  vi.useFakeTimers();
  let generation = 0, reads = 0;
  const runs: CodedActionRun[] = [];
  const fetcher = vi.fn(async (url: RequestInfo | URL) => {
    if (String(url).endsWith("/agent/restarts/latest")) return new Response(JSON.stringify({ success: true, data: latest() }));
    return new Response(JSON.stringify({ success: true, data: detail(++reads) }));
  });
  const capability = vi.fn(async (operationId: string, resourceRef: { kind: string; id: string }) => ({ operationId, resourceRef, available: true }));
  const api = createHostRestarts({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability, generation: () => generation, publish: (run) => runs.push(run), changed: () => {} });
  return { api, runs, fetcher, identity: () => { generation++; api.reset(); } };
}

// Row 1w: a reload during an unsettled restart follows it to connected and publishes its
// terminal transition exactly once.
it("follows an unsettled restart read after a reload to its outcome", async () => {
  const f = hub(() => ({ run: active }), (reads) => reads < 2 ? { ...active, state: "awaiting" } : { ...active, state: "connected", settledAt: settled });
  const run = await f.api.latest("example");
  expect(run?.status).toBe("running");
  await vi.advanceTimersByTimeAsync(3000);
  expect(f.runs.filter((r) => r.status === "succeeded")).toHaveLength(1);
  expect(f.api.retained().at(-1)).toMatchObject({ id: "run-active", status: "succeeded", finishedAt: settled });
  f.api.reset();
});

// Row 8: settled history is returned for the panel but never published as a new run.
it("returns settled history without publishing it", async () => {
  const f = hub(() => ({ run: { ...active, state: "failed", code: "host.busy", settledAt: settled } }));
  const run = await f.api.latest("example");
  expect(run).toMatchObject({ status: "failed", error: { code: "runMessages.restartRefused" }, finishedAt: settled });
  // A busy refusal proves the request reached the agent: reserved done, sent failed.
  expect(run?.steps.map((step) => step.state)).toEqual(["done", "error", "skipped", "skipped"]);
  expect(f.runs).toEqual([]);
  expect(f.api.retained()).toEqual([]);
  await vi.advanceTimersByTimeAsync(5000);
  expect(f.fetcher).toHaveBeenCalledTimes(1);
  f.api.reset();
});

// Row 2: an empty envelope means no restart; malformed data never becomes panel data.
it.each([
  ["empty", {}, undefined],
  ["numeric run ID", { run: { ...active, runId: 7 } }, "invalid"],
  ["unknown state", { run: { ...active, state: "rebooting" } }, "invalid"],
  ["terminal without settledAt", { run: { ...active, state: "connected" } }, "invalid"],
  ["settledAt on an active run", { run: { ...active, settledAt: settled } }, "invalid"],
  ["array", [], "invalid"],
  ["numeric dates", { run: { ...active, createdAt: 2026, deadline: 2027 } }, "invalid"],
])("reads %s", async (_name, data, outcome) => {
  const f = hub(() => data);
  if (outcome === undefined) await expect(f.api.latest("example")).resolves.toBeUndefined();
  else await expect(f.api.latest("example")).rejects.toMatchObject({ outcome: { code: "hosts.invalid-restart" } });
  expect(f.runs).toEqual([]);
  f.api.reset();
});

it("keeps an unknown valid code as a generic failure", async () => {
  const f = hub(() => ({ run: { ...active, state: "failed", code: "agent.future-code", settledAt: settled } }));
  expect((await f.api.latest("example"))?.error).toEqual({ code: "hosts.errors.failed", params: {} });
  f.api.reset();
});

// Row 3: a read answered after an identity change writes nothing.
it("drops a latest read that resolves after an identity change", async () => {
  let resolve!: (response: Response) => void;
  const f = hub(() => ({ run: active }));
  f.fetcher.mockImplementationOnce(() => new Promise<Response>((done) => { resolve = done; }));
  const read = f.api.latest("example");
  f.identity();
  resolve(new Response(JSON.stringify({ success: true, data: { run: active } })));
  await expect(read).rejects.toBeDefined();
  await vi.advanceTimersByTimeAsync(3000);
  expect(f.runs).toEqual([]);
  expect(f.api.retained()).toEqual([]);
});

// Row 9: a seed older than an in-session observation of the same run never regresses it.
it("prefers the in-session observation over an older seed", async () => {
  const f = hub(() => ({ run: active }), () => ({ ...active, state: "connected", settledAt: settled }));
  f.fetcher.mockImplementationOnce(async () => new Response(JSON.stringify({ success: true, data: active }), { status: 202 }));
  await f.api.restartAgent("example");
  await vi.advanceTimersByTimeAsync(1000);
  expect(f.api.retained().at(-1)?.status).toBe("succeeded");
  expect((await f.api.latest("example"))?.status).toBe("succeeded");
  f.api.reset();
});

// Row 9 counterpart: a reopen after the follow stopped applies the hub's further observation of
// the same run instead of keeping the stale active one.
it("applies a further observation of a known run after its follow stopped", async () => {
  let latest: Restart = active;
  const f = hub(() => ({ run: latest }));
  f.fetcher.mockImplementation(async (url) => String(url).endsWith("/agent/restarts/latest")
    ? new Response(JSON.stringify({ success: true, data: { run: latest } }))
    : new Response(JSON.stringify({ code: "agent.unavailable" }), { status: 503 }));
  await f.api.latest("example");
  await vi.advanceTimersByTimeAsync(300_000);
  const reads = f.fetcher.mock.calls.length;
  await vi.advanceTimersByTimeAsync(100_000);
  expect(f.fetcher.mock.calls.length).toBe(reads);
  latest = { ...active, state: "connected", settledAt: settled };
  const fresh = await f.api.latest("example");
  expect(fresh).toMatchObject({ status: "succeeded", finishedAt: settled });
  expect(f.runs.filter((run) => run.status === "succeeded")).toHaveLength(1);
  f.api.reset();
});
