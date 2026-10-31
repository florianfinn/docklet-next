// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original demo/disconnected parity for host availability, restart, editing and removal.

import { afterEach, expect, it, vi } from "vitest";
import { createDisconnectedAdapter } from "../disconnected-adapter";
import { createDemoAdapter } from "./demo-adapter";
import { HOSTS } from "./fixtures/hosts";
import { defaultHostColor } from "@/domain/hosts";
import { queueDemoFailure } from "./options";
import { simulationStore } from "./simulation";
afterEach(() => { vi.useRealTimers(); simulationStore.reset(); });
function fixture(status: typeof HOSTS[number]["status"] = "online") {
  vi.useFakeTimers(); simulationStore.set(s => ({ ...s, live: false, latency: "instant", actionSpeed: 20 }));
  return createDemoAdapter({ seedRuns: [], hosts: HOSTS.map(h => h.id === "atlas" ? { ...h, status } : h) });
}
async function settle<T>(task: Promise<T>): Promise<T> { const result = task.then(value => ({ value }), error => ({ error })); await vi.advanceTimersByTimeAsync(500); const settled = await result; if ("error" in settled) throw settled.error; return settled.value; }
it("H8 keeps standby settings actionable and offline unavailable", async () => {
  const adapter = fixture("standby");
  expect((await settle(adapter.getAutoHealSettings("atlas"))).enabled).toBeTypeOf("boolean");
  await expect(settle(adapter.getAutoHealSettings("cedar"))).rejects.toMatchObject({ outcome: { code: "agent.unavailable" } });
  const id = await adapter.reconnectHost("atlas"); await vi.advanceTimersByTimeAsync(500);
  expect((await adapter.listActions()).find(run => run.id === id)?.status).toBe("succeeded");
});
it("H8 applies typed names, colour reset and removal of hosts with workloads", async () => {
  const adapter = fixture();
  await expect(settle(adapter.updateHost("atlas", { name: " Boreal ", color: null }))).rejects.toMatchObject({ outcome: { code: "host.name-taken" } });
  expect(await settle(adapter.updateHost("atlas", { name: " Renamed ", color: null }))).toMatchObject({ name: "Renamed", color: defaultHostColor("atlas") });
  expect((await adapter.listContainers()).some(c => c.hostId === "atlas")).toBe(true);
  await settle(adapter.removeHost("atlas"));
  expect((await adapter.listHosts()).some(h => h.id === "atlas")).toBe(false);
  expect((await adapter.listContainers()).some(c => c.hostId === "atlas")).toBe(false);
});
it("H8 restart locks runtime work, finishes, and unlocks the host", async () => {
  const adapter = fixture(); const id = await adapter.restartAgent("atlas");
  expect((await adapter.getCapability("updateHost", { kind: "host", id: "atlas" })).disabledReasonCode).toBe("host.restarting");
  await expect(adapter.runStackCommand("media", "stop")).rejects.toMatchObject({ outcome: { code: "host.restarting" } });
  await vi.advanceTimersByTimeAsync(1000);
  expect((await adapter.listActions()).find(run => run.id === id)?.status).toBe("succeeded");
  expect((await adapter.getCapability("updateHost", { kind: "host", id: "atlas" })).available).toBe(true);
});
it("H8 token rotation supersedes a running restart and leaves the host pending", async () => {
  const adapter = fixture(); simulationStore.set(s => ({ ...s, actionSpeed: 1 }));
  const id = await adapter.restartAgent("atlas"); await settle(adapter.issueAgentToken("atlas"));
  await vi.advanceTimersByTimeAsync(5000);
  const run = (await adapter.listActions()).find(run => run.id === id);
  expect(run?.status).toBe("failed");
  expect(run?.error?.code).toBe("containers.operationReasons.restartSuperseded");
  expect((await adapter.listHosts()).find(h => h.id === "atlas")?.status).toBe("pending");
});
it("H8 v10 disables restart, and hub restart remains unavailable", async () => {
  const adapter = fixture("update-required");
  expect((await adapter.getCapability("restartAgent", { kind: "host", id: "atlas" })).disabledReasonCode).toBe("agent.update-required");
  await expect(adapter.restartAgent("atlas")).rejects.toMatchObject({ outcome: { code: "agent.update-required" } });
  expect((await adapter.getCapability("restartAgent", { kind: "host", id: "0" })).available).toBe(false);
});
it("H8 typed UI configuration refusal and disconnected capabilities preserve no fake effects", async () => {
  const adapter = createDemoAdapter({ uiConfigurationDisabledHosts: ["atlas"] });
  await expect(adapter.saveAutoHealSettings("atlas", { enabled: true, maxRestarts: 3, windowMinutes: 10, excludedStacks: [], excludedContainers: [] })).rejects.toMatchObject({ outcome: { code: "agent.ui-configuration-disabled" } });
  const disconnected = createDisconnectedAdapter();
  for (const operation of ["updateHost", "connectHost", "reconnectHost", "restartAgent", "listSystemComponents", "saveSettings"] as const) expect((await disconnected.getCapability(operation, { kind: "host", id: "example" })).available).toBe(false);
  await expect(disconnected.restartAgent("example")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
});

it("H8 replays failed recreate receipts before fresh restart admission", async () => {
  const adapter = fixture();
  await adapter.setContainerOrigin("c-netdata", "standalone");
  queueDemoFailure(adapter, { operation: "recreateContainer", at: "start" });
  const input = { image: "example/app:2" };
  const run = await adapter.recreateContainer("c-netdata", input, "example");
  await vi.advanceTimersByTimeAsync(1000);
  expect((await adapter.listActions()).find(item => item.id === run)?.status).toBe("failed");
  const container = (await adapter.listContainers()).find(item => item.id === "c-netdata")!;
  await adapter.restartAgent(container.hostId);
  const before = (await adapter.listActions()).length;
  await expect(adapter.recreateContainer(container.id, input, "example")).resolves.toBe(run);
  await expect(adapter.recreateContainer(container.id, input, "new-intent")).rejects.toMatchObject({ outcome: { code: "host.restarting" } });
  expect((await adapter.listActions()).length).toBe(before);
});
