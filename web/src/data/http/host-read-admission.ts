// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original FIFO physical host GET admission through response-body completion.

const BODY_METHODS = new Set(["json", "text", "arrayBuffer", "blob", "formData", "bytes"]);

/** Keep the native body methods: Chromium records a completed no-store body read through a JS stream reader as aborted
 * (#326), so the slot is released when a body method settles, the body is cancelled or a caller's own reader closes. */
function admitted(response: Response, release: () => void): Response {
  return new Proxy(response, {
    get(target, prop) {
      if (prop === "clone") return () => admitted(target.clone(), release);
      const member: unknown = Reflect.get(target, prop, target);
      if (BODY_METHODS.has(String(prop)) && typeof member === "function") return async () => {
        try { return await (member as () => Promise<unknown>).call(target); } finally { release(); }
      };
      if (prop === "body" && member instanceof ReadableStream) return new Proxy(member, {
        get(stream, field) {
          if (field === "cancel") return async (reason?: unknown) => { try { await stream.cancel(reason); } finally { release(); } };
          if (field === "getReader") return (...args: unknown[]) => {
            const reader = (stream.getReader as (...a: unknown[]) => ReadableStreamDefaultReader)(...args);
            void reader.closed.then(release, release);
            return reader;
          };
          const value: unknown = Reflect.get(stream, field, stream);
          return typeof value === "function" ? (value as (...a: unknown[]) => unknown).bind(stream) : value;
        },
      });
      return typeof member === "function" ? (member as (...a: unknown[]) => unknown).bind(target) : member;
    },
  });
}

/** HTTP/1.1 has six connections per origin; two per host plus the stream leaves room for other hosts. Downloads are
 * not admitted, so a long transfer never holds a host's slots. */
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
    if (method !== "GET" || !host || url.pathname.endsWith("/download") || headers.get("Accept")?.includes("text/event-stream")) return fetcher(input, init);
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
        // A caller that never reads its body releases the slot when its request is aborted.
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
      return admitted(response, release);
    } catch (error) { release(); throw error; }
  };
}
