// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W8 demo parity for identity, retained results, stop modes and definition repair.

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "./demo-adapter";
import { simulationStore } from "./simulation";

beforeEach(() => { vi.useFakeTimers(); simulationStore.set((state) => ({ ...state, latency: "instant", live: false, actionSpeed: 10 })); });
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); simulationStore.reset(); });

it("W8 keeps manual origin through rename/recreate, changes the ID and retains the keyed result", async () => {
  const adapter = createDemoAdapter();
  await adapter.setContainerOrigin("c-netdata", "standalone");
  const old = (await adapter.listContainers()).find((c) => c.id === "c-netdata")!;
  expect(old.originSource).toBe("manual");
  const edit = { image: "example/app:2", name: "renamed", environment: ["EXAMPLE=value"], hostConfig: { restartPolicy: { name: "no" } } };
  const id = await adapter.recreateContainer(old.id, edit, "example");
  await vi.runAllTimersAsync();
  const run = await adapter.getRuntimeRun!(old.hostId, id);
  expect(run.status).toBe("succeeded");
  expect(run.recreateResult).toMatchObject({ oldId: old.id, oldName: old.name, newName: "renamed" });
  const replacement = (await adapter.listContainers()).find((c) => c.id === run.recreateResult?.newId)!;
  expect(replacement).toMatchObject({ name: "renamed", origin: "standalone", originSource: "manual", env: { EXAMPLE: "value" }, restartPolicy: "no" });
  expect((await adapter.listContainers()).some((c) => c.id === old.id)).toBe(false);
  expect(await adapter.recreateContainer(old.id, edit, "example")).toBe(id);
  await expect(adapter.recreateContainer(old.id, { ...edit, name: "different" }, "example")).rejects.toMatchObject({ outcome: { code: "runtime.idempotency-mismatch" } });
  await adapter.setContainerOrigin(replacement.id, "unknown");
  expect((await adapter.listContainers()).find((c) => c.id === replacement.id)).toMatchObject({ origin: "unknown", originSource: "detected" });
});

it("W8 keeps Unraid and Compose locked through descriptors and command admission", async () => {
  const adapter = createDemoAdapter();
  const unraid = await adapter.getCapability("recreateContainer", { kind: "container", id: "c-duplicati" });
  expect(unraid).toMatchObject({ available: false, disabledReasonCode: "container.origin-locked" });
  await expect(adapter.recreateContainer("c-duplicati", { image: "example/app:2" })).rejects.toMatchObject({ outcome: { code: "container.origin-locked" } });
  const compose = (await adapter.listContainers()).find((c) => c.origin === "compose")!;
  await expect(adapter.setContainerOrigin(compose.id, "standalone")).rejects.toMatchObject({ outcome: { code: "container.origin-locked" } });
});

it("W8 down retains definition services without containers, start restores them, stop keeps containers", async () => {
  const adapter = createDemoAdapter();
  await adapter.runStackCommand("media", "stop"); await vi.runAllTimersAsync();
  const down = await adapter.getStack("media");
  expect(down.services.length).toBeGreaterThan(0);
  expect(down.services.every((service) => service.state === "stopped" && service.containersRemoved)).toBe(true);
  expect((await adapter.listContainers()).filter((c) => c.stackId === "media")).toHaveLength(0);
  await adapter.runStackCommand("media", "start"); await vi.runAllTimersAsync();
  expect((await adapter.listContainers()).some((c) => c.stackId === "media")).toBe(true);
  const settings = await adapter.getSettings();
  const save = adapter.saveSettings("general", { ...settings.general, stackStopMode: "stop" }); await vi.advanceTimersByTimeAsync(500); await save;
  await adapter.runStackCommand("media", "stop"); await vi.runAllTimersAsync();
  expect((await adapter.listContainers()).filter((c) => c.stackId === "media").every((c) => c.state === "stopped")).toBe(true);
  expect((await adapter.listContainers()).filter((c) => c.stackId === "media").length).toBeGreaterThan(0);
  await adapter.runStackCommand("media", "down"); await vi.runAllTimersAsync();
  await adapter.saveStackFiles("media", "services: [", "");
  expect((await adapter.listStacks()).find((stack) => stack.id === "media")).toMatchObject({ definitionHint: "definition-invalid", services: [] });
  await adapter.saveStackFiles("media", "services:\n  repaired:\n    image: example/app:1\n", "");
  expect((await adapter.getStack("media")).services.map((service) => service.name)).toEqual(["repaired"]);
});

it.each([false, true])("W8 replacement preserves network attachment and isolation identity (isolated=%s)", async (isolated) => {
  const adapter = createDemoAdapter();
  const oldId = "c-netdata";
  await adapter.setContainerOrigin(oldId, "standalone");
  const before = (await adapter.getTopology()).attachments.filter((attachment) => attachment.containerId === oldId);
  expect(before.length).toBeGreaterThan(0);
  if (isolated) for (const attachment of before) {
    const disconnect = adapter.disconnectNetwork({ containerId: oldId, networkId: attachment.networkId });
    await vi.advanceTimersByTimeAsync(300); await disconnect;
  }
  const old = (await adapter.listContainers()).find((container) => container.id === oldId)!;
  const runId = await adapter.recreateContainer(oldId, { image: "example/app:2" });
  await vi.runAllTimersAsync();
  const run = await adapter.getRuntimeRun!(old.hostId, runId);
  const newId = run.recreateResult!.newId;
  const topology = await adapter.getTopology();
  expect(topology.attachments.some((attachment) => attachment.containerId === oldId)).toBe(false);
  expect(topology.attachments.filter((attachment) => attachment.containerId === newId)).toEqual(isolated ? [] : before.map((attachment) => ({ ...attachment, containerId: newId })));
  expect((await adapter.listContainers()).find((container) => container.id === newId)!.networks).toEqual(old.networks);
});
