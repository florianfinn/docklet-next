// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

import { CancelledError } from "@tanstack/react-query";

const releases = new WeakMap<Response, () => void>();

/** Status-only callers discard unread data and release request ownership immediately. */
export function discardResponse(response: Response): void {
  releases.get(response)?.();
  releases.delete(response);
  void response.body?.cancel().catch(() => undefined);
}

/** Abort owned requests and guard bodies even when a fetch implementation ignores abort. */
export function createIdentityReads(fetcher: typeof fetch) {
  let generation = 0;
  const owned = new Set<AbortController>();
  const check = (version: number) => { if (version !== generation) throw new CancelledError(); };
  const fetch: typeof globalThis.fetch = async (input, init) => {
    // The public startup policy and CSRF bootstrap contain no session data.
    if (new URL(input instanceof Request ? input.url : String(input), (typeof window === "undefined" ? "https://hub.example.com" : window.location.href)).pathname.endsWith("/auth/mode")) return fetcher(input, init);
    const version = generation;
    const controller = new AbortController();
    const signal = init?.signal ?? (input instanceof Request ? input.signal : undefined);
    const abort = () => controller.abort();
    if (signal?.aborted) abort(); else signal?.addEventListener("abort", abort, { once: true });
    owned.add(controller);
    const release = () => { owned.delete(controller); signal?.removeEventListener("abort", abort); };
    try {
      const response = await fetcher(input, { ...init, signal: controller.signal });
      check(version);
      // Keep cancellation ownership through body consumption; streams are owned separately.
      const wrap = (value: Response): Response => {
        const wrapped = new Proxy(value, {
          get(target, prop) {
            if (prop === "clone") return () => wrap(target.clone());
            const member: unknown = Reflect.get(target, prop, target);
            if (["json", "text", "arrayBuffer", "blob", "formData", "bytes"].includes(String(prop)) && typeof member === "function") return async () => {
              try { const body: unknown = await member.call(target); check(version); return body; } finally { release(); }
            };
            return typeof member === "function" ? member.bind(target) : member;
          },
        });
        releases.set(wrapped, release);
        return wrapped;
      };
      return wrap(response);
    } catch (error) { release(); check(version); throw error; }
  };
  return { fetch, reset() { ++generation; for (const controller of owned) controller.abort(); owned.clear(); } };
}
