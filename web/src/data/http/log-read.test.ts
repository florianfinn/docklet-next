// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { afterEach, expect, it, vi } from "vitest";
import { createRuntimeLogs } from "./logs";
import { logFixture, LogSocket, settleLogs } from "./logs.test-support";
const options = { tail: 20, maxLines: 20, maxBytes: 65536 };
const ref = { kind: "container" as const, id: "0~example" };
const wire = (seq: number, message = "hello", timestamp = "2026-01-02T03:04:05Z") => ({ seq, message, timestamp, level: "info" });
afterEach(() => vi.unstubAllGlobals());
it("cuts a frame at line equality and closes without retry", async () => {
  const f = logFixture(), sink = vi.fn();
  const read = f.owner.readLogs(ref, { ...options, maxLines: 2 }, sink, new AbortController().signal);
  await settleLogs(); f.sockets[0]!.onopen?.(new Event("open"));
  f.sockets[0]!.send([wire(1), wire(2), wire(3)]);
  expect(await read).toEqual({ reason: "cap-lines", lines: 2, bytes: 10 });
  expect(sink.mock.calls[0]![0]).toHaveLength(2); expect(f.sockets[0]!.close).toHaveBeenCalledOnce();
  const url = new URL(f.urls[0]!); expect(url.searchParams.get("follow")).toBe("false"); expect(url.searchParams.get("tail")).toBe("20");
});
it("caps UTF-8 bytes and keeps only whole lines", async () => {
  const f = logFixture(), sink = vi.fn();
  const read = f.owner.readLogs(ref, { ...options, maxBytes: 4 }, sink, new AbortController().signal);
  await settleLogs(); f.sockets[0]!.send([wire(1, "é"), wire(2, "é"), wire(3)]);
  expect(await read).toEqual({ reason: "cap-bytes", lines: 2, bytes: 4 });
  const read2 = f.owner.readLogs(ref, { ...options, maxBytes: 1 }, sink, new AbortController().signal);
  await settleLogs(); f.sockets[1]!.send(wire(1, "é")); expect(await read2).toEqual({ reason: "cap-bytes", lines: 0, bytes: 0 });
});
it("drops out-of-window lines and continues to later in-window lines and source failures", async () => {
  const f = logFixture(), sink = vi.fn();
  const read = f.owner.readLogs(ref, { ...options, since: "2026-01-02T03:04:05Z", until: "2026-01-02T03:04:06Z" }, sink, new AbortController().signal);
  await settleLogs(); f.sockets[0]!.onopen?.(new Event("open"));
  f.sockets[0]!.send([wire(1), wire(2, "old", "2026-01-01T00:00:00Z"), wire(3, "future", "2026-01-03T00:00:00Z"), { ...wire(4, "source closed"), level: "error" }]);
  f.sockets[0]!.disconnect(); expect((await read).reason).toBe("end"); expect(sink.mock.calls[0]![0].map((l: { message: string }) => l.message)).toEqual(["hello", "source closed"]);
});
it("reports capability denial and close/error before open as unavailable", async () => {
  const f = logFixture(); f.state.allowed = false;
  expect((await f.owner.readLogs(ref, options, vi.fn(), new AbortController().signal)).reason).toBe("unavailable");
  f.state.allowed = true;
  for (const error of [false, true]) {
    const read = f.owner.readLogs(ref, options, vi.fn(), new AbortController().signal); await settleLogs();
    if (error) f.sockets.at(-1)!.onerror?.(new Event("error")); else f.sockets.at(-1)!.disconnect();
    expect((await read).reason).toBe("unavailable");
  }
});
it("reset aborts admission and rejects captured callbacks from open reads", async () => {
  let admit!: () => void;
  const gate = new Promise<void>((resolve) => { admit = resolve; });
  const socket = new LogSocket(), factory = vi.fn(() => socket), sink = vi.fn();
  const owner = createRuntimeLogs({ baseUrl: "https://hub.example.com/api", socket: factory, authenticated: () => true, checkIdentity: async () => true, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), admit: () => gate });
  const waiting = owner.readLogs(ref, options, sink, new AbortController().signal);
  await settleLogs(); owner.reset(); admit(); await settleLogs();
  expect(factory).not.toHaveBeenCalled(); expect((await waiting).reason).toBe("aborted");
  const running = owner.readLogs(ref, options, sink, new AbortController().signal); await settleLogs();
  const late = socket.onmessage!; owner.reset(); late(new MessageEvent("message", { data: JSON.stringify(wire(1)) }));
  expect((await running).reason).toBe("aborted"); expect(sink).not.toHaveBeenCalled(); expect(socket.close).toHaveBeenCalledOnce();
});
it("caller abort closes the socket and drops captured frames", async () => {
  const f = logFixture(), sink = vi.fn(), controller = new AbortController();
  const read = f.owner.readLogs(ref, options, sink, controller.signal);
  await settleLogs(); const late = f.sockets[0]!.onmessage!;
  controller.abort(); late(new MessageEvent("message", { data: JSON.stringify(wire(1)) }));
  expect(await read).toEqual({ reason: "aborted", lines: 0, bytes: 0 });
  expect(sink).not.toHaveBeenCalled(); expect(f.sockets[0]!.close).toHaveBeenCalledOnce();
});
it("reports a close without the normal end after opening as interrupted, keeping the delivered lines", async () => {
  const f = logFixture(), sink = vi.fn();
  const read = f.owner.readLogs(ref, options, sink, new AbortController().signal);
  await settleLogs(); f.sockets[0]!.onopen?.(new Event("open"));
  f.sockets[0]!.send([wire(1), wire(2)]);
  f.sockets[0]!.onerror?.(new Event("error"));
  f.sockets[0]!.disconnect(1006);
  expect(await read).toEqual({ reason: "interrupted", lines: 2, bytes: 10 });
  expect(sink.mock.calls[0]![0]).toHaveLength(2);
});
