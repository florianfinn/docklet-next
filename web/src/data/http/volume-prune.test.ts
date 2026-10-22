// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Producer-shaped prune contract and mixed-host tests for #181.

import { describe, expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import { volumeDto, volumeFixture } from "./volume-commands.test-support";

type Schema = components["schemas"];
const candidate: Schema["VolumePrunePlanCandidate"] = { name: "free", driver: "local", createdAt: volumeDto.createdAt, reason: "unused" };
const plan: Schema["VolumePrunePlan"] = { candidates: [candidate, { ...candidate, name: "held", reason: "unused-anonymous" }, { ...candidate, name: "gone" }, { ...candidate, name: "replaced" }, { ...candidate, name: "failed" }] };
const result: Schema["VolumePruneResult"] = { removed: ["free"], skipped: [{ name: "held", reason: "in-use" }, { name: "gone", reason: "not-found" }, { name: "replaced", reason: "changed" }], failed: [{ name: "failed", code: "volume.remove-failed" }], spaceReclaimed: 0 };

describe.each([false, true])("prune transport (Edge=%s)", (edge) => {
  it("submits only the selected plan list, preserving snapshot timestamps and all outcomes", async () => {
    const f = volumeFixture(edge); const original = f.fetcher.getMockImplementation()!;
    let submitted: unknown;
    f.fetcher.mockImplementation(async (input, init) => {
      if (String(input).endsWith("/prune?all=true")) return f.json(plan);
      if (String(input).endsWith("/prune") && init?.method === "POST") { submitted = JSON.parse(String(init.body)); return f.json(result); }
      return original(input, init);
    });
    const preview = await f.adapter.planVolumePrune({ hostIds: ["host"] });
    expect(preview.candidates).toHaveLength(5);
    expect(preview.candidates.every((v) => v.sizeUnavailable)).toBe(true);
    // A removed selection must never be replaced by a fresh all-volume read.
    const selected = preview.candidates.filter((v) => v.name !== "failed");
    await f.adapter.pruneVolumes({ hostIds: ["host"], volumeIds: selected.map((v) => v.id), candidates: selected, all: preview.all });
    expect(submitted).toEqual({ all: true, candidates: selected.map(({ name, createdAt }) => ({ name, createdAt })) });
    const execution = await f.adapter.pruneVolumes({ hostIds: ["host"], volumeIds: preview.candidates.map((v) => v.id), candidates: preview.candidates });
    expect(execution.removed.map((v) => v.id)).toEqual(["host~free"]);
    expect(execution.skipped.map((v) => v.reason)).toEqual(["in-use", "not-found", "changed"]);
    expect(execution.failed?.[0]?.code).toBe("volume.remove-failed");
    expect(execution.spaceReclaimed).toBe(0);
  });
  it("keeps successful preview hosts alongside offline and failing hosts", async () => {
    const f = volumeFixture(edge); const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init) => {
      if (new URL(String(input), "https://hub.example.com").pathname.endsWith("/environments")) return new Response(JSON.stringify({ success: true, data: ["host", "offline", "failed"].map((id) => ({ id, name: id, apiUrl: "https://agent.example.com", isEdge: edge, status: id === "offline" ? "offline" : "online", enabled: true })), pagination: { totalItems: 3 } }));
      if (String(input).includes("/failed/volumes/prune")) return f.json({ code: "agent.unavailable" }, 502);
      if (String(input).includes("/host/volumes/prune")) return f.json(plan);
      return original(input, init);
    });
    const preview = await f.adapter.planVolumePrune({ hostIds: ["host", "offline", "failed"] });
    expect(preview.candidates).toHaveLength(5);
    expect(preview.skippedHosts).toEqual(["offline"]);
    expect(preview.failedHosts).toEqual([{ hostId: "failed", code: "agent.unavailable" }]);
    expect(f.fetcher.mock.calls.some(([input]) => String(input).includes("/offline/volumes"))).toBe(false);
  });
  it("rejects execution without a snapshot identity", async () => {
    const f = volumeFixture(edge);
    await expect(f.adapter.pruneVolumes({ hostIds: ["host"], volumeIds: ["host~free"] })).rejects.toMatchObject({ code: "invalid-plan" });
    expect(f.state.calls).toEqual([]);
  });
});
