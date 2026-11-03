// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original runtime history and profile preference transport for issues #335, #336, #337 and #339.

import type { components } from "@/api/generated/schema";
import { HISTORY_BUCKETS, HISTORY_RANGES, type HistoryApi, type HistoryBucket, type HistoryRange, type HostCapacity, type RuntimeHistory } from "@/domain/history";
import { isUiSection, type UiPreferencesApi, type UiPreferencesDocument } from "@/domain/ui-preferences";
import { AdapterOutcomeError } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";

type WireHistory = components["schemas"]["HistoryResponse"];
type WireBucket = components["schemas"]["HistoryBucket"];
type WireCapacity = components["schemas"]["HistoryCapacity"];

/** Typed hub problems are plain `{code, status}`; enveloped errors carry the code under `data`. */
function failure(body: unknown, status?: number): AdapterOutcomeError {
  const envelope = body && typeof body === "object" ? body as Record<string, unknown> : {};
  const object = envelope.data && typeof envelope.data === "object" ? envelope.data as Record<string, unknown> : envelope;
  return new AdapterOutcomeError({ kind: "error", code: typeof object.code === "string" ? object.code : "http.error", status });
}

const value = (n: number | null | undefined) => (typeof n === "number" && Number.isFinite(n) ? n : undefined);
const average = (values: number[]) => (values.length ? values.reduce((n, v) => n + v, 0) / values.length : undefined);

function mapBucket(row: WireBucket, minutes: number): HistoryBucket {
  const start = Date.parse(row.start);
  if (!Number.isFinite(start) || !row.minutes) throw failure({ code: "history.invalid-response" });
  const cpuAvg = value(row.cpuAverage), cpuMax = value(row.cpuMax), memAvg = value(row.memoryAverage), memMax = value(row.memoryMax);
  return {
    start: row.start,
    end: new Date(start + minutes * 60_000).toISOString(),
    state: row.state,
    minutes: { running: row.minutes.running, unhealthy: row.minutes.unhealthy, down: row.minutes.down, stopped: row.minutes.stopped },
    ...(row.partial ? { partial: true } : {}),
    ...(cpuAvg === undefined ? {} : { cpuAvg }),
    ...(cpuMax === undefined ? {} : { cpuMax }),
    ...(memAvg === undefined ? {} : { memAvg }),
    ...(memMax === undefined ? {} : { memMax }),
    ...(row.memoryLimit > 0 ? { memLimit: row.memoryLimit } : {}),
    ...(row.state === "down" || row.state === "stopped" ? { exitCode: row.exitCode } : {}),
  };
}

/** Maps a hub history response; aggregates cover buckets with a measurement, traffic sums the per-bucket deltas. */
export function mapHistory(wire: WireHistory, range: HistoryRange): RuntimeHistory {
  if (!wire || wire.range !== range || !HISTORY_RANGES.includes(wire.range)) throw failure({ code: "history.invalid-response" });
  const rows = wire.buckets ?? [];
  const buckets = rows.map((row) => mapBucket(row, HISTORY_BUCKETS[range].minutes));
  const measured = buckets.filter((b) => b.cpuAvg !== undefined);
  const memory = buckets.filter((b) => b.memAvg !== undefined);
  const peak = (values: (number | undefined)[]) => { const known = values.filter((v): v is number => v !== undefined); return known.length ? Math.max(...known) : undefined; };
  const sum = (key: "networkRx" | "networkTx" | "blockRead" | "blockWrite") => rows.reduce((n, row) => n + (value(row[key]) ?? 0), 0);
  const uptime = value(wire.uptimePercent);
  return {
    range,
    buckets,
    uptimePercent: uptime ?? null,
    ...(measured.length ? { cpuAvg: average(measured.map((b) => b.cpuAvg!)), cpuMax: peak(measured.map((b) => b.cpuMax ?? b.cpuAvg)), cpuNow: measured.at(-1)!.cpuAvg } : {}),
    ...(memory.length ? { memAvg: average(memory.map((b) => b.memAvg!)), memMax: peak(memory.map((b) => b.memMax ?? b.memAvg)) } : {}),
    ...(buckets.findLast((b) => b.memLimit)?.memLimit ? { memLimit: buckets.findLast((b) => b.memLimit)!.memLimit } : {}),
    totals: { netRx: sum("networkRx"), netTx: sum("networkTx"), diskRead: sum("blockRead"), diskWrite: sum("blockWrite") },
  };
}

