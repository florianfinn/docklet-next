// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original real App provider/recovery boundary tests with a bounded dashboard route fixture.

import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createHttpAdapter } from "@/data/http/http-adapter";
import { HUB_UPDATE_RECORD } from "@/data/http/hub-update-session";
import type { DockletAdapter } from "@/data/adapter";
import { setAppLocale } from "@/app/i18n/locale-store";

const injection = vi.hoisted(() => ({ adapter: undefined as DockletAdapter | undefined }));
vi.mock("@/runtime", async original => ({ ...await original<object>(), createAdapter: () => injection.adapter }));
vi.mock("@/app/router", async () => {
  const { createMemoryRouter } = await import("react-router");
  const { AuthGate } = await import("@/app/AuthGate");
  return { router: createMemoryRouter([{ element: <AuthGate />, children: [{ path: "/dashboard", element: <p>Dashboard ready</p> }] }], { initialEntries: ["/dashboard"] }) };
});
import { App } from "@/app/App";

const cleanups: (() => void)[] = [];
afterEach(() => { cleanup(); cleanups.splice(0).forEach(f => f()); sessionStorage.clear(); setAppLocale("de"); });
function fixture(settled = false, failIdentity = false) {
  setAppLocale("en-US");
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify(data), { headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path.endsWith("/auth/mode")) return json({ success: true, data: { mode: "none", setupPending: false } });
    if (path.endsWith("/auth/me")) { if (failIdentity) throw new TypeError("offline"); return json({ success: true, data: { id: "admin", username: "operator", requiresPasswordChange: false, isAdmin: true } }); }
    if (path.endsWith("/upgrade/check")) return json({ version: "1.0.1", eligibility: { state: "checking", manual: null }, operation: { opId: "operation", runId: "update-run", targetVersion: "1.0.1", helperRunning: !settled, phase: settled ? "settled" : "helper-started", ...(settled ? { outcome: "healthy" } : {}) } });
    if (path.endsWith("/app-version")) return json({ currentVersion: "v1.0.1" });
    if (path.endsWith("/settings") || path.endsWith("/settings/public")) return json([{ key: "setupCompleted", value: "true" }, { key: "effectiveTimeZone", value: "UTC" }]);
    if (path.endsWith("/activities/update-run")) return json({ success: true, data: { activity: { id: "update-run", environmentId: "0", type: "hub.update", status: "running", startedAt: "2026-11-19T12:00:00Z" }, messages: [], stepTrace: [], output: [], outputTruncated: false } });
    return json({ success: true, data: [] });
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => ({ close() {}, onopen: null, onerror: null, onmessage: null }) });
  injection.adapter = adapter; cleanups.push(() => adapter.hubUpdateSession!.clear()); return { adapter, fetcher };
}
it("B7 real App on dashboard recovers an unsettled operation outside the router", async () => {
  const f = fixture(); render(<App />);
  expect(await screen.findByRole("dialog") !== null).toBe(true); expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("running");
});
it.each([false, true])("B7 real App replays settled operation only with this tab record: %s", async recorded => {
  const f = fixture(true);
  if (recorded) sessionStorage.setItem(HUB_UPDATE_RECORD, JSON.stringify({ hubOrigin: location.origin, principalId: "admin", opId: "operation", runId: "update-run", startedAt: Date.now() }));
  render(<App />);
  await waitFor(() => expect(f.fetcher.mock.calls.some(([path]) => String(path).includes("/upgrade/check"))).toBe(true)); await act(async () => {});
  if (recorded) { expect(await screen.findByRole("button", { name: "Open Docklet" }) !== null).toBe(true); expect(sessionStorage.getItem(HUB_UPDATE_RECORD)).toBeNull(); }
  else expect(screen.queryByRole("dialog") === null).toBe(true);
});
it("B6 real App keeps overlay above currentUser error gate", async () => {
  const f = fixture(false, true); f.adapter.hubUpdateSession!.establish("admin");
  await f.adapter.hubUpdateSession!.start("1.0.1", async () => ({ accepted: true, opId: "operation", runId: "update-run", phase: "reserved" }));
  render(<App />); await screen.findByRole("dialog");
  await waitFor(() => expect(document.querySelector('[data-slot="gate-screen"]')?.textContent).toContain("not reachable"));
  expect(document.querySelector('[data-slot="gate-screen"]')?.closest('[aria-hidden="true"]') !== null).toBe(true);
});
