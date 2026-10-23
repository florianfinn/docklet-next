// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Producer-shaped mutation fixtures and scoped admission tests for #181.

import { describe, expect, it } from "vitest";
import { volumeFixture } from "./volume-commands.test-support";

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
