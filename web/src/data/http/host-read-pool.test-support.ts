// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original six-connection HTTP/1.1 fetch scheduler with a persistent stream connection, dense Dashboard timing profile and the hub's bounded unavailable answer. C2 #271: retained snapshot answers and changed fresh Direct rows.

import { vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { securityFixture } from "./vulnerabilities.test-support";

/** Physical requests above six wait FIFO, including healthy hosts; abort removes queued or held work. */
export function hostReadPool({ dense = false, holdDirect, hubBound = false, hubSnapshot = false }: { dense?: boolean; holdDirect?: boolean; hubBound?: boolean; hubSnapshot?: boolean } = {}) {
  const fixture = securityFixture(); fixture.inventory.state.hostIds = ["direct", "0", "edge"];
  const original = fixture.fetcher.getMockImplementation()!;
  let active = 0, maximum = 0, held = holdDirect ?? !dense;
  const queue: (() => void)[] = [], heldReads = new Set<() => void>();
  const sent: string[] = [], waiting: string[] = [];
  const activeHosts = new Map<string, number>(), maximumHosts = new Map<string, number>();
  const reserve = () => { maximum = Math.max(maximum, ++active); };
  const release = () => { --active; queue.shift()?.(); };
  const fetcher: typeof fetch = vi.fn((input, init) => new Promise<Response>((resolve, reject) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname;
    const host = path.match(/\/environments\/([^/]+)\//)?.[1];
    let started = false, finished = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const finish = () => {
      if (finished) return;
      finished = true;
      init?.signal?.removeEventListener("abort", abort); clearTimeout(timer); heldReads.delete(answer);
      if (started) { if (host) activeHosts.set(host, activeHosts.get(host)! - 1); release(); }
      else { const index = queue.indexOf(start); if (index >= 0) queue.splice(index, 1); }
    };
    const abort = () => { finish(); reject(init?.signal?.reason); };
    const answer = (snapshot = false) => {
      clearTimeout(timer); heldReads.delete(answer);
      timer = setTimeout(() => { void (async () => {
        try {
          const response = await original(input, init), body = await response.json();
          if (path === "/api/environments") body.data = body.data.map((h: { id: string }) => ({ ...h, isEdge: h.id === "edge" }));
          if (body.pagination && Array.isArray(body.data) && /\/(projects|containers|images|networks|volumes)$/.test(path)) {
            const sample = body.data[0];
            // Three real pages exercise the remainder of each host's original logical budget.
            const total = hubSnapshot && host === "direct" ? 1 : dense && host === "0" ? path.endsWith("/projects") ? 21 : /\/(containers|images)$/.test(path) ? 336 : 201 : 201;
            const start = Number(url.searchParams.get("start") ?? 0);
            body.data = sample ? Array.from({ length: Math.min(100, total - start) }, (_, offset) => {
              const index = start + offset;
              return { ...sample, id: `${sample.id}-${index}`, ...(typeof sample.name === "string" ? { name: `${sample.name}-${index}` } : {}), ...(sample.names ? { names: [`/${hubSnapshot && host === "direct" ? snapshot ? "snapshot" : "fresh" : "web"}-${index}`], state: snapshot ? "exited" : sample.state } : {}) };
            }) : [];
            body.pagination.totalItems = sample ? total : 0;
          }
          const headers = new Headers(response.headers);
          if (snapshot) headers.set("X-Docklet-Host-Reason", "agent.unavailable");
          const bytes = new TextEncoder().encode(JSON.stringify(body)); let delivered = false;
          resolve(new Response(new ReadableStream<Uint8Array>({
            pull(controller) { if (!delivered) { delivered = true; controller.enqueue(bytes); } else { controller.close(); finish(); } },
            cancel() { finish(); },
          }, { highWaterMark: 0 }), { status: response.status, headers }));
        } catch (error) { finish(); reject(error); }
      })(); }, dense && host === "0" ? url.searchParams.get("sort") === "cpuUsage" ? 11_500 : 2_000 : 250);
    };
    const start = () => {
      started = true; reserve(); sent.push(path + url.search);
      if (host) { const count = (activeHosts.get(host) ?? 0) + 1; activeHosts.set(host, count); maximumHosts.set(host, Math.max(maximumHosts.get(host) ?? 0, count)); }
      if (host === "direct" && held && (!hubSnapshot || /\/(projects|containers|images|networks|volumes)$/.test(path))) {
        heldReads.add(answer);
        // Like the hub's 20 s inventory bound, a paused agent's read is answered as unavailable.
        if (hubBound) timer = setTimeout(() => {
          heldReads.delete(answer);
          if (hubSnapshot && /\/(projects|containers|images|networks|volumes)$/.test(path)) answer(true);
          else { resolve(Response.json({ success: false, code: "agent.unavailable" }, { status: 502 })); finish(); }
        }, 20_000);
      } else answer();
    };
    init?.signal?.addEventListener("abort", abort, { once: true });
    if (init?.signal?.aborted) abort();
    else if (active < 6) start();
    else { waiting.push(path + url.search); queue.push(start); }
  }));
  const source = fixture.source;
  const adapter = createHttpAdapter({ fetch: fetcher, webSocket: fixture.inventory.webSocket, eventSource: () => {
    reserve(); source.close = () => release();
    return source;
  } });
  return {
    adapter, sent, waiting, maximumHosts, activeHosts,
    get maximum() { return maximum; },
    resume() { held = false; [...heldReads].forEach((answer) => answer()); },
  };
}
