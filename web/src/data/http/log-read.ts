// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import type { LogLine } from "@/domain/model";
import type { LogsApi, LogReadResult } from "../api";
import type { CapabilityDescriptor, ResourceRef } from "../capability";
import { logAddress, mapLog } from "./log-mapping";
import type { MetricsSocket } from "./metrics";

interface ReadTransport {
  baseUrl: string;
  socket: (url: string) => MetricsSocket;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  authenticated: () => boolean;
  admit?: () => Promise<void>;
}
export function createLogReads(transport: ReadTransport) {
  const reads = new Set<AbortController>();
  let attempt = 0;
  const readLogs: LogsApi["readLogs"] = (ref, options, sink, signal) => new Promise((resolve) => {
    const controller = new AbortController();
    reads.add(controller);
    let socket: (MetricsSocket & { onopen?: ((event: Event) => void) | null }) | undefined;
    let done = false, opened = false, lines = 0, bytes = 0;
    const finish = (reason: LogReadResult["reason"]) => {
      if (done) return;
      done = true;
      signal.removeEventListener("abort", abort);
      reads.delete(controller);
      if (socket) { socket.onopen = socket.onmessage = socket.onerror = socket.onclose = null; socket.close(); }
      resolve({ reason, lines, bytes });
    };
    const abort = () => finish("aborted");
    controller.signal.addEventListener("abort", abort, { once: true });
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) { abort(); return; }
    void (async () => {
      try {
        const capability = await transport.capability(ref.kind === "stack" ? "subscribeLogs" : "subscribeContainerLogs", ref);
        await transport.admit?.();
        if (done) return;
        if (!capability.available || !transport.authenticated()) { finish("unavailable"); return; }
        const address = logAddress(transport.baseUrl, ref.kind, ref.id);
        const url = new URL(address.url);
        url.searchParams.set("follow", "false");
        url.searchParams.delete("tail");
        for (const key of ["tail", "since", "until"] as const) if (options[key] !== undefined) url.searchParams.set(key, String(options[key]));
        if (options.maxLines <= 0) { finish("cap-lines"); return; }
        if (options.maxBytes <= 0) { finish("cap-bytes"); return; }
        const identity = ++attempt;
        socket = transport.socket(url.href);
        socket.onopen = () => { opened = true; };
        socket.onclose = socket.onerror = () => finish(opened ? "end" : "unavailable");
        socket.onmessage = (event) => {
          if (done) return;
          if (!transport.authenticated()) { abort(); return; }
          try {
            const frame: unknown = JSON.parse(String(event.data));
            const batch: LogLine[] = [];
            let reason: LogReadResult["reason"] | undefined;
            for (const value of Array.isArray(frame) ? frame : [frame]) {
              const line = mapLog(value, address.key, identity, ref.kind === "container" ? address.raw : "", () => new Date().toISOString(), false);
              const at = Date.parse(line.ts);
              if ((options.since && at < Date.parse(options.since)) || (options.until && at > Date.parse(options.until))) continue;
              const size = new TextEncoder().encode(line.message).length;
              if (bytes + size > options.maxBytes) { reason = "cap-bytes"; break; }
              batch.push(line); lines++; bytes += size;
              if (lines >= options.maxLines) { reason = "cap-lines"; break; }
              if (bytes >= options.maxBytes) { reason = "cap-bytes"; break; }
            }
            if (batch.length) sink(batch);
            if (reason) finish(reason);
          } catch { finish("unavailable"); }
        };
      } catch { finish("unavailable"); }
    })();
  });
  return { readLogs, reset() { for (const read of reads) read.abort(); reads.clear(); } };
}
