// Source: docklet-next/issues/469
// Revision: 001338db0eb3d7d5faddce98610ef6d8dce363e3
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original schedule read hygiene proof.

import { expect, it } from "vitest";
import type { DockletAdapter } from "../adapter";
import type { Host } from "@/domain/model";
import { createCsrfClient } from "./csrf-client";
import { READ_TAG_HEADER } from "./host-read-policy";
import { createScheduleReader } from "./inventory-schedule";
import { updateFixture, updateJob } from "./stack-updates.test-support";

// The settings projection and the update-check row share one hub jobs read.
it("reads the hub jobs once per schedule read", async () => {
  const f = updateFixture();
  f.state.policy.mode = "scheduled"; f.state.policy.window = { kind: "weekly", day: 0, time: "04:00" };
  const rows = await f.adapter.listSchedule(["0", "direct"]);
  expect(rows.some((r) => r.kind === "update-check" && r.hostId === "0")).toBe(true);
  expect(rows.some((r) => r.kind === "auto-update")).toBe(true);
  expect(f.calls.filter((c) => c.path === "/api/environments/0/jobs")).toHaveLength(1);
});

// The shared jobs read belongs to the automatic fan-out, so it carries the read tag.
it("tags the shared hub jobs read", async () => {
  const seen: { path: string; tag: string | null }[] = [];
  const base = "https://hub.example.com/api";
  const client = createCsrfClient(base, async (input, init) => {
    const path = new URL(String(input)).pathname;
    seen.push({ path, tag: new Headers(init?.headers).get(READ_TAG_HEADER) });
    if (path.endsWith("/jobs")) return Response.json({ jobs: [updateJob] });
    if (path.endsWith("/settings")) return Response.json([{ key: "effectiveTimeZone", value: "UTC" }, { key: "updateCheckInterval", value: "12h" }, { key: "updateNotify", value: "false" }]);
    if (path.endsWith("/updater/policies")) return Response.json({ success: true, data: [] });
    return Response.json({ success: true, data: { mode: "local" } }, { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
  });
  const hub = { id: "0", name: "hub", status: "online" } as Host;
  const read = createScheduleReader(base, client, async () => [hub], async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), () => 0,
    async () => ({ volumes: [] }) as unknown as Awaited<ReturnType<NonNullable<DockletAdapter["getBackupOverview"]>>>);
  const rows = await read(["0"], "background");
  expect(rows.some((r) => r.kind === "update-check")).toBe(true);
  expect(seen.filter((c) => c.path.endsWith("/environments/0/jobs")).map((c) => c.tag)).toEqual(["background"]);
});
