// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original deterministic log sockets, frames and lifetime fixture.

import { vi } from "vitest";
import type { LogLine } from "@/domain/model";
import { createRuntimeLogs } from "./logs";
import type { MetricsSocket } from "./metrics";
export class LogSocket implements MetricsSocket {
  onopen: ((event: Event) => void) | null = null;
  onmessage: MetricsSocket["onmessage"] = null;
  onclose: MetricsSocket["onclose"] = null;
  onerror: MetricsSocket["onerror"] = null;
  close = vi.fn();
  send(value: unknown) { this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(value) })); }
  disconnect() { this.onclose?.(new CloseEvent("close")); }
}
export const wireLog = (seq: number, message = `line ${seq}`) => ({ seq, message, level: "stdout", timestamp: "2026-01-02T03:04:05Z" });
export async function settleLogs() { for (let i = 0; i < 12; ++i) await Promise.resolve(); }
export function logFixture() {
  const frames = new Map<number, FrameRequestCallback>(); let frameId = 0;
  vi.stubGlobal("requestAnimationFrame", vi.fn((cb: FrameRequestCallback) => { frames.set(++frameId, cb); return frameId; }));
  vi.stubGlobal("cancelAnimationFrame", vi.fn((id: number) => frames.delete(id)));
  const sockets: LogSocket[] = [], urls: string[] = [];
  const state = { authenticated: true, allowed: true };
  const identity = vi.fn(async (_signal: AbortSignal) => state.authenticated);
  const owner = createRuntimeLogs({
    baseUrl: "https://hub.example.com/api", authenticated: () => state.authenticated, checkIdentity: identity,
    capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: state.allowed }),
    socket: (url) => { urls.push(url); const socket = new LogSocket(); sockets.push(socket); return socket; },
  });
  let lines: LogLine[] = [];
  const push = vi.fn((value: LogLine[]) => { lines = value; });
  const flush = () => { const callbacks = [...frames.values()]; frames.clear(); callbacks.forEach((cb) => cb(0)); };
  return { owner, sockets, urls, state, identity, push, frames, flush, lines: () => lines };
}
