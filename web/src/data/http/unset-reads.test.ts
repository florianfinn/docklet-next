// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

import { describe, expect, it, vi } from "vitest";
import fixture from "./testdata/unset-reads.json";
import { baseline, points, policy, schedules } from "./update-decoders";
import { observation } from "./update-validation";
import { createStackUpdates } from "./stack-updates";
import type { CsrfClient } from "./csrf-client";

describe("shared registered-handler unset reads", () => {
  it("decodes the absent baseline, policy, points and unchecked inventory", () => {
    expect(baseline(fixture.baseline)).toMatchObject({ available: false, code: "update.baseline-unavailable" });
    expect(policy(fixture.policy, "0", "example-stack")).toMatchObject({ revision: 0, mode: "off", candidates: [] });
    expect(points(fixture.points, "example-project")).toEqual([]);
    const unchecked = observation(fixture.unchecked["registry.example.com/app:stable"]);
    expect(unchecked).toMatchObject({ state: "unchecked", hasUpdate: false, authState: "unknown" });
    expect(fixture.unchecked).not.toHaveProperty("registry.example.com/absent:stable");
  });
  it("decodes the owner-unset schedule then saves and reloads through the HTTP adapter", async () => {
    expect(schedules(fixture.schedule, "example-offline")).toMatchObject({ revision: 0, jobs: expect.arrayContaining([expect.objectContaining({ enabled: false, code: "update.schedule-owner-unset" })]) });
    let saved = false;
    const fetch = vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "PUT") { saved = true; expect(JSON.parse(init.body as string)).toMatchObject({ expectedRevision: 0 }); }
      return new Response(JSON.stringify({ success: true, data: saved ? fixture.configuredSchedule : fixture.schedule }));
    });
    const api = createStackUpdates({ baseUrl: "/api", client: { fetch } as unknown as CsrfClient,
      capability: async (operation, resourceRef) => ({ operationId: operation, resourceRef, available: true }), generation: () => 0, changed: vi.fn() });
    expect((await api.getUpdateSchedules("example-offline")).revision).toBe(0);
    const input = { pollingInterval: fixture.configuredSchedule.pollingInterval, autoUpdateInterval: fixture.configuredSchedule.autoUpdateInterval, pollingEnabled: true, autoUpdateEnabled: true, expectedRevision: 0 };
    expect((await api.saveUpdateSchedule("example-offline", input)).revision).toBe(1);
    expect((await api.getUpdateSchedules("example-offline")).revision).toBe(1);
  });
});
