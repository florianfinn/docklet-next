// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original runtime history and profile preference transport mapping tests for issues #335, #337 and #339.

import { describe, expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import { AdapterOutcomeError } from "../capability";
import { createHttpHistory, createHttpUiPreferences, mapCapacity } from "./history";

type WireBucket = components["schemas"]["HistoryBucket"];

const empty = { running: 0, unhealthy: 0, down: 0, stopped: 0 };
function bucket(start: string, extra: Partial<WireBucket> = {}): WireBucket {
  return {
    start, state: "nodata", minutes: empty, partial: false, cpuAverage: null, cpuMax: null, memoryAverage: null, memoryMax: null, memoryLimit: 0,
    networkRx: 0, networkTx: 0, blockRead: 0, blockWrite: 0, diskUsed: 0, diskTotal: 0, diskSource: "", exitCode: 0, ...extra,
  };
}

function client(reply: (url: string, init?: RequestInit) => Response) {
  const calls: { url: string; init?: RequestInit }[] = [];
  return {
    calls,
    client: { token: async () => "token", fetch: (async (input: RequestInfo | URL, init?: RequestInit) => { calls.push({ url: String(input), init }); return reply(String(input), init); }) as typeof fetch },
  };
}
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "Content-Type": "application/json" } });

describe("history transport", () => {
  it("reads container history by name on its host and maps buckets and aggregates", async () => {
    const wire = {
      range: "24h", totals: { running: 25, unhealthy: 0, down: 5, stopped: 0 }, uptimePercent: 83.3,
      buckets: [
        bucket("2026-11-03T10:00:00Z"),
        bucket("2026-11-03T10:15:00Z", { state: "down", minutes: { running: 10, unhealthy: 0, down: 5, stopped: 0 }, cpuAverage: 4, cpuMax: 9, memoryAverage: 100, memoryMax: 150, memoryLimit: 512, networkRx: 10, networkTx: 3, blockRead: 7, blockWrite: 1, exitCode: 137 }),
        bucket("2026-11-03T10:30:00Z", { state: "running", partial: true, minutes: { running: 15, unhealthy: 0, down: 0, stopped: 0 }, cpuAverage: 6, cpuMax: 12, memoryAverage: 120, memoryMax: 200, networkRx: 5, networkTx: 1 }),
      ],
    };
    const { calls, client: c } = client(() => ok(wire));
    const history = await createHttpHistory("/api", c).getContainerHistory({ hostId: "host-1", name: "web app" }, "24h");
    expect(calls[0]!.url).toBe("/api/environments/host-1/containers/web%20app/history?range=24h");
    expect(history.buckets.map((b) => [b.state, b.end, b.partial === true, b.exitCode])).toEqual([
      ["nodata", "2026-11-03T10:15:00.000Z", false, undefined],
      ["down", "2026-11-03T10:30:00.000Z", false, 137],
      ["running", "2026-11-03T10:45:00.000Z", true, undefined],
    ]);
    expect(history.buckets[0]!.cpuAvg === undefined && history.buckets[0]!.memLimit === undefined).toBe(true);
    expect({ avg: history.cpuAvg, max: history.cpuMax, now: history.cpuNow, memAvg: history.memAvg, memMax: history.memMax, limit: history.memLimit, up: history.uptimePercent })
      .toEqual({ avg: 5, max: 12, now: 6, memAvg: 110, memMax: 200, limit: 512, up: 83.3 });
    expect(history.totals).toEqual({ netRx: 15, netTx: 4, diskRead: 7, diskWrite: 1 });
  });

  it("splits the stack id into host and project, keeps a null uptime and omits aggregates without measurements", async () => {
    const { calls, client: c } = client(() => ok({ range: "7d", totals: empty, uptimePercent: null, buckets: [bucket("2026-11-03T08:00:00Z")] }));
    const history = await createHttpHistory("/api", c).getStackHistory("host-1~media", "7d");
    expect(calls[0]!.url).toBe("/api/environments/host-1/projects/media/history?range=7d");
    expect(history.buckets[0]!.end).toBe("2026-11-03T10:00:00.000Z");
    expect(history.uptimePercent).toBeNull();
    expect(history.cpuAvg === undefined && history.memAvg === undefined && history.memLimit === undefined).toBe(true);
  });

  it("maps typed hub problems and rejects a response for another range", async () => {
    const problem = client(() => new Response(JSON.stringify({ code: "history.unavailable", status: 503 }), { status: 503 }));
    const error = await createHttpHistory("/api", problem.client).getStackHistory("host-1~media", "24h").catch((e: unknown) => e);
    expect(error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "history.unavailable" && error.outcome.status === 503).toBe(true);
    const wrong = client(() => ok({ range: "30d", totals: empty, uptimePercent: null, buckets: [] }));
    const mismatch = await createHttpHistory("/api", wrong.client).getStackHistory("host-1~media", "24h").catch((e: unknown) => e);
    expect(mismatch instanceof AdapterOutcomeError && mismatch.outcome.kind === "error" && mismatch.outcome.code === "history.invalid-response").toBe(true);
  });

  it("maps capacity with the hub growth estimate and a line from the current disk source only", async () => {
    const history = { range: "30d", totals: empty, uptimePercent: null, buckets: [
      bucket("2026-11-01T00:00:00Z", { diskUsed: 10, diskTotal: 100, diskSource: "/old" }),
      bucket("2026-11-01T08:00:00Z"),
      bucket("2026-11-01T16:00:00Z", { diskUsed: 40, diskTotal: 100, diskSource: "/srv" }),
    ] };
    const { calls, client: c } = client(() => ok([
      { hostId: "host-1", usedBytes: 40, totalBytes: 100, growthBytesPerDay: 5, daysUntilFull: 12, growthUnavailable: false, diskSource: "/srv", history },
      { hostId: "host-2", usedBytes: 1, totalBytes: 2, growthBytesPerDay: null, daysUntilFull: null, growthUnavailable: true, diskSource: "", history: { ...history, buckets: null } },
    ]));
    const [first, second] = await createHttpHistory("/api", c).getCapacity();
    expect(calls[0]!.url).toBe("/api/history/capacity");
    expect(first).toEqual({ hostId: "host-1", diskTotal: 100, diskUsed: 40, growthPerDay: 5, daysUntilFull: 12, series: [{ at: "2026-11-02T00:00:00.000Z", used: 40 }], domain: { start: "2026-11-01T00:00:00.000Z", end: "2026-11-02T00:00:00.000Z" }, diskSource: "/srv" });
    expect(second).toEqual({ hostId: "host-2", diskTotal: 2, diskUsed: 1, series: [] });
  });
});

