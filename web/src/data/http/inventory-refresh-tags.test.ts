// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary scope, reason, retention and lifetime regression coverage. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { expect, it } from "vitest";
import { resourceFixture } from "./resources.test-support";
import type { Host } from "@/domain/model";
import { overview } from "./backups.test-support";
import { storedPolicy } from "./stack-updates.test-support";

const names = ["listStacks", "listContainers", "listImages", "listVolumes", "getTopology", "listNetworkSummaries"] as const;
function freeze<T>(value: T): T {
  if (value && typeof value === "object") { Object.freeze(value); Object.values(value).forEach(freeze); }
  return value;
}
function fixture(edge: boolean) {
  const f = resourceFixture(); f.state.hostIds = ["first"]; f.state.isEdge = edge;
  const stop = f.adapter.subscribeChanges(() => {});
  const environment = (status: string) => {
    f.state.status = status;
    f.source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: "2026-11-11T12:00:00Z", environment: { environments: [{ id: "first", name: "first", status, enabled: true, apiUrl: "http://agent.example.com", isEdge: edge }] } }), lastEventId: status } as MessageEvent);
  };
  const hostRequests = () => f.fetcher.mock.calls.filter(([input]) => /\/environments\/(?:first|second)\//.test(String(input))).length;
  return { ...f, stop, environment, hostRequests };
}

for (const name of names) it.each([false, true])(`AUTO-3 ${name} keeps rows when offline refresh is skipped and reads once on online, Edge=%s`, async (edge) => {
  const f = fixture(edge);
  const read = async () => {
    const result = await f.adapter[name]();
    return Array.isArray(result) ? result : result.networks;
  };
  const first = freeze(await read()); expect(first.length).toBeGreaterThan(0);
  f.environment("offline"); const before = f.hostRequests();
  const skipped = await read();
  expect(f.hostRequests()).toBe(before);
  expect(skipped.map((r) => r.id)).toEqual(first.map((r) => r.id));
  expect(skipped[0]).not.toBe(first[0]);
  expect((await f.adapter.listHosts())[0]?.inventory).toMatchObject({ stale: true, reason: "agent.unavailable" });
  const prior = first[0] as { inventory?: { stale: boolean } }; expect(prior.inventory?.stale ?? false).toBe(false);
  f.environment("online"); const recoveryStart = f.hostRequests();
  const recovered = await read(); expect(recovered.map((r) => r.id)).toEqual(first.map((r) => r.id));
  expect(f.hostRequests()).toBeGreaterThan(recoveryStart);
  // Each physical list/detail URL is dispatched once for this recovery, with concurrent context shared.
  const recent = f.fetcher.mock.calls.filter(([input]) => /\/environments\/(?:first|second)\//.test(String(input))).slice(recoveryStart).map(([input]) => String(input));
  expect(new Set(recent).size).toBe(recent.length);
  f.stop();
});

it.each([false, true])("AUTO-3 first offline inventory reads the hub's retained snapshots, Edge=%s", async (edge) => {
  const f = fixture(edge); f.environment("offline");
  const rows = await f.adapter.listContainers();
  expect(rows.length).toBeGreaterThan(0); expect(f.hostRequests()).toBeGreaterThan(0); f.stop();
});

it.each([false, true])("AUTO-3 schedule backup and updater fan-out retains rows offline and recovers, Edge=%s", async (edge) => {
  const f = fixture(edge), original = f.fetcher.getMockImplementation()!; f.state.hostIds = ["first", "second"];
  let settingsReads = 0;
  f.fetcher.mockImplementation(async (input) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path.endsWith("/settings")) { settingsReads++; return Response.json([{ key: "effectiveTimeZone", value: "UTC" }, { key: "timeZone", value: "" }, { key: "updateCheckInterval", value: "12h" }, { key: "updateNotify", value: "false" }]); }
    if (path.endsWith("/volumes/backups/overview")) return Response.json({ success: true, data: overview });
    if (path.endsWith("/updater/policies")) return Response.json({ success: true, data: [{ ...storedPolicy, environmentId: path.split("/")[3]!, registrationId: path.split("/")[3]!, mode: "scheduled", window: { kind: "daily", time: "04:00" } }] });
    const answer = await original(input);
    if (path.endsWith("/capabilities")) {
      const body = await answer.json();
      body.data.push(...["listSchedule", "listUpdatePolicies", "getUpdateSettings", "saveSettings", "getBackupOverview"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: new URL(String(input), "https://hub.example.com").searchParams.get("kind") ?? "hub", id: new URL(String(input), "https://hub.example.com").searchParams.get("id") ?? "0" } })));
      return Response.json(body, { headers: answer.headers });
    }
    return answer;
  });
  const first = freeze(await f.adapter.listSchedule());
  expect(first.map((r) => r.kind)).toEqual(["backup", "backup", "auto-update", "auto-update"]);
  expect(settingsReads).toBe(1);
  f.environment("offline"); const before = f.hostRequests();
  const skipped = await f.adapter.listSchedule();
  expect(f.hostRequests()).toBe(before);
  expect(skipped.map((r) => r.id).sort()).toEqual(first.map((r) => r.id).sort());
  expect(skipped.every((r) => r.unavailable)).toBe(true);
  expect(skipped[0]).not.toBe(first[0]); expect(first.every((r) => !r.unavailable)).toBe(true);
  f.environment("online"); const start = f.hostRequests();
  const next = await f.adapter.listSchedule();
  expect(f.hostRequests() - start).toBe(4); expect(next.every((r) => !r.unavailable)).toBe(true); f.stop();
});

