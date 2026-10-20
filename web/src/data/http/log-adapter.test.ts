// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original real HTTP-adapter log capability and identity integration.

import { afterEach, expect, it, vi } from "vitest";
import type { LogLine } from "@/domain/model";
import { logDelivery, logLifetime } from "../log-buffer";
import { createHttpAdapter } from "./http-adapter";
import { LogSocket, settleLogs, wireLog } from "./logs.test-support";
const response = (data: unknown, status = 200) => new Response(JSON.stringify({ success: status === 200, data }), { status });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });
function setup() {
  const frames = new Map<number, FrameRequestCallback>(); let id = 0;
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { frames.set(++id, cb); return id; });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  const state = { allowed: true, user: "first", rejected: false };
  const sockets: LogSocket[] = [], urls: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) return response(["subscribeLogs", "subscribeContainerLogs"].map((operationId) => ({ operationId, available: state.allowed, resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") } })));
    if (url.pathname.endsWith("/auth/me")) return response({ id: state.user, username: "operator", requiresPasswordChange: false }, state.rejected ? 401 : 200);
    return response(null);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, webSocket: (url) => { urls.push(url); const socket = new LogSocket(); sockets.push(socket); return socket; } });
  return { adapter, state, sockets, urls, fetcher, flush: () => { const pending = [...frames.values()]; frames.clear(); pending.forEach((cb) => cb(0)); } };
}
it.each(["0", "direct", "edge"])("maps both real adapter subscriptions through capabilities for %s", async (host) => {
  const f = setup(); await f.adapter.getCurrentUser();
  const one = vi.fn(), two = vi.fn();
  const stop1 = f.adapter.subscribeLogs(`${host}~project /?`, one), stop2 = f.adapter.subscribeContainerLogs(`${host}~container /?`, two);
  await Promise.all([f.adapter.getCapability("subscribeLogs", { kind: "stack", id: `${host}~project /?` }), f.adapter.getCapability("subscribeContainerLogs", { kind: "container", id: `${host}~container /?` })]);
  await settleLogs(); expect(f.sockets).toHaveLength(2);
  expect(f.urls[0]).toContain(`/environments/${host}/ws/projects/project%20%2F%3F/logs?format=json&follow=true&tail=100&timestamps=true&batched=false`);
  expect(f.urls[1]).toContain(`/environments/${host}/ws/containers/container%20%2F%3F/logs?format=json&follow=true&tail=100&timestamps=true&batched=false`);
  f.sockets[0]!.send(wireLog(1)); f.sockets[1]!.send(wireLog(1)); f.flush();
  expect(one.mock.lastCall?.[0]).toHaveLength(1); expect(two.mock.lastCall?.[0]).toHaveLength(1);
  expect(f.adapter.mappedOperations?.has("subscribeLogs")).toBe(true); expect(f.adapter.mappedOperations?.has("subscribeContainerLogs")).toBe(true);
  stop1(); stop2(); expect(f.sockets.every((s) => s.close.mock.calls.length === 1)).toBe(true);
});
it("capability denial creates no websocket", async () => {
  const f = setup(); f.state.allowed = false; let lines: LogLine[] = [];
  const stop = f.adapter.subscribeLogs("0~example", (value) => { lines = value; });
  await f.adapter.getCapability("subscribeLogs", { kind: "stack", id: "0~example" }); await settleLogs(); f.flush();
  expect(f.sockets).toHaveLength(0); expect(logDelivery(lines)?.status).toBe("unavailable"); stop();
});
it("identity replacement and authoritative reconnect rejection reset the log lifetime", async () => {
  const f = setup(); await f.adapter.getCurrentUser(); const lifetime = logLifetime(f.adapter.subscribeLogs);
  let lines: LogLine[] = []; let stop = f.adapter.subscribeLogs("0~example", (value) => { lines = value; });
  await f.adapter.getCapability("subscribeLogs", { kind: "stack", id: "0~example" }); await settleLogs();
  f.sockets[0]!.send(wireLog(1)); f.flush(); expect(lines).toHaveLength(1);
  const old = f.sockets[0]!.onmessage!, version = lifetime.version(); f.state.user = "replacement"; await f.adapter.getCurrentUser();
  expect(lifetime.version()).toBeGreaterThan(version); expect(lines).toEqual([]); expect(f.sockets[0]!.close).toHaveBeenCalledTimes(1);
  old(new MessageEvent("message", { data: JSON.stringify(wireLog(2)) })); f.flush(); expect(lines).toEqual([]); stop();
  stop = f.adapter.subscribeLogs("0~example", (value) => { lines = value; });
  await f.adapter.getCapability("subscribeLogs", { kind: "stack", id: "0~example" }); await settleLogs();
  const rejected = new Promise<void>((resolve) => { const off = lifetime.listen(() => { off(); resolve(); }); });
  f.state.rejected = true; f.sockets[1]!.disconnect(); await rejected; f.flush();
  expect(lines).toEqual([]); expect(logDelivery(lines)?.status).toBe("unauthenticated");
  expect(f.fetcher.mock.calls.some(([input]) => String(input).endsWith("/auth/me"))).toBe(true); stop();
});
