// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original PF1 test of owner-bound preference writes through the real HTTP adapter, issue #339.

import { afterEach, expect, it, vi } from "vitest";

import { createPreferenceSync, OWNER_KEY } from "@/shared/preferences/sync/engine";
import type { SectionBinding } from "@/shared/preferences/sync/sections";
import { createStore } from "@/shared/lib/store";

import { createHttpAdapter } from "./http-adapter";

afterEach(() => { vi.useRealTimers(); localStorage.clear(); });

function deferred<T>() {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>((r) => { resolve = r; }), resolve };
}
const flush = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };
// CSRF header values of the first and the renewed session.
const FIRST = "a".repeat(43);
const RENEWED = "b".repeat(43);
const ok = (data: unknown, csrf = FIRST) => new Response(JSON.stringify({ success: true, data }), { headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf } });
const user = (id: string) => ({ id, username: id, requiresPasswordChange: false, isAdmin: false, assignments: [] });

it("never dispatches a preference write retried after a CSRF renewal once another user signed in", async () => {
  vi.useFakeTimers();
  localStorage.setItem(OWNER_KEY, "a");
  const renewal = deferred<Response>();
  const nextUser = deferred<Response>();
  const writes: { session: string; value: unknown }[] = [];
  let session = "a";
  let me = 0;
  let renewing = false;
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/auth/me")) return ++me === 1 ? ok(user("a")) : nextUser.promise;
    if (url.endsWith("/auth/mode")) { renewing = true; return renewal.promise; }
    if (init?.method === "PUT") {
      writes.push({ session, value: JSON.parse(String(init.body)).value });
      if (writes.length === 1) return new Response(JSON.stringify({ code: "csrf.invalid" }), { status: 403 });
    }
    return ok({ revision: 0, sections: {} });
  }) as typeof fetch;
  const adapter = createHttpAdapter({ fetch: fetcher, eventSource: () => ({ close() {}, onopen: null, onmessage: null, onerror: null }) as unknown as EventSource });
  await adapter.getCurrentUser();
  const store = createStore<object>({ mode: "default" });
  const binding: SectionBinding<object> = { section: "help", store, legacyKey: "example.help", toWire: (s) => s, fromWire: (raw, current) => (raw as object) ?? current };
  const sync = createPreferenceSync({ api: adapter, bindings: [binding as SectionBinding], storage: localStorage });
  try {
    sync.setIdentity("a");
    await flush();
    // This identity read is already in flight when the write's CSRF renewal starts.
    const refresh = adapter.getCurrentUser();
    await flush();
    store.set({ mode: "private-a" });
    await vi.advanceTimersByTimeAsync(1000);
    expect(renewing).toBe(true);
    session = "b";
    nextUser.resolve(ok(user("b"), RENEWED));
    await refresh;
    sync.setIdentity("b");
    renewal.resolve(ok({ mode: "local" }, RENEWED));
    await flush();
    expect(writes).toEqual([{ session: "a", value: { mode: "private-a" } }]);
  } finally {
    sync.dispose();
  }
});