it("retained catalogue uses new host observations", async () => {
  const f = fixture(false); const hosts = freeze(await f.adapter.listHosts());
  await f.adapter.listStacks(); f.environment("offline"); await f.adapter.listStacks();
  const next: Host[] = await f.adapter.listHosts();
  expect(next[0]?.inventory?.stale).toBe(true); expect(hosts[0]?.inventory).toBeUndefined(); f.stop();
});

it.each([false, true])("AUTO-3 standby fan-out reads, interval poll skips, Edge=%s", async (edge) => {
  const f = fixture(edge); const first = await f.adapter.listContainers();
  f.environment("standby"); const before = f.hostRequests();
  const skipped = await f.adapter.listContainers(undefined, undefined, "poll");
  expect(f.hostRequests()).toBe(before); expect(skipped.map((r) => r.id)).toEqual(first.map((r) => r.id));
  await f.adapter.listContainers(undefined, undefined, "background");
  expect(f.hostRequests()).toBeGreaterThan(before); f.stop();
});

it("volume rows survive an ordinary transient refresh failure through the identity owner", async () => {
  const f = fixture(false); const first = freeze(await f.adapter.listVolumes());
  f.state.failureHost = "first"; f.state.failure = "http";
  const next = await f.adapter.listVolumes();
  expect(next.map((r) => r.id)).toEqual(first.map((r) => r.id));
  expect(next[0]?.inventory?.stale).toBe(true); expect(first[0]?.inventory?.stale).toBe(false); f.stop();
});

it("first offline resource load still reads retained rows after another inventory was loaded", async () => {
  const f = fixture(false); await f.adapter.listStacks(); f.environment("offline");
  const before = f.hostRequests(), images = await f.adapter.listImages();
  expect(images.length).toBeGreaterThan(0); expect(f.hostRequests()).toBeGreaterThan(before); f.stop();
});

it("first inventory load queued across an offline stream update still reads the retained hub snapshot", async () => {
  const f = fixture(false), original = f.fetcher.getMockImplementation()!;
  let changed = false;
  f.fetcher.mockImplementation(async (input) => {
    const answer = await original(input);
    // This list began before the stream update. The policy correctly rejects its old online status.
    if (new URL(String(input), "https://hub.example.com").pathname === "/api/environments" && !changed) { changed = true; f.environment("offline"); }
    return answer;
  });
  const rows = await f.adapter.listContainers();
  expect(changed).toBe(true);
  expect(rows.length).toBeGreaterThan(0);
  const paths = f.fetcher.mock.calls.map(([input]) => String(input));
  expect(paths.filter((path) => path.includes("/environments/first/containers?"))).toHaveLength(1);
  f.stop();
});

it("R1E-5 a skipped refresh of an update-required host keeps the persistent refusal as the visible reason", async () => {
  const f = fixture(false);
  await f.adapter.listStacks();
  f.environment("update-required"); const before = f.hostRequests();
  await f.adapter.listStacks();
  expect(f.hostRequests()).toBe(before);
  expect((await f.adapter.listHosts())[0]?.inventory).toMatchObject({ stale: true, reason: "agent.update-required" });
  f.stop();
});
