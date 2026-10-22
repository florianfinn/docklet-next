// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original live volume mapping and regression coverage for issue #181.

import { describe, expect, it } from "vitest";
import { resourceFixture } from "./resources.test-support";

// DTOs follow VolumeHandler.GetVolume/GetVolumeUsage/GetVolumeSizes and
// volume.NewSummary: usage contains container IDs, sizes use `size`.
describe("volume reads", () => {
  it("loads detail, usage, sizes and stack consumers for Direct and Edge IDs", async () => {
    const f = resourceFixture();
    for (const host of ["first", "second"]) {
      const detail = await f.adapter.getVolume(`${host}~data`);
      expect(detail.scope).toBe("local");
      expect(detail.sizeBytes).toBe(512);
      expect(detail.consumers.map((c) => [c.containerId, c.stackId, c.running])).toEqual([[`${host}~web`, `${host}~project`, true]]);
      expect(f.fetcher.mock.calls.some(([url]) => String(url).endsWith(`/environments/${host}/volumes/data/usage`))).toBe(true);
    }
  });

  it("retains rows when one host fails and recovers on retry", async () => {
    const f = resourceFixture();
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
