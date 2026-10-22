// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original existing-WS log mapping with arrival identities.

import type { LogLine } from "@/domain/model";
import { boundLogLine } from "../log-buffer";
import { splitInventoryId } from "./inventory-mapping";

export type LogKind = "stack" | "container";
export const LOG_OPTIONS = { format: "json", follow: "true", tail: "100", timestamps: "true", batched: "false" } as const;
export function logAddress(baseUrl: string, kind: LogKind, id: string) {
  const [host, raw] = splitInventoryId(id);
  if (host.length > 1024 || raw.length > 1024) throw new Error("logs.invalid-resource");
  const url = new URL(`${baseUrl}/environments/${encodeURIComponent(host)}/ws/${kind === "stack" ? "projects" : "containers"}/${encodeURIComponent(raw)}/logs`, window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.search = new URLSearchParams(LOG_OPTIONS).toString();
  return { url: url.href, host, raw, key: JSON.stringify([host, kind, raw, LOG_OPTIONS]) };
}
const streams = new Set(["stdout", "stderr", "combined", "unknown"]);
const levels = new Set(["info", "warn", "error", "debug"]);
function wireTimestamp(value: unknown): string | undefined {
  if (typeof value !== "string" || value.length > 64) return;
  const parts = /^(\d{4})-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])T([01]\d|2[0-3]):([0-5]\d):([0-5]\d)(?:\.(\d+))?(Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.exec(value);
  if (!parts) return;
  const year = Number(parts[1]), month = Number(parts[2]), day = Number(parts[3]);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  if (day > [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1]!) return;
  // ECMAScript guarantees millisecond ISO parsing, not RFC3339 nanoseconds.
  // Validate the full wire value first, then truncate only its display precision.
  return `${value.slice(0, 19)}.${(parts[7] ?? "").padEnd(3, "0").slice(0, 3)}${parts[8]}`;
}
function text(value: unknown, max = 1024): value is string { return typeof value === "string" && value.length <= max; }
export function mapLog(value: unknown, scope: string, attempt: number, fallbackService: string, now: () => string): LogLine {
  const v = value as Record<string, unknown> | null;
  const timestamp = wireTimestamp(v?.timestamp);
  if (!v || typeof v !== "object" || Array.isArray(v) || !Number.isSafeInteger(v.seq) || (v.seq as number) < 1 || typeof v.message !== "string" ||
      (v.level !== undefined && !text(v.level, 64)) || (v.timestamp !== undefined && timestamp === undefined) ||
      (v.service !== undefined && !text(v.service)) || (v.containerId !== undefined && !text(v.containerId)) ||
      (v.stream !== undefined && (typeof v.stream !== "string" || !streams.has(v.stream))) || (v.truncated !== undefined && typeof v.truncated !== "boolean")) throw new Error("logs.invalid-response");
  return boundLogLine({
    id: JSON.stringify([scope, attempt, v.seq]), seq: v.seq as number,
    ts: timestamp ?? now(),
    service: (v.service || v.containerId || fallbackService) as string,
    level: levels.has(v.level as string) ? v.level as LogLine["level"] : "info",
    message: v.message, stream: (v.stream ?? "unknown") as NonNullable<LogLine["stream"]>, truncated: v.truncated === true,
  });
}
