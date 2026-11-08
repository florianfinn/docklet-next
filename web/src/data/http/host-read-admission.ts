// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original fair HTTP/1.1 physical host GET admission through response-body completion; multiplexed protocols bypass admission.

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

type WaitingRead = { order: number; start: () => void };
type HostReads = { active: number; held: boolean; queue: WaitingRead[] };

function navigationProtocol(): string {
  return typeof performance !== "undefined" && typeof performance.getEntriesByType === "function"
    ? (performance.getEntriesByType("navigation")[0] as PerformanceNavigationTiming | undefined)?.nextHopProtocol ?? "" : "";
}

export function createHostReadAdmission(fetcher: typeof fetch, baseUrl = "/api", protocol: () => string = navigationProtocol): typeof fetch {
  const base = new URL(baseUrl, "https://hub.example.com");
  const prefix = `${base.pathname.replace(/\/$/, "")}/environments/`;
  const hosts = new Map<string, HostReads>();
  // Six HTTP/1.1 connections per origin minus the persistent stream leave five host reads.
  const limit = 5;
  let active = 0, order = 0;
  const cleanup = (key: string, state: HostReads) => { if (!state.active && !state.queue.length && !state.held) hosts.delete(key); };
  function drain() {
    while (active < limit) {
      let next: HostReads | undefined;
      for (const state of hosts.values()) {
        if (!state.queue.length || state.held && state.active >= 1) continue;
        if (!next || state.active < next.active || state.active === next.active && state.queue[0]!.order < next.queue[0]!.order) next = state;
      }
      if (!next) break;
      next.queue.shift()!.start();
    }
  }
  return async (input, init) => {
    const request = input instanceof Request ? input : undefined;
    const url = new URL(request?.url ?? String(input), base);
    const method = (init?.method ?? request?.method ?? "GET").toUpperCase();
    const suffix = url.pathname.startsWith(prefix) ? url.pathname.slice(prefix.length) : "";
    const host = suffix.includes("/") ? suffix.split("/")[0] : undefined;
    const headers = new Headers(init?.headers ?? request?.headers);
    if (["h2", "h3"].includes(protocol()) || method !== "GET" || !host || url.pathname.endsWith("/download") || headers.get("Accept")?.includes("text/event-stream")) return fetcher(input, init);
    const key = `${url.origin}/${host}`;
    const signal = init?.signal ?? request?.signal;
    signal?.throwIfAborted();
    const state = hosts.get(key) ?? { active: 0, held: false, queue: [] };
    hosts.set(key, state);
    let release!: () => void;
    let receivedHeaders = false;
    await new Promise<void>((resolve, reject) => {
      const entry: WaitingRead = { order: order++, start: () => {
        signal?.removeEventListener("abort", abort);
        ++active; ++state.active;
        const startedAt = Date.now();
        let released = false;
        const abortActive = () => {
          // Ten seconds without headers distinguishes held transport from ordinary short cancellation.
          if (!released && !receivedHeaders && Date.now() - startedAt >= 10_000) state.held = true;
          release();
        };
        release = () => {
          if (released) return;
          released = true;
          signal?.removeEventListener("abort", abortActive);
          --active; --state.active;
          cleanup(key, state); drain();
        };
        // A caller that never reads its body releases the slot when its request is aborted.
        signal?.addEventListener("abort", abortActive, { once: true });
        resolve();
      } };
      const abort = () => {
        const index = state.queue.indexOf(entry);
        if (index >= 0) state.queue.splice(index, 1);
        signal?.removeEventListener("abort", abort);
        cleanup(key, state); reject(signal?.reason);
      };
      state.queue.push(entry); signal?.addEventListener("abort", abort, { once: true }); drain();
    });
    try {
      signal?.throwIfAborted();
      const response = await fetcher(input, init);
      receivedHeaders = true;
      const current = hosts.get(key);
      if (current) { current.held = false; cleanup(key, current); drain(); }
      if (!response.body || response.headers.get("Content-Type")?.includes("text/event-stream")) { release(); return response; }
      return admitted(response, release);
    } catch (error) { release(); throw error; }
  };
}
