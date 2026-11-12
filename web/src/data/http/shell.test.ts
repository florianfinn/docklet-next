// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell socket control, refusal and identity lifecycle evidence.

import { expect, it, vi } from "vitest";
import type { ShellEvent } from "../shell";
import { createShellSockets } from "./shell";
import { Socket } from "./shell.test-support";

function fixture(available = true) {
  const sockets: Socket[] = [], urls: string[] = [], events: ShellEvent[] = [];
  const auth = { active: true };
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ code: "agent.update-required" }), { status: 409 }));
  const owner = createShellSockets({ baseUrl: "/api", socket: (url) => { urls.push(url); const socket = new Socket(); sockets.push(socket); return socket; }, fetch: fetcher, authenticated: () => auth.active, capability: async (id) => ({ operationId: "openContainerShell", resourceRef: { kind: "container", id }, available, ...(available ? {} : { disabledReasonCode: "container.shell-not-running" }) }) });
  const open = () => owner.open("example-host~container /?", { shell: "auto", cols: 80, rows: 24 }, (event) => events.push(event));
  return { owner, sockets, urls, events, auth, fetcher, open };
}
it("keeps binary terminal I/O and text resize distinct", async () => {
  const f = fixture(), session = f.open(); await vi.waitFor(() => expect(f.sockets.length).toBe(1));
  const socket = f.sockets[0]!; expect(new URL(f.urls[0]!).pathname).toContain("container%20%2F%3F/terminal"); expect(socket.binaryType).toBe("arraybuffer");
  socket.open(); session.send("echo example\r"); session.resize(120, 40);
  expect(socket.sent[0]).toBe(JSON.stringify({ type: "resize", cols: 80, rows: 24 }));
  expect(new TextDecoder().decode(socket.sent[1] as Uint8Array)).toBe("echo example\r");
  expect(socket.sent[2]).toBe(JSON.stringify({ type: "resize", cols: 120, rows: 40 }));
  socket.message(new TextEncoder().encode("output").buffer); expect(f.events.at(-1)?.type).toBe("output");
  session.close(); expect(socket.closed).toBe(true);
});
it("keeps an exit frame authoritative over late errors and close codes", async () => {
  const f = fixture(), session = f.open(); await vi.waitFor(() => expect(f.sockets.length).toBe(1));
  const socket = f.sockets[0]!; socket.open(); socket.message(JSON.stringify({ type: "exit", code: 23, reason: "container.shell-orphaned" }));
  socket.fail(); socket.message(new TextEncoder().encode("late output").buffer); session.send("after exit");
  expect(f.events.at(-1)).toEqual({ type: "exit", code: 23, reason: "container.shell-orphaned" });
  expect(f.events.filter((event) => event.type === "exit").length).toBe(1); expect(socket.sent.length).toBe(1); expect(f.fetcher).not.toHaveBeenCalled(); session.close();
});
it("maps a failed upgrade HTTP problem once without using its close code", async () => {
  const f = fixture(), session = f.open(); await vi.waitFor(() => expect(f.sockets.length).toBe(1)); f.sockets[0]!.fail();
  await vi.waitFor(() => expect(f.events.at(-1)).toEqual({ type: "exit", code: null, reason: "agent.update-required" }));
  expect(f.fetcher).toHaveBeenCalledTimes(1); expect(f.fetcher.mock.calls[0]?.[1]?.credentials).toBe("same-origin"); session.close();
});
it("fences an old socket after reconnect and after identity reset", async () => {
  const f = fixture(), old = f.open(); await vi.waitFor(() => expect(f.sockets.length).toBe(1)); const first = f.sockets[0]!; first.open();
  const late = first.onmessage; old.close(); const replacement = f.open(); await vi.waitFor(() => expect(f.sockets.length).toBe(2)); const current = f.sockets[1]!; current.open();
  const before = f.events.length; late?.call(first as unknown as WebSocket, new MessageEvent("message", { data: new TextEncoder().encode("stale").buffer })); expect(f.events.length).toBe(before);
  replacement.send("current"); expect(new TextDecoder().decode(current.sent.at(-1) as Uint8Array)).toBe("current");
  f.auth.active = false; f.owner.reset(); expect(current.closed).toBe(true); expect(f.events.at(-1)).toEqual({ type: "exit", code: null, reason: "" });
});
it("refuses a capability denial without constructing a socket", async () => {
  const f = fixture(false), session = f.open(); await vi.waitFor(() => expect(f.events.at(-1)).toEqual({ type: "exit", code: null, reason: "container.shell-not-running" })); expect(f.sockets).toEqual([]); session.close();
});
it("rejects invalid controls and query bounds", async () => {
  const f = fixture(), session = f.open(); await vi.waitFor(() => expect(f.sockets.length).toBe(1)); f.sockets[0]!.open(); f.sockets[0]!.message("not JSON");
  expect(f.events.at(-1)).toEqual({ type: "exit", code: null, reason: "container.shell-protocol" }); session.close();
  const events: ShellEvent[] = []; f.owner.open("example~container", { shell: "sh", cols: 0 }, (event) => events.push(event)); expect(events.at(-1)).toEqual({ type: "exit", code: null, reason: "container.shell-invalid" }); f.owner.reset();
});
