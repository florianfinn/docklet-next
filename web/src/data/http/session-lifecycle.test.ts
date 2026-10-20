// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

import { afterEach, expect, it, vi } from "vitest";
import type { MetricsSnapshot } from "@/domain/model";
import { identityLifetime } from "../identity-lifetime";
import { runGateOf, withRunGate } from "../run-gate";
import { deferred } from "./activity-reader.test-support";
import { settleLogs, wireLog } from "./logs.test-support";
import { sessionFixture } from "./session-lifecycle.test-support";
import { envelope, RUN_ID, snapshot } from "./runtime-commands.test-support";

const settle = async () => { await settleLogs(); await vi.advanceTimersByTimeAsync(0); };
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

it("logout synchronously closes all adapter viewers, clears metric history and rejects captured callbacks", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser();
  const snapshots: MetricsSnapshot[] = [];
  const offChanges = f.adapter.subscribeChanges(vi.fn());
  const offMetrics = f.adapter.subscribeMetrics((value) => snapshots.push(value));
  const logs = vi.fn(); const offLogs = f.adapter.subscribeLogs("0~example", logs);
  await settle();
  const stats = f.sockets.find(({ url }) => url.includes("/stats"))!.socket;
  const log = f.sockets.find(({ url }) => url.includes("/logs"))!.socket;
  f.sample(stats); log.send(wireLog(1)); await vi.advanceTimersByTimeAsync(20);
  expect(snapshots.at(-1)?.hosts["0"]?.cpuHistory).toEqual([25]);
  const oldMessage = stats.onmessage!, oldError = stats.onclose!, oldSourceError = f.sources.at(-1)!.onerror!;
  await f.adapter.logout();
  expect(stats.close).toHaveBeenCalled(); expect(log.close).toHaveBeenCalled(); expect(f.sources[0]!.close).toHaveBeenCalled();
  expect(snapshots.at(-1)?.hosts).toEqual({}); expect(snapshots.at(-1)?.containers).toEqual({}); expect(logs.mock.lastCall?.[0]).toEqual([]);
  const socketCount = f.sockets.length;
  oldMessage(new MessageEvent("message", { data: JSON.stringify({ cpuUsage: 99 }) })); oldError(new CloseEvent("close")); oldSourceError(new Event("error"));
  await vi.advanceTimersByTimeAsync(60_000); expect(f.sockets).toHaveLength(socketCount);
  await f.adapter.login("operator", "example-password"); await settle();
  const fresh = f.sockets.filter(({ url }) => url.includes("/stats")).at(-1)!.socket;
  f.sample(fresh, 10); oldError(new CloseEvent("close")); await settle();
  expect(snapshots.at(-1)?.hosts["0"]?.cpuHistory).toEqual([10]); expect(identityLifetime(f.adapter).active()).toBe(true);
  offLogs(); offMetrics(); offChanges();
});

it.each(["admitted", "ambiguous", "pending"] as const)("CC-R1-11 clears %s RunGate holds/listeners on logout/relogin and refuses late settlement", async (kind) => {
  vi.useFakeTimers(); const f = sessionFixture(); const adapter = withRunGate(f.adapter); const gate = runGateOf(adapter)!;
  await adapter.getCurrentUser(); const oldNotices = vi.fn(); gate.subscribe(oldNotices);
  if (kind === "ambiguous") f.state.ambiguous = true;
  if (kind === "pending") f.state.commandHeld = deferred<Response>();
  const command = adapter.runContainerCommand("0~example", "start");
  const result = command.catch((error: unknown) => error);
  if (kind !== "pending") await result; else await settle();
  expect(gate.size).toBe(1);
  const pending = f.state.commandHeld;
  await adapter.logout(); expect(gate.size).toBe(0);
  await adapter.login("operator", "example-password");
  f.state.commandHeld = undefined; f.state.ambiguous = false;
  const freshNotice = vi.fn(); gate.subscribe(freshNotice);
  expect(await adapter.runContainerCommand("0~example", "start")).toBe(RUN_ID);
  pending?.resolve(envelope({ code: "run.started", activityId: RUN_ID, activity: snapshot() }, 202));
  if (kind === "pending") expect(await result).toMatchObject({ message: "CancelledError" });
  await adapter.runContainerCommand("0~example", "start");
  expect(oldNotices).not.toHaveBeenCalled(); expect(freshNotice).toHaveBeenCalledTimes(1); expect(gate.size).toBe(1);
  await adapter.logout();
});

