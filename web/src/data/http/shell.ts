// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original owned binary/control socket with authoritative exits and HTTP refusal recovery.

import type { CapabilityDescriptor } from "../capability";
import { shellExit, type OpenShell, type ShellOptions, type ShellResize } from "../shell";
import { splitInventoryId } from "./inventory-mapping";
export type ShellSocket = Pick<WebSocket, "onopen" | "onmessage" | "onerror" | "onclose" | "binaryType" | "readyState" | "send" | "close">;
export function shellAddress(baseUrl: string, id: string, options: ShellOptions) {
  const [host, raw] = splitInventoryId(id);
  const url = new URL(`${baseUrl}/environments/${encodeURIComponent(host)}/ws/containers/${encodeURIComponent(raw)}/terminal`, window.location.href);
  url.searchParams.set("shell", options.shell);
  if (options.cols !== undefined) url.searchParams.set("cols", String(options.cols));
  if (options.rows !== undefined) url.searchParams.set("rows", String(options.rows));
  return url;
}
export function createShellSockets(options: {
  baseUrl: string;
  socket: (url: string) => ShellSocket;
  fetch: typeof globalThis.fetch;
  capability: (id: string) => Promise<CapabilityDescriptor>;
  authenticated: () => boolean;
}) {
  const sessions = new Set<() => void>();
  const open: OpenShell = (id, query, push) => {
    let active = true, exited = false, opened = false, recovering = false;
    let socket: ShellSocket | undefined;
    let cols = query.cols ?? 80, rows = query.rows ?? 24;
    const controller = new AbortController();
    function exit(reason: string) { if (!active || exited) return; exited = true; push({ type: "exit", code: null, reason }); }
    function close() {
      if (!active) return; active = false; controller.abort(); sessions.delete(reset);
      if (socket) { socket.onopen = socket.onmessage = socket.onerror = socket.onclose = null; socket.close(); socket = undefined; }
    }
    function reset() { exit(""); close(); }
    sessions.add(reset);
    async function failed() {
      if (!active || exited || recovering) return;
      if (opened) { exit(""); return; }
      recovering = true;
      // Browsers hide failed-upgrade HTTP bodies. A plain GET rechecks the same
      // hub route without upgrading or opening another shell activity.
      let reason = "";
      const timeout = setTimeout(() => controller.abort(), 5000);
      try {
        const response = await options.fetch(shellAddress(options.baseUrl, id, query), { credentials: "same-origin", signal: controller.signal });
        const problem: unknown = await response.json();
        if (problem && typeof problem === "object" && "code" in problem && typeof problem.code === "string") reason = problem.code;
        if (response.status === 401) reason = "authentication.required";
        if (response.status === 403) reason = "capability.unavailable";
      } catch { /* A lost route has no more specific bounded reason. */ }
      finally { clearTimeout(timeout); }
      exit(reason);
    }
    void (async () => {
      push({ type: "connecting" });
      try {
        if (!["auto", "sh", "bash"].includes(query.shell) || [cols, rows].some((v) => !Number.isInteger(v) || v < 1 || v > 1000)) { exit("container.shell-invalid"); return; }
        if (!options.authenticated()) { exit("authentication.required"); return; }
        const capability = await options.capability(id);
        if (!active) return;
        if (!capability.available) { exit(capability.disabledReasonCode ?? "capability.unavailable"); return; }
        const address = shellAddress(options.baseUrl, id, query);
        address.protocol = address.protocol === "https:" ? "wss:" : "ws:";
        socket = options.socket(address.href); socket.binaryType = "arraybuffer";
        socket.onopen = () => { if (!active || !socket || exited) return; opened = true; push({ type: "live" }); socket.send(JSON.stringify({ type: "resize", cols, rows } satisfies ShellResize)); };
        socket.onmessage = (event) => {
          if (!active || exited || !options.authenticated()) return;
          if (event.data instanceof ArrayBuffer) { push({ type: "output", data: new Uint8Array(event.data) }); return; }
          if (typeof event.data === "string") {
            try { const end = shellExit(JSON.parse(event.data)); if (end) { exited = true; push(end); return; } } catch { /* Invalid control ends the session below. */ }
          }
          exit("container.shell-protocol"); socket?.close();
        };
        socket.onerror = socket.onclose = () => { void failed(); };
      } catch { exit(""); }
    })();
    return {
      send(data) { if (active && opened && !exited && options.authenticated()) socket?.send(new TextEncoder().encode(data)); },
      resize(nextCols, nextRows) {
        if (!Number.isInteger(nextCols) || !Number.isInteger(nextRows) || nextCols < 1 || nextCols > 1000 || nextRows < 1 || nextRows > 1000) return;
        cols = nextCols; rows = nextRows;
        if (active && opened && !exited) socket?.send(JSON.stringify({ type: "resize", cols, rows } satisfies ShellResize));
      },
      close,
    };
  };
  return { open, reset() { for (const close of [...sessions]) close(); } };
}
