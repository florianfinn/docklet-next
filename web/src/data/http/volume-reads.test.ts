// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original live volume mapping and regression coverage for issue #181.

import { describe, expect, it } from "vitest";
import { resourceFixture } from "./resources.test-support";

// GetVolume includes the shared consumer projection; sizes use `size`.
describe.each([false, true])("volume reads (Edge=%s)", (edge) => {
  it("loads detail, sizes and stack consumers without a separate usage request for Direct and Edge IDs", async () => {
    const f = resourceFixture(); f.state.isEdge = edge;
    for (const host of ["first", "second"]) {
      const detail = await f.adapter.getVolume(`${host}~data`);
      expect(detail.scope).toBe("local");
      expect(detail.sizeBytes).toBe(512);
      expect(detail.consumers.map((c) => [c.containerId, c.stackId, c.running])).toEqual([[`${host}~web`, `${host}~project`, true]]);
      expect(f.fetcher.mock.calls.some(([url]) => String(url).endsWith(`/environments/${host}/volumes/data/usage`))).toBe(false);
    }
  });

  it("keeps stopped stack consumers and omits an absent Edge mountpoint", async () => {
    const f = resourceFixture(); f.state.isEdge = edge;
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => {
      const response = await original(input);
      const url = new URL(String(input), "https://hub.example.com");
      if (!url.pathname.endsWith("/containers") && !url.pathname.endsWith("/volumes/data")) return response;
      const body = await response.json();
      if (url.pathname.endsWith("/containers")) body.data[0].state = "exited";
      else body.data.mountpoint = "";
      return new Response(JSON.stringify(body), { headers: response.headers });
    });
    const detail = await f.adapter.getVolume("first~data");
    expect(detail.source).toBe("");
    expect(detail.consumers).toMatchObject([{ containerId: "first~web", stackId: "first~project", running: false, target: "/data" }]);
  });

  it("retains rows when one host fails and recovers on retry", async () => {
    const f = resourceFixture(); f.state.isEdge = edge;
    expect((await f.adapter.listVolumes()).length).toBe(2);
    f.state.failure = "http";
    const partial: string[][] = [];
    const failed = await f.adapter.listVolumes((rows) => partial.push(rows.map((v) => v.id)));
    expect(failed.map((v) => v.id).sort()).toEqual(["first~data", "second~data"]);
    expect(partial.every((rows) => rows.includes("second~data"))).toBe(true);
    expect(failed.find((v) => v.hostId === "second")?.inventory?.stale).toBe(true);
    f.state.failure = "";
    expect((await f.adapter.listVolumes()).every((v) => !v.inventory?.stale)).toBe(true);
  });
});
