// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Producer-shaped mutation fixtures and scoped admission tests for #181.

import { describe, expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createHttpAdapter } from "./http-adapter";

// Create/Rename return volume.NewSummary plus activityId; remove returns
// base.MessageResponse. Huma errors carry detail, agent errors carry code.
export const volumeDto: components["schemas"]["VolumeVolume"] = { id: "data", name: "data", driver: "local", scope: "local", mountpoint: "", createdAt: "2026-10-22T12:00:00Z", containers: [], inUse: false, labels: {}, options: {}, size: 0 };
export function volumeFixture(edge = false) {
  const state = { available: true, status: 200, code: "", detail: "", calls: [] as { path: string; method: string; body: unknown }[] };
  const json = (data: unknown, status = 200) => new Response(JSON.stringify(status < 300 ? { success: true, data } : data), { status, headers: { "X-CSRF-Token": "example".padEnd(43, "x"), "Content-Type": "application/json" } });
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) return json(["createVolume", "deleteVolume", "renameVolume", "planVolumePrune", "pruneVolumes", "listHosts"].map((operationId) => ({ operationId, available: state.available, disabledReasonCode: "capability.unavailable", resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") } })));
    if (url.pathname.endsWith("/environments")) return new Response(JSON.stringify({ success: true, data: [{ id: "host", name: "Host", isEdge: edge, status: "online", enabled: true }], pagination: { totalItems: 1 } }));
    const method = init?.method ?? "GET";
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    state.calls.push({ path: url.pathname + url.search, method, body });
    if (state.status !== 200) return json({ code: state.code || undefined, detail: state.detail }, state.status);
    if (method === "DELETE") return json({ message: "Volume removed successfully", activityId: "activity" });
    return json({ ...volumeDto, id: body?.name ?? "data", name: body?.name ?? "data" });
  });
  return { state, fetcher, json, adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }) };
}

describe.each([false, true])("volume mutations (Edge=%s)", (edge) => {
  it("creates, removes without force, and renames to the new host-scoped ID", async () => {
    const f = volumeFixture(edge);
    expect((await f.adapter.createVolume({ hostId: "host", name: "new", labels: {} })).id).toBe("host~new");
    await f.adapter.deleteVolume("host~data");
    expect((await f.adapter.renameVolume("host~data", "renamed")).id).toBe("host~renamed");
    expect(f.state.calls).toEqual([
      { path: "/api/environments/host/volumes", method: "POST", body: { name: "new", driver: "local", labels: {} } },
      { path: "/api/environments/host/volumes/data?force=false", method: "DELETE", body: undefined },
      { path: "/api/environments/host/volumes/data/rename", method: "POST", body: { name: "renamed" } },
    ]);
  });
  it("refuses a capability without the required rights before sending a mutation", async () => {
    const f = volumeFixture(edge); f.state.available = false;
    await expect(f.adapter.createVolume({ hostId: "host", name: "new", labels: {} })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.deleteVolume("host~data")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.renameVolume("host~data", "new")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    expect(f.state.calls).toEqual([]);
  });
  it.each([[403, ""], [404, ""], [409, "agent.update-required"], [502, "agent.unavailable"], [504, "agent.unavailable"]])("maps error %s %s", async (status, code) => {
    const f = volumeFixture(edge); f.state.status = Number(status); f.state.code = String(code);
    const failure = f.adapter.deleteVolume("host~data");
    if (status === 404) await expect(failure).rejects.toMatchObject({ code: "not-found" });
    else await expect(failure).rejects.toMatchObject({ outcome: { status, code: code || "http.error" } });
  });
  it.each([409, 500])("maps in-use conflicts, including the current producer's wrapped %s", async (status) => {
    const f = volumeFixture(edge); f.state.status = status; f.state.detail = "Failed to delete volume: failed to remove volume: volume is in use";
    await expect(f.adapter.deleteVolume("host~data")).rejects.toMatchObject({ code: "in-use" });
  });
});
