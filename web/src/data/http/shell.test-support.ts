// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell socket control, refusal and identity lifecycle evidence.


import type { ShellSocket } from "./shell";

export class Socket implements ShellSocket {
  onopen: ShellSocket["onopen"] = null;
  onmessage: ShellSocket["onmessage"] = null;
  onerror: ShellSocket["onerror"] = null;
  onclose: ShellSocket["onclose"] = null;
  binaryType: BinaryType = "blob";
  readyState: ShellSocket["readyState"] = 0;
  sent: Parameters<WebSocket["send"]>[0][] = [];
  closed = false;
  send(data: Parameters<WebSocket["send"]>[0]) { this.sent.push(data); }
  close() { this.closed = true; }
  open() { this.readyState = 1; this.onopen?.call(this as unknown as WebSocket, new Event("open")); }
  message(data: unknown) { this.onmessage?.call(this as unknown as WebSocket, new MessageEvent("message", { data })); }
  fail() { this.onerror?.call(this as unknown as WebSocket, new Event("error")); this.onclose?.call(this as unknown as WebSocket, new CloseEvent("close", { code: 1006 })); }
}
