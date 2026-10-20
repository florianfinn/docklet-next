// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original bounded log snapshots and presentation metadata.

import type { LogLine } from "@/domain/model";

export const LOG_MAX_LINES = 2000;
export const LOG_MAX_BYTES = 1024 * 1024;
export const LOG_LINE_BYTES = 64 * 1024;
// Published React snapshots and the mutable next-frame window share the budget.
// Each half is bounded even while animation frames are suspended indefinitely.
const WINDOW_LINES = LOG_MAX_LINES / 2;
const WINDOW_BYTES = LOG_MAX_BYTES / 2;
const encoder = new TextEncoder();
export const logBytes = (line: LogLine) => encoder.encode(JSON.stringify(line)).byteLength;
export type LogStatus = "connecting" | "live" | "retrying" | "offline" | "unavailable" | "error" | "unauthenticated";
export interface LogDelivery {
  status: LogStatus;
  reconnect: boolean;
  dropped: number;
  retry?: () => void;
}
const deliveries = new WeakMap<LogLine[], LogDelivery>();
const gaps = new WeakMap<LogLine, number>();
export function logDelivery(lines: LogLine[]) { return deliveries.get(lines); }
export function setLogDelivery(lines: LogLine[], delivery: LogDelivery) { deliveries.set(lines, delivery); return lines; }
export function logGap(line: LogLine) { return gaps.get(line); }
export function gapLine(id: string, ts: string, count: number): LogLine {
  const line: LogLine = { id, ts, service: "", level: "warn", message: "" };
  gaps.set(line, count);
  return line;
}

export function boundLogLine(line: LogLine): LogLine {
  if (line.message.length > LOG_LINE_BYTES) line = { ...line, message: line.message.slice(0, LOG_LINE_BYTES), truncated: true };
  if (logBytes(line) <= LOG_LINE_BYTES) return line;
  // Metadata is validated/bounded at the wire boundary; the message consumes
  // the remaining serialized budget, including JSON escapes and UTF-8 bytes.
  const bounded = { ...line, truncated: true };
  let lo = 0, hi = Math.min(line.message.length, LOG_LINE_BYTES);
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    bounded.message = line.message.slice(0, mid);
    if (logBytes(bounded) <= LOG_LINE_BYTES) lo = mid; else hi = mid - 1;
  }
  if (lo && /[\uD800-\uDBFF]/.test(line.message[lo - 1]!)) --lo;
  bounded.message = line.message.slice(0, lo);
  if (logBytes(bounded) > LOG_LINE_BYTES) throw new Error("logs.invalid-metadata");
  return bounded;
}

export class LogBuffer {
  private rows: { line: LogLine; bytes: number }[] = [];
  bytes = 0;
  dropped = 0;
  append(value: LogLine) {
    const line = boundLogLine(value), bytes = logBytes(line);
    this.rows.push({ line, bytes }); this.bytes += bytes;
    while (this.rows.length > WINDOW_LINES || this.bytes > WINDOW_BYTES) {
      this.bytes -= this.rows.shift()!.bytes;
      ++this.dropped;
    }
  }
  snapshot(delivery: Omit<LogDelivery, "dropped">): LogLine[] {
    return setLogDelivery(this.rows.map(({ line }) => line), { ...delivery, dropped: this.dropped });
  }
  clear() { this.rows = []; this.bytes = 0; this.dropped = 0; }
}

// Subscription functions survive the RunGate proxy unchanged. This log-only
// lifetime signal also clears a paused view after its transport unsubscribes.
export interface LogLifetime { version: () => number; listen: (notify: () => void) => () => void }
const lifetimes = new WeakMap<object, LogLifetime>();
export const NO_LOG_LIFETIME: LogLifetime = { version: () => 0, listen: () => () => {} };
export function registerLogLifetime(subscribe: object, lifetime: LogLifetime) { lifetimes.set(subscribe, lifetime); }
export function logLifetime(subscribe: object) { return lifetimes.get(subscribe) ?? NO_LOG_LIFETIME; }