it("aborts a pending read and ignores its response even when fetch ignores AbortSignal", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser();
  f.state.blocked = deferred<Response>();
  const pending = f.adapter.listHosts().catch((error: unknown) => error); await settle();
  const signal = f.calls.find(({ path }) => path.endsWith("/environments"))!.init.signal!;
  await f.adapter.logout(); expect(signal.aborted).toBe(true);
  await f.adapter.login("operator", "example-password");
  f.state.blocked.resolve(envelope([{ id: "old-identity" }])); f.state.blocked = undefined;
  expect(await pending).toMatchObject({ message: "CancelledError" });
  expect((await f.adapter.listHosts()).map(({ id }) => id)).toEqual(["0"]);
});

it("stream uncertainty verifies identity, healthy transport reconnects and an old error cannot revoke a new login", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser(); const stop = f.adapter.subscribeChanges(vi.fn());
  f.sources[0]!.failClosed(); await settle(); await vi.advanceTimersByTimeAsync(250);
  expect(f.sources).toHaveLength(2); expect(identityLifetime(f.adapter).active()).toBe(true);
  const old = f.sources[1]!.onerror!;
  f.state.authenticated = false; f.sources[1]!.failClosed(); await settle();
  expect(identityLifetime(f.adapter).active()).toBe(false);
  await f.adapter.login("operator", "example-password"); old(new Event("error")); await settle();
  expect(identityLifetime(f.adapter).active()).toBe(true); expect(f.sources).toHaveLength(3); stop();
});

it("credential form rejection stays inline and a valid password renewal keeps metric history", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser(); const stop = f.adapter.subscribeMetrics(vi.fn()); await settle();
  f.state.credentialFailure = true;
  await expect(f.adapter.login("operator", "wrong-password")).rejects.toMatchObject({ outcome: { status: 401 } });
  await expect(f.adapter.changePassword("wrong-password", "example-password")).rejects.toMatchObject({ outcome: { status: 401 } });
  expect(identityLifetime(f.adapter).active()).toBe(true);
  f.state.credentialFailure = false;
  const before = identityLifetime(f.adapter).version(); await f.adapter.changePassword("example-password", "example-password-two");
  expect(identityLifetime(f.adapter).version()).toBe(before); stop();
});


it("logout cancels an in-flight identity verification timer and its old 401 cannot revoke a replacement", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeChanges(vi.fn());
  const held = deferred<Response>(); f.state.authHeld = held;
  f.sources[0]!.failClosed(); await settle();
  await f.adapter.logout(); expect(vi.getTimerCount()).toBe(0);
  f.state.authHeld = undefined;
  await f.adapter.login("operator", "example-password");
  held.resolve(new Response("{}", { status: 401 })); await settle();
  expect(identityLifetime(f.adapter).active()).toBe(true); stop();
});


it("known same-user renewal replaces a held generic read once without accepting its old observation", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser();
  const held = deferred<Response>(); f.state.blocked = held;
  const pending = f.adapter.listHosts(); await settle();
  await f.adapter.changePassword("example-password", "example-password-two");
  f.state.blocked = undefined; held.resolve(envelope([{ id: "old-identity-observation" }]));
  expect((await pending).map(({ id }) => id)).toEqual(["0"]);
  expect(f.calls.filter(({ path }) => path.endsWith("/environments"))).toHaveLength(2);
});


it("a reconnecting EventSource retains its native transport retry while identity remains valid", async () => {
  vi.useFakeTimers(); const f = sessionFixture(); await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeChanges(vi.fn());
  const source = f.sources[0]!; source.readyState = 0; source.onerror?.(new Event("error"));
  await settle(); await vi.advanceTimersByTimeAsync(1000);
  expect(source.close).not.toHaveBeenCalled(); expect(f.sources).toHaveLength(1);
  await f.adapter.logout(); expect(source.close).toHaveBeenCalledTimes(1); stop();
});
