// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original administrator-only hint and asynchronous anchor-focus interaction coverage.

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "use-intl";
import { MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { FreeUserMenu } from "@/app/shell/free/FreeUserMenu";
import SettingsPage from "@/features/settings/SettingsPage";
import { LiveUpdatesSelfUpdate } from "@/features/settings/sections/LiveUpdatesSelfUpdate";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import { createHttpAdapter } from "@/data/http/http-adapter";
import { AdapterContext } from "@/data/use-adapter";
import { useHubVersionHint } from "@/data/queries";
import type { DockletAdapter } from "@/data/adapter";
import { SidebarProvider } from "@/shared/ui/sidebar";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { renderRoutes } from "@/test/render";

const cleanups: (() => void)[] = [];
afterEach(() => { cleanup(); cleanups.splice(0).forEach(f => f()); vi.useRealTimers(); });
function hintFixture(admin: boolean | undefined = true, kind: DockletAdapter["kind"] = "http") {
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify(data), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path.endsWith("/auth/mode")) return json({ success: true, data: { mode: "none" } });
    if (path.endsWith("/auth/me")) return admin === undefined ? new Promise<Response>(() => {}) : json({ success: true, data: { id: "admin", username: "operator", requiresPasswordChange: false, isAdmin: admin } });
    if (path.endsWith("/app-version")) return json({ updateAvailable: true, newestVersion: "1.0.1" });
    return json({ success: true, data: [] });
  });
  const inner = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => ({ close() {}, onopen: null, onerror: null, onmessage: null }) });
  const adapter = { ...inner, kind, getHubVersionHint: vi.fn(inner.getHubVersionHint) };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => { client.clear(); inner.hubUpdateSession!.clear(); });
  return { adapter, fetcher, client };
}
function Probe() { return <p>{useHubVersionHint() ?? "no hint"}</p>; }
it.each([[false, "http"], [undefined, "http"], [true, "demo"], [true, "disconnected"]] as const)("B10 hint makes zero version requests for admin=%s kind=%s", async (admin, kind) => {
  const f = hintFixture(admin, kind);
  render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={f.client}><Probe /></QueryClientProvider></AdapterContext.Provider>);
  await act(async () => {}); expect(f.adapter.getHubVersionHint).not.toHaveBeenCalled(); expect(f.fetcher.mock.calls.filter(([path]) => String(path).endsWith("/app-version"))).toHaveLength(0);
  expect(screen.queryByText("no hint") !== null).toBe(true);
});
it("B10 hint is hidden while a hub update runs", async () => {
  const f = hintFixture();
  render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={f.client}><Probe /></QueryClientProvider></AdapterContext.Provider>);
  expect(await screen.findByText("1.0.1") !== null).toBe(true);
  act(() => { f.adapter.hubUpdateSession!.set({ state: { phase: "starting", target: "1.0.1", startedAt: Date.now() }, dismissed: false }); });
  expect([screen.queryByText("no hint") !== null, screen.queryByText("1.0.1") === null]).toEqual([true, true]);
});
it("B10 both badges name the version and Settings click reaches the hub anchor", async () => {
  const f = hintFixture();
  const routes = renderRoutes([{ path: "/settings/:section", element: <><SidebarProvider><FreeUserMenu onNavigate={() => {}} /></SidebarProvider><SettingsPage /></> }], ["/settings/help"], f.adapter);
  expect(await screen.findByText("Hub-Update 1.0.1 verfügbar") !== null).toBe(true);
  const updates = screen.getByRole("link", { name: /Updates.*1.0.1/ }); expect(updates.getAttribute("href")).toBe("/settings/updates#hub-update");
  fireEvent.keyDown(screen.getByRole("button", { name: "Profil und Hub-Einstellungen" }), { key: "Enter" });
  const row = await screen.findByRole("menuitem", { name: /Einstellungen.*1.0.1/ });
  fireEvent.click(row); await waitFor(() => expect(routes.router.state.location.hash).toBe("#hub-update"));
  expect(routes.router.state.location.pathname).toBe("/settings/updates"); routes.router.dispose();
});
it.each([true, false])("B10 async hub section focuses the enabled control or disabled heading: %s", async available => {
  let release!: (value: Awaited<ReturnType<DockletAdapter["getSelfUpdate"]>>) => void;
  const adapter = { ...createDisconnectedAdapter(), getCapability: async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: true }), getSelfUpdate: () => new Promise<Awaited<ReturnType<DockletAdapter["getSelfUpdate"]>>>(resolve => { release = resolve; }) };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => client.clear());
  render(<AdapterContext.Provider value={adapter}><QueryClientProvider client={client}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><TooltipProvider><MemoryRouter initialEntries={["/settings/updates#hub-update"]}><LiveUpdatesSelfUpdate hostId="0" /></MemoryRouter></TooltipProvider></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  await waitFor(() => expect(typeof release).toBe("function"));
  await act(async () => release({ version: "1.0.0", eligibility: { state: available ? "available" : "current", manual: null, ...(available ? { candidate: { version: "1.0.1" } } : {}) } }));
  await screen.findByText(/1.0.0/);
  expect(document.activeElement?.tagName).toBe(available ? "BUTTON" : "H4");
  expect(document.activeElement?.closest("#hub-update") !== null).toBe(true);
});

it("B10 refused hub capability focuses its heading after capability hydration", async () => {
  const adapter = { ...createDisconnectedAdapter(), getCapability: async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: false, disabledReasonCode: "access.admin-required" }) };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => client.clear());
  render(<AdapterContext.Provider value={adapter}><QueryClientProvider client={client}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter initialEntries={["/settings/updates#hub-update"]}><LiveUpdatesSelfUpdate hostId="0" /></MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  await screen.findByText(/administrator/i);
  expect(document.activeElement?.tagName).toBe("H4"); expect(document.activeElement?.closest("#hub-update") !== null).toBe(true);
});
