// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original integration proof for real adapter composition and removed-volume recovery.

import { expect, it, vi } from "vitest";
import { connectedFixture } from "@/production-connected.test-support";
import { createHttpAdapter } from "./http-adapter";
import { overview, ok } from "./backups.test-support";
import { jobHref } from "@/domain/job-links";
import { detail } from "./activity-reader.test-support";

it("decorates actual volume inventory and exposes host backup next runs to the dashboard", async () => {
  const f = connectedFixture(); f.state.signedIn = true;
  const adapter = createHttpAdapter({ fetch: f.fetcher as typeof fetch });
  const volumes = await adapter.listVolumes();
  expect(volumes[0]).toMatchObject({ id: "first~data", lastBackup: { status: "partial", trigger: "safety" }, backup: { retentionCount: 7 }, consumers: [{ target: "/data" }] });
  const jobs = await adapter.listSchedule(["first"]), backup = jobs.find((row) => row.kind === "backup")!;
  expect(backup?.nextRunAt).toBe("2026-11-01T01:00:00Z"); expect(jobHref(backup)).toBe("/volumes/first~data?tab=backups");
});
it("opens metadata and snapshot history for a removed volume through the real adapter", async () => {
  const f = connectedFixture(); f.state.signedIn = true; const original = f.fetcher.getMockImplementation()!;
  const dto = structuredClone(overview); dto.volumes![0]!.volumeExists = false;
  f.fetcher.mockImplementation(async (input, init) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/volumes/data")) return Response.json({ code: "volume.not-found" }, { status: 404 });
    if (url.pathname.endsWith("/backups/overview")) return ok(dto);
    return original(input, init);
  });
  const adapter = createHttpAdapter({ fetch: f.fetcher as typeof fetch });
  await expect(adapter.getVolume("first~data")).resolves.toMatchObject({ volumeExists: false, backups: [{ id: "snapshot" }] });
});
it("carries the safe reason of an upload refused after admission into the followed activity", async () => {
  const f = connectedFixture(); f.state.signedIn = true; const original = f.fetcher.getMockImplementation()!;
  const dto = structuredClone(overview);
  dto.volumes![0]!.lastRestore = { restoreId: "restore", activityId: "upload", kind: "upload", mode: "replace", status: "failed", phase: "preparing", at: "2026-10-28T03:00:00Z", code: "volume.backup.upload-invalid", reason: "link", pendingRestart: false, stoppedContainers: [] };
  const activity = detail("first", "upload");
  activity.activity.type = "resource_action"; activity.activity.resourceType = "volume"; activity.activity.metadata = { action: "restore_volume_backup" };
  activity.activity.status = "failed"; activity.activity.error = "volume.backup.upload-invalid";
  f.fetcher.mockImplementation(async (input, init) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/activities/upload")) return ok(activity);
    if (url.pathname.endsWith("/backups/overview")) return ok(dto);
    return original(input, init);
  });
  const adapter = createHttpAdapter({ fetch: f.fetcher as typeof fetch });
  const states: { status: string; run?: { backupOutcome?: { reason?: string } } }[] = [];
  const stop = adapter.observeActionDetail!("first", "upload", (state) => states.push(state as never));
  await vi.waitFor(() => expect(states.some((state) => state.status === "ready")).toBe(true));
  stop();
  expect(states.find((state) => state.status === "ready")?.run?.backupOutcome?.reason).toBe("link");
});
