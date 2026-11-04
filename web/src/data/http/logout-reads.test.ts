// Source: docklet-next/issues/340
// Revision: 4155c8fe73dc2c562ddd41075b20e0304f879f5b
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original test of background reads around a logout through the real HTTP adapter (M17 acceptance M17-A2).

import { expect, it } from "vitest";

import { createHttpAdapter } from "./http-adapter";

function deferred<T>() {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((r) => { resolve = r; }), resolve };
}
const flush = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };
// CSRF header value of the session.
const CSRF = "a".repeat(43);
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "Content-Type": "application/json", "X-CSRF-Token": CSRF } });
const user = { id: "a", username: "a", requiresPasswordChange: false, isAdmin: true, assignments: [] };

function harness() {
  const reads: { url: string; signal?: AbortSignal | null }[] = [];
  let held: ReturnType<typeof deferred<Response>> | undefined;
  const logout = deferred<Response>();
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/auth/me")) return ok(user);
    if (url.endsWith("/auth/mode")) return ok({ mode: "local" });
    if (url.endsWith("/auth/login")) return ok({ user });
    if (url.endsWith("/auth/logout")) return logout.promise;
    reads.push({ url, signal: init?.signal });
    if (held) {
      const response = held.promise;
      // Like the browser, an aborted request rejects.
      return new Promise<Response>((resolve, reject) => {
        init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        void response.then(resolve);
      });
    }
    return ok({ items: [], pagination: { totalItems: 0, currentPage: 1, itemsPerPage: 100, totalPages: 1 } });
  }) as typeof fetch;
  const adapter = createHttpAdapter({ fetch: fetcher, eventSource: () => ({ close() {}, onopen: null, onmessage: null, onerror: null }) as unknown as EventSource });
  return {
    adapter, reads,
    hold: () => { held = deferred<Response>(); return held; },
    release: () => { held = undefined; },
    logout: () => logout,
  };
}

it("cancels running reads when a logout starts and starts none until the next sign-in", async () => {
  const h = harness();
  await h.adapter.getCurrentUser();
  h.hold();
  const running = h.adapter.listHosts().catch(() => "failed");
  await flush();
  const inFlight = h.reads.length;
  expect(inFlight > 0).toBe(true);
  const pending = h.adapter.logout();
  await flush();
  expect(h.reads.slice(0, inFlight).every((r) => r.signal?.aborted === true)).toBe(true);
  h.release();
  // A read queued while the logout runs, and one after it, never reach the network.
  const during = h.adapter.listHosts().catch(() => "failed");
  await flush();
  h.logout().resolve(ok({}));
  await pending;
  const after = h.adapter.listHosts().catch(() => "failed");
  await Promise.all([running, during, after]);
  expect(h.reads.length).toBe(inFlight);
  await h.adapter.login("a", "p".repeat(12));
  await h.adapter.listHosts().catch(() => "failed");
  expect(h.reads.length > inFlight).toBe(true);
});

it("reopens reads when the logout fails", async () => {
  const h = harness();
  await h.adapter.getCurrentUser();
  const pending = h.adapter.logout();
  await flush();
  h.logout().resolve(new Response(JSON.stringify({ code: "http.error" }), { status: 500 }));
  await expect(pending).rejects.toBeTruthy();
  await h.adapter.listHosts().catch(() => "failed");
  expect(h.reads.length > 0).toBe(true);
});
