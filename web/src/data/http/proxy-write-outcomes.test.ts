// Source: docklet-next/issues/209
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original Direct/Edge proxy write uncertainty and inventory invalidation matrix.

import { describe, expect, it, vi } from "vitest";
import { networkDraft, networkFixture } from "./network-commands.test-support";
import { volumeFixture } from "./volume-commands.test-support";
import { createVolumeTransport } from "./volume-commands";

describe.each([
  { edge: false, status: 502, error: "failed to forward request" },
  { edge: true, status: 504, error: "request timed out" },
])("proxy write uncertainty (Edge=$edge, status=$status)", ({ edge, status, error }) => {
  const envelope = edge ? { success: false, code: "agent.unavailable", error }
    : { success: false, code: "agent.unavailable", data: { error } };
  function volumeProxy() {
    const f = volumeFixture(edge); const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init) => {
      const response = await original(input, init);
      return init?.method && init.method !== "GET" ? f.json(envelope, status) : response;
    });
    return f;
  }
  it.each(["create", "delete", "connect", "disconnect"])("network %s refreshes inventory and reports outcome unknown without retry", async (operation) => {
    const f = networkFixture(edge);
    expect((await f.adapter.listHosts())[0]?.connection).toBe(edge ? "edge" : "direct");
    f.state.response = { status, body: envelope };
    const changed = vi.fn(); const stop = f.adapter.subscribeChanges(changed);
    try {
      const result = operation === "create" ? f.adapter.createNetwork(networkDraft())
        : operation === "delete" ? f.adapter.removeNetwork("host~net1")
        : operation === "connect" ? f.adapter.connectNetwork({ networkId: "host~net1", containerId: "host~container1" })
        : f.adapter.disconnectNetwork({ networkId: "host~net1", containerId: "host~container1" });
      await expect(result).rejects.toMatchObject({ outcome: { kind: "error", code: "network.outcome-unknown", status } });
      expect(changed).toHaveBeenCalledExactlyOnceWith(["topology", "containers"]);
      expect(f.state.calls.filter((call) => call.method !== "GET")).toHaveLength(1);
    } finally { stop(); }
  });

  it.each(["create", "delete", "rename"])("volume %s refreshes inventory and reports outcome unknown without retry", async (operation) => {
    const f = volumeProxy();
    expect((await f.adapter.listHosts())[0]?.connection).toBe(edge ? "edge" : "direct");
    f.state.status = status; f.state.code = "agent.unavailable";
    const changed = vi.fn(); const stop = f.adapter.subscribeChanges(changed);
    try {
      const result = operation === "create" ? f.adapter.createVolume({ hostId: "host", name: "example", labels: {} })
        : operation === "delete" ? f.adapter.deleteVolume("host~data") : f.adapter.renameVolume("host~data", "example");
      await expect(result).rejects.toMatchObject({ outcome: { kind: "error", code: "volume.outcome-unknown", status } });
      expect(changed).toHaveBeenCalledExactlyOnceWith(["volumes"]);
      expect(f.state.calls).toHaveLength(1);
    } finally { stop(); }
  });

  it("volume prune refreshes inventory and marks every submitted candidate outcome unknown", async () => {
    const f = volumeProxy(); f.state.status = status; f.state.code = "agent.unavailable";
    const changed = vi.fn(); const stop = f.adapter.subscribeChanges(changed);
    try {
      const candidates = ["one", "two"].map((name) => ({ id: `host~${name}`, hostId: "host", name, createdAt: "2026-10-24T00:00:00Z" }));
      const result = await f.adapter.pruneVolumes({ hostIds: ["host"], volumeIds: candidates.map((row) => row.id), candidates });
      expect(result.removed).toEqual([]); expect(result.skipped).toEqual([]);
      expect(result.failed?.map(({ id, code }) => ({ id, code }))).toEqual(candidates.map(({ id }) => ({ id, code: "volume.outcome-unknown" })));
      expect(changed).toHaveBeenCalledExactlyOnceWith(["volumes"]);
      expect(f.state.calls).toHaveLength(1);
    } finally { stop(); }
  });

  it("volume GET preserves its definite proxy read error", async () => {
    const f = volumeFixture(edge); f.state.status = status; f.state.code = "agent.unavailable";
    const result = await f.adapter.planVolumePrune({ hostIds: ["host"] });
    expect(result.failedHosts).toEqual([{ hostId: "host", code: "agent.unavailable" }]);
    expect(f.state.calls).toHaveLength(1);
  });
});

it.each([400, 401, 403, 409, 500])("does not reclassify a definite %s response as an uncertain proxy write", async (status) => {
  const f = volumeFixture(); f.state.status = status; f.state.code = "agent.unavailable";
  const transport = createVolumeTransport({ baseUrl: "/api", client: { fetch: f.fetcher, token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), changed: vi.fn() });
  await expect(transport.request("host", "", "POST")).rejects.toMatchObject({ outcome: { code: "agent.unavailable", status } });
});
