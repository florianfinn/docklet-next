// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original FIFO physical host GET admission through response-body completion.

/** HTTP/1.1 has six connections per origin; two per host plus the stream leaves room for other hosts. */
export function createHostReadAdmission(fetcher: typeof fetch, baseUrl = "/api"): typeof fetch {
  const base = new URL(baseUrl, "https://hub.example.com");
  const prefix = `${base.pathname.replace(/\/$/, "")}/environments/`;
  const hosts = new Map<string, { active: number; queue: (() => void)[] }>();
  return async (input, init) => {
    const request = input instanceof Request ? input : undefined;
    const url = new URL(request?.url ?? String(input), base);
    const method = (init?.method ?? request?.method ?? "GET").toUpperCase();
    const suffix = url.pathname.startsWith(prefix) ? url.pathname.slice(prefix.length) : "";
    const host = suffix.includes("/") ? suffix.split("/")[0] : undefined;
    const headers = new Headers(init?.headers ?? request?.headers);
    if (method !== "GET" || !host || headers.get("Accept")?.includes("text/event-stream")) return fetcher(input, init);
    const key = `${url.origin}/${host}`;
    const signal = init?.signal ?? request?.signal;
    signal?.throwIfAborted();
    const state = hosts.get(key) ?? { active: 0, queue: [] };
    hosts.set(key, state);
    let release!: () => void;
    await new Promise<void>((resolve, reject) => {
      const cleanup = () => { if (!state.active && !state.queue.length) hosts.delete(key); };
      const start = () => {
        signal?.removeEventListener("abort", abort);
        ++state.active;
        let released = false;
        release = () => {
          if (released) return;
          released = true;
          signal?.removeEventListener("abort", release);
          --state.active;
          state.queue.shift()?.();
          cleanup();
        };
        signal?.addEventListener("abort", release, { once: true });
        resolve();
      };
      const abort = () => {
        const index = state.queue.indexOf(start);
        if (index >= 0) state.queue.splice(index, 1);
        signal?.removeEventListener("abort", abort);
        cleanup(); reject(signal?.reason);
      };
      if (state.active < 2) start();
      else { state.queue.push(start); signal?.addEventListener("abort", abort, { once: true }); }
    });
    try {
      signal?.throwIfAborted();
      const response = await fetcher(input, init);
      if (!response.body || response.headers.get("Content-Type")?.includes("text/event-stream")) { release(); return response; }
      const reader = response.body.getReader();
      void reader.closed.then(release, release);
      const body = new ReadableStream<Uint8Array>({
        async pull(controller) {
          try {
            const chunk = await reader.read();
            if (chunk.done) { release(); controller.close(); }
            else controller.enqueue(chunk.value);
          } catch (error) { release(); controller.error(error); }
        },
        async cancel(reason) { try { await reader.cancel(reason); } finally { release(); } },
      }, { highWaterMark: 0 });
      const wrapped = new Response(body, { status: response.status, statusText: response.statusText, headers: response.headers });
      // Preserve transport metadata while native body methods and clone share the guarded stream.
      return new Proxy(wrapped, { get(target, prop) {
        if (prop === "url" || prop === "redirected" || prop === "type") return Reflect.get(response, prop, response);
        const value: unknown = Reflect.get(target, prop, target);
        return typeof value === "function" ? value.bind(target) : value;
      } });
    } catch (error) { release(); throw error; }
  };
}