/** Capacity keeps the hub's growth estimate; the line uses the 30-day buckets measured on the current disk source. */
export function mapCapacity(wire: WireCapacity): HostCapacity {
  if (!wire?.hostId || !Number.isFinite(wire.totalBytes) || !Number.isFinite(wire.usedBytes)) throw failure({ code: "history.invalid-response" });
  const growth = value(wire.growthBytesPerDay), days = value(wire.daysUntilFull);
  const minutes = HISTORY_BUCKETS["30d"].minutes;
  const series = (wire.history?.buckets ?? [])
    .filter((row) => row.diskTotal > 0 && row.diskSource === wire.diskSource && Number.isFinite(Date.parse(row.start)))
    .map((row) => ({ at: new Date(Date.parse(row.start) + minutes * 60_000).toISOString(), used: row.diskUsed }));
  return {
    hostId: wire.hostId,
    diskTotal: wire.totalBytes,
    diskUsed: wire.usedBytes,
    ...(!wire.growthUnavailable && growth !== undefined ? { growthPerDay: growth } : {}),
    ...(!wire.growthUnavailable && days !== undefined ? { daysUntilFull: days } : {}),
    series,
  };
}

function mapDocument(wire: components["schemas"]["PreferencesDocument"]): UiPreferencesDocument {
  if (!wire || !Number.isInteger(wire.revision) || !wire.sections || typeof wire.sections !== "object") throw failure({ code: "preferences.invalid-response" });
  // Unknown sections from a newer hub stay on the server; the web only reads its own.
  return { revision: wire.revision, sections: Object.fromEntries(Object.entries(wire.sections).filter(([key]) => isUiSection(key))) };
}

async function request<T>(client: CsrfClient, url: string, init: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await client.fetch(url, { credentials: "same-origin", cache: "no-store", ...init, headers: { Accept: "application/json", ...(init.body ? { "Content-Type": "application/json" } : {}) } });
  } catch (error) { if (error instanceof DOMException && error.name === "AbortError") throw error; throw failure(null); }
  let body: unknown;
  try { body = await response.json(); } catch { throw failure(null, response.status); }
  if (!response.ok) throw failure(body, response.status);
  const envelope = body as { success?: unknown; data?: unknown };
  if (envelope?.success !== true || !("data" in envelope)) throw failure({ code: "history.invalid-response" }, response.status);
  return envelope.data as T;
}

export function createHttpHistory(baseUrl: string, client: CsrfClient): HistoryApi {
  const host = (id: string) => `${baseUrl}/environments/${encodeURIComponent(id)}`;
  return {
    async getContainerHistory({ hostId, name }, range) {
      return mapHistory(await request<WireHistory>(client, `${host(hostId)}/containers/${encodeURIComponent(name)}/history?range=${range}`), range);
    },
    async getStackHistory(stackId, range) {
      const [hostId, projectId] = splitInventoryId(stackId);
      return mapHistory(await request<WireHistory>(client, `${host(hostId)}/projects/${encodeURIComponent(projectId)}/history?range=${range}`), range);
    },
    async getCapacity() {
      const rows = await request<WireCapacity[] | null>(client, `${baseUrl}/history/capacity`);
      return (rows ?? []).map(mapCapacity);
    },
  };
}

export function createHttpUiPreferences(baseUrl: string, client: CsrfClient): UiPreferencesApi {
  return {
    async getUiPreferences() {
      return mapDocument(await request(client, `${baseUrl}/auth/me/ui-preferences`));
    },
    async putUiPreference(section, sectionValue) {
      return mapDocument(await request(client, `${baseUrl}/auth/me/ui-preferences/${encodeURIComponent(section)}`, { method: "PUT", body: JSON.stringify({ value: sectionValue }) }));
    },
  };
}
