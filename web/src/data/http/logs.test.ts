// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original deterministic shared log lifetime tests.

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { logDelivery, logGap, logLifetime, logBytes, LOG_MAX_BYTES, LOG_MAX_LINES } from "../log-buffer";
import { logFixture, settleLogs, wireLog } from "./logs.test-support";

beforeEach(() => { vi.useFakeTimers(); vi.spyOn(navigator, "onLine", "get").mockReturnValue(true); });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); });
it("shares a socket and preserves arrays, duplicate content and out-of-order timestamps", async () => {
  const f = logFixture(); const stop1 = f.owner.subscribeLogs("0~example", f.push), other = vi.fn();
  const stop2 = f.owner.subscribeLogs("0~example", other);
  await settleLogs(); expect(f.sockets).toHaveLength(1);
  f.sockets[0]!.send([{ ...wireLog(1, "same"), timestamp: "2026-01-02T03:04:06Z", service: "web" }, { ...wireLog(2, "same"), service: "worker", stream: "stderr" }]);
  expect(f.push).toHaveBeenCalledTimes(1); expect(f.frames.size).toBe(1);
  f.flush(); expect(f.lines().map((l) => l.service)).toEqual(["web", "worker"]);
  expect(f.lines().map((l) => l.message)).toEqual(["same", "same"]); expect(f.lines()[1]!.level).toBe("info");
  expect(f.lines()[0]!.ts > f.lines()[1]!.ts).toBe(true);
  stop1(); expect(f.sockets[0]!.close).not.toHaveBeenCalled(); stop2(); expect(f.sockets[0]!.close).toHaveBeenCalledTimes(1); expect(f.frames.size).toBe(0);
});
it("marks sequence gaps and reconnects with a new identity and a replacement tail", async () => {
  const f = logFixture(); const stop = f.owner.subscribeContainerLogs("direct~example", f.push);
  await settleLogs(); f.sockets[0]!.send(wireLog(4, "same")); f.sockets[0]!.send(wireLog(7, "same")); f.flush();
  expect(logGap(f.lines()[1]!)).toBe(2); const previous = f.lines()[0]!.id;
  f.sockets[0]!.disconnect(); await settleLogs(); f.flush(); expect(logDelivery(f.lines())?.status).toBe("retrying");
  await vi.advanceTimersByTimeAsync(1000); await settleLogs();
  expect(f.identity).toHaveBeenCalledTimes(1); expect(f.sockets).toHaveLength(2);
  f.sockets[1]!.send(wireLog(4, "same")); f.flush();
  expect(f.lines()).toHaveLength(1); expect(f.lines()[0]!.id).not.toBe(previous); expect(logDelivery(f.lines())?.reconnect).toBe(true);
  stop();
});
it("increments attempts when the same shared source is reopened after the last subscriber", async () => {
  const f = logFixture(); let stop = f.owner.subscribeLogs("0~example", f.push);
  await settleLogs(); f.sockets[0]!.send(wireLog(1)); f.flush(); const id = f.lines()[0]!.id; stop();
  stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs(); f.sockets[1]!.send(wireLog(1)); f.flush();
  expect(f.lines()[0]!.id).not.toBe(id); expect(logDelivery(f.lines())?.reconnect).toBe(true); stop();
});
it("bounds pending frames, drops oldest complete rows and schedules only one flush", async () => {
  const f = logFixture(); const stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs();
  for (let seq = 1; seq <= 2100; ++seq) f.sockets[0]!.send(wireLog(seq));
  expect(f.frames.size).toBe(1); expect(f.push).toHaveBeenCalledTimes(1); f.flush();
  const old = f.lines(); expect(logDelivery(old)?.dropped).toBeGreaterThan(0);
  for (let seq = 2101; seq <= 2400; ++seq) f.sockets[0]!.send(wireLog(seq, "😀".repeat(30000)));
  expect(f.frames.size).toBe(1); f.flush();
  expect(old.length + f.lines().length).toBeLessThanOrEqual(LOG_MAX_LINES);
  expect([...old, ...f.lines()].reduce((sum, l) => sum + logBytes(l), 0)).toBeLessThanOrEqual(LOG_MAX_BYTES);
  expect(f.lines().at(-1)?.seq).toBe(2400); expect(f.lines().at(-1)?.truncated).toBe(true); stop();
});
it("reset closes sources and frames synchronously and rejects captured old callbacks", async () => {
  const f = logFixture(); const lifetime = logLifetime(f.owner.subscribeLogs), notify = vi.fn(); const unwatch = lifetime.listen(notify);
  const stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs();
  const late = f.sockets[0]!.onmessage!; f.sockets[0]!.send(wireLog(1));
  f.owner.reset(); expect(f.frames.size).toBe(0); expect(f.lines()).toEqual([]); expect(notify).toHaveBeenCalledTimes(1); expect(lifetime.version()).toBe(1);
  late(new MessageEvent("message", { data: JSON.stringify(wireLog(2)) })); f.flush(); expect(f.lines()).toEqual([]);
  expect(f.sockets[0]!.close).toHaveBeenCalledTimes(1); stop(); unwatch();
});
it("last unsubscribe cancels a pending reconnect and removes network listeners", async () => {
  const f = logFixture(); const stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs();
  f.sockets[0]!.disconnect(); await settleLogs(); expect(vi.getTimerCount()).toBe(1); stop();
  expect(vi.getTimerCount()).toBe(0); expect(f.frames.size).toBe(0);
  window.dispatchEvent(new Event("online")); await settleLogs(); await vi.runAllTimersAsync(); expect(f.sockets).toHaveLength(1);
});
it("offline and unauthenticated subscriptions do not connect or retry", async () => {
  const f = logFixture(); vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
  const stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs(); f.flush();
  expect(logDelivery(f.lines())?.status).toBe("offline"); expect(f.sockets).toHaveLength(0);
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(true); window.dispatchEvent(new Event("online")); await settleLogs();
  expect(f.sockets).toHaveLength(1); f.state.authenticated = false; f.sockets[0]!.disconnect(); await settleLogs(); f.flush();
  expect(logDelivery(f.lines())?.status).toBe("unauthenticated"); expect(vi.getTimerCount()).toBe(0); stop();
});
it("denied capability starts zero sources and malformed wire stops mapping visibly", async () => {
  const f = logFixture(); f.state.allowed = false; let stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs(); f.flush();
  expect(f.sockets).toHaveLength(0); expect(logDelivery(f.lines())?.status).toBe("unavailable"); stop();
  f.state.allowed = true; stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs(); f.sockets[0]!.send({ message: "no seq" }); f.flush();
  expect(logDelivery(f.lines())?.status).toBe("error"); expect(f.lines()).toEqual([]); expect(f.sockets[0]!.close).toHaveBeenCalledTimes(1); stop();
});
it("exhausts delayed reconnects without an unbounded retry loop", async () => {
  const f = logFixture(); const stop = f.owner.subscribeLogs("0~example", f.push); await settleLogs();
  for (let i = 0; i < 6; ++i) {
    f.sockets.at(-1)!.disconnect(); await settleLogs();
    await vi.runAllTimersAsync(); await settleLogs();
  }
  f.flush(); expect(f.sockets).toHaveLength(6); expect(logDelivery(f.lines())?.status).toBe("error"); expect(vi.getTimerCount()).toBe(0); stop();
});

it("marks an open quiet source live and ignores a late open callback after reset", async () => {
  const f = logFixture(); const stop = f.owner.subscribeLogs("0~quiet", f.push); await settleLogs();
  const opened = f.sockets[0]!.onopen!; opened(new Event("open")); f.flush();
  expect(logDelivery(f.lines())?.status).toBe("live"); expect(f.lines()).toEqual([]);
  f.owner.reset(); opened(new Event("open")); f.flush(); expect(logDelivery(f.lines())?.status).toBe("unauthenticated"); stop();
});
