// Source: docklet-next/issues/100
// Revision: fdb37e836f708bd9ff76f75075bbb3e60ae2f0b3
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original concurrent inventory and identity refresh regressions.

import { expect, it } from "vitest";
import { resourceFixture } from "./resources.test-support";

it("shares simultaneous navigation and page inventory reads but refetches later", async () => {
  const f = resourceFixture();
  const [stacks, containers, images, volumes, topology] = await Promise.all([f.adapter.listStacks(), f.adapter.listContainers(), f.adapter.listImages(), f.adapter.listVolumes(), f.adapter.getTopology()]);
  expect(stacks).toHaveLength(2); expect(containers).toHaveLength(4);
  expect(images).toHaveLength(6); expect(volumes).toHaveLength(2); expect(topology.networks).toHaveLength(2);
  const counts = () => f.fetcher.mock.calls.filter(([url]) => /\/first\/(projects|containers)\?/.test(String(url))).length;
  expect(counts()).toBe(2);
  await f.adapter.listContainers();
  expect(counts()).toBe(4);
});

it("does not cancel in-flight inventory when the sidebar re-reads the same identity", async () => {
  const f = resourceFixture(); const original = f.fetcher.getMockImplementation()!;
  let release = () => {}; const blocked = new Promise<void>((resolve) => { release = resolve; });
  let entered = () => {}; const started = new Promise<void>((resolve) => { entered = resolve; });
  let delay = false;
  f.fetcher.mockImplementation(async (input) => {
    if (String(input).endsWith("/auth/me")) return new Response(JSON.stringify({ success: true, data: { id: "operator", username: "operator", requiresPasswordChange: false } }));
    if (delay && String(input).includes("/environments?")) { entered(); await blocked; }
    return original(input);
  });
  await f.adapter.getCurrentUser();
  delay = true;
  const pending = f.adapter.listImages().then((rows) => ({ rows }), (error: unknown) => ({ error }));
  await started;
  await f.adapter.getCurrentUser();
  release();
  expect(await pending).toMatchObject({ rows: expect.any(Array) });
});

it("still invalidates capabilities when an identity actually changes", async () => {
  const f = resourceFixture(); const original = f.fetcher.getMockImplementation()!;
  let id = "operator";
  f.fetcher.mockImplementation(async (input) => String(input).endsWith("/auth/me") ? new Response(JSON.stringify({ success: true, data: { id, username: id, requiresPasswordChange: false } })) : original(input));
  await f.adapter.getCurrentUser(); await f.adapter.getCapability("listImages");
  await f.adapter.getCurrentUser(); await f.adapter.getCapability("listImages");
  expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/capabilities"))).toHaveLength(1);
  id = "another-operator";
  await f.adapter.getCurrentUser(); await f.adapter.getCapability("listImages");
  expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/capabilities"))).toHaveLength(2);
});
