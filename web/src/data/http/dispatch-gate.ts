// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original physical dispatch gate for hub maintenance holds and unavailable-host skips (#389, #391).

import { MAINTENANCE_CODE, problemCode } from "./problem-code";
import { READ_TAG_HEADER, SKIPPED_HEADER, type HostReadPolicy, type ReadTag } from "./host-read-policy";
import type { MaintenanceObserver } from "./maintenance-observer";

/** A held-class answer that met maintenance is sent again after the hold, at most this often. */
const MAINTENANCE_RETRIES = 2;
// Read-only POST projections that are safe to send again; every other write is sent once and never replayed.
const READ_PROJECTIONS = [/\/environments\/[^/]+\/images\/vulnerabilities\/(active-)?summaries$/];
const NEVER_HELD = [/\/backups\/maintenance$/, /\/auth\/(login|logout)$/];

function json(status: number, body: object, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

/** The unavailable answer readers already map to retained rows; produced without a request. */
export function skippedResponse(): Response {
  return json(502, { success: false, code: "agent.unavailable", data: { reason: "host.skipped" } }, { [SKIPPED_HEADER]: "host" });
}

function maintenanceResponse(): Response {
  return json(503, { status: 503, detail: MAINTENANCE_CODE }, { "Content-Type": "application/problem+json" });
}

async function isMaintenance(response: Response): Promise<boolean> {
  if (response.status !== 503) return false;
  try { return problemCode(await response.clone().json()) === MAINTENANCE_CODE; } catch { return false; }
}

/** Wraps the base fetch below host admission, so admitted, queued and bypassing requests all pass it immediately
 * before the network. Maintenance holds replay-safe reads; tagged background reads of an unavailable host are
 * skipped. Both honour the request's own abort signal, which identity reads link to identity cancellation. */
export function createDispatchGate(fetcher: typeof fetch, baseUrl: string, maintenance: MaintenanceObserver, hosts: HostReadPolicy): typeof fetch {
  const base = new URL(baseUrl, "https://hub.example.com");
  const prefix = `${base.pathname.replace(/\/$/, "")}/environments/`;
  return async (input, init) => {
    const request = input instanceof Request ? input : undefined;
    const url = new URL(request?.url ?? String(input), base);
    const method = (init?.method ?? request?.method ?? "GET").toUpperCase();
    const signal = init?.signal ?? request?.signal ?? undefined;
    const headers = new Headers(init?.headers ?? request?.headers);
    const tag = headers.get(READ_TAG_HEADER) as ReadTag | null;
    let options = init;
    if (tag) { headers.delete(READ_TAG_HEADER); options = { ...init, headers }; }
    const suffix = url.pathname.startsWith(prefix) ? url.pathname.slice(prefix.length) : "";
    const host = suffix.includes("/") ? suffix.split("/")[0] : undefined;
    const read = method === "GET" || method === "HEAD" || method === "POST" && READ_PROJECTIONS.some((pattern) => pattern.test(url.pathname));
    const held = read && !NEVER_HELD.some((pattern) => pattern.test(url.pathname));
    const send = () => fetcher(request ? request.clone() : input, options);
    for (let attempt = 0; ; ++attempt) {
      if (held && maintenance.holding()) {
        if (await maintenance.wait(signal) === "overflow") return maintenanceResponse();
      }
      signal?.throwIfAborted();
      if (tag && host && (tag === "background" || tag === "poll") && hosts.skip(host, tag)) return skippedResponse();
      const response = await send();
      if (!await isMaintenance(response)) return response;
      maintenance.signal();
      // An overdue hold sends nothing again: the request surfaces its own answer.
      if (!held || attempt >= MAINTENANCE_RETRIES || !maintenance.holding()) return response;
      void response.arrayBuffer().catch(() => undefined);
    }
  };
}