describe("preference transport", () => {
  it("reads the document, drops unknown sections and puts one section as {value}", async () => {
    const document = { revision: 4, sections: { appearance: { theme: "dark" }, futureSection: { x: 1 } } };
    const { calls, client: c } = client(() => ok(document));
    const api = createHttpUiPreferences("/api", c);
    expect(await api.getUiPreferences()).toEqual({ revision: 4, sections: { appearance: { theme: "dark" } } });
    await api.putUiPreference("sidebar", { collapsed: true });
    expect(calls[1]!.url).toBe("/api/auth/me/ui-preferences/sidebar");
    expect(calls[1]!.init?.method).toBe("PUT");
    expect(JSON.parse(String(calls[1]!.init?.body))).toEqual({ value: { collapsed: true } });
  });

  it("keeps the typed preference code of a rejected write", async () => {
    const { client: c } = client(() => new Response(JSON.stringify({ code: "preferences.too-large", status: 413 }), { status: 413 }));
    const error = await createHttpUiPreferences("/api", c).putUiPreference("views", {}).catch((e: unknown) => e);
    expect(error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "preferences.too-large" && error.outcome.status === 413).toBe(true);
  });
});

it("C2-b the capacity trend keeps only buckets of the current disk source, the full bucket domain and the source", () => {
  const bucket = (day: number, source: string, total = 100) => ({ start: new Date(Date.UTC(2026, 9, day)).toISOString(), diskTotal: total, diskUsed: day, diskSource: source });
  const capacity = mapCapacity({ hostId: "first", totalBytes: 100, usedBytes: 50, diskSource: "/", growthBytesPerDay: null, daysUntilFull: null, growthUnavailable: true,
    history: { range: "30d", uptimePercent: null, totals: {}, buckets: [bucket(1, "/example"), bucket(2, "/"), bucket(3, "/", 0), bucket(4, "/")] } } as never);
  expect([capacity.series.map((p) => p.used), capacity.domain, capacity.diskSource]).toEqual([[2, 4], { start: "2026-10-01T00:00:00.000Z", end: "2026-10-04T08:00:00.000Z" }, "/"]);
});
