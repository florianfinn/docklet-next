// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original none/local/proxy browser route and logout matrix.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createHttpAdapter } from "@/data/http/http-adapter";
import type { AuthMode } from "@/domain/settings";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";
import { renderRoutes } from "@/test/render";

import { AuthGate } from "./AuthGate";
import { LogoutDialog } from "./shell/LogoutDialog";

afterEach(() => { cleanup(); localStorage.clear(); });

function server(mode: AuthMode, initialSession = true) {
  let session = initialSession;
  const calls: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    calls.push(path);
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/me") return session ? new Response(JSON.stringify({ success: true, data: { id: "admin", username: "operator", requiresPasswordChange: false } })) : new Response("{}", { status: 401 });
    if (path === "/api/auth/logout") {
      expect(init?.method).toBe("POST");
      session = false;
      return new Response(JSON.stringify({ success: true, data: { message: "signed out" } }));
    }
    if (path === "/api/environments/0/settings") return new Response(JSON.stringify(Object.entries({
      setupCompleted: "true", hubName: "Docklet", hubAddress: "192.0.2.10:8080", domainUrl: "", linkBase: "auto", effectiveTimeZone: "Europe/Berlin",
      keepActionOutput: "true", actionPresentation: "dialog",
    }).map(([key, value]) => ({ key, value, type: "text" }))));
    throw new Error(`unexpected ${path}`);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch });
  const routes = [
    { path: "/login", element: <ProductLoginPage /> },
    { element: <AuthGate />, children: [{ path: "/dashboard", element: <><p>Dashboard</p><LogoutDialog open onOpenChange={() => undefined} /></> }] },
  ];
  return { adapter, routes, calls };
}

describe.each(["none", "local", "proxy"] as const)("%s mode", (mode) => {
  it("routes a fresh load", async () => {
    const current = server(mode, mode !== "local");
    const { router } = renderRoutes(current.routes, ["/dashboard"], current.adapter);
    if (mode === "local") await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    else expect((await screen.findByText("Dashboard")) !== null).toBe(true);
  });

  it("handles a 401 after a successful load", async () => {
    const current = server(mode);
    const { router } = renderRoutes(current.routes, ["/dashboard"], current.adapter);
    await screen.findByText("Dashboard");
    window.dispatchEvent(new Event("docklet:unauthorized"));
    if (mode === "local") await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    else if (mode === "proxy") expect((await screen.findByRole("alert")).textContent).toMatch(/Proxy/);
    else expect((await screen.findByText("Die Sitzung konnte nicht geprüft werden.")) !== null).toBe(true);
  });

  it("ends only a local session", async () => {
    const current = server(mode);
    const { router } = renderRoutes(current.routes, ["/dashboard"], current.adapter);
    await screen.findByText("Dashboard");
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Verstanden" }));
    if (mode === "local") await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    else expect(router.state.location.pathname).toBe("/dashboard");
    expect(current.calls.filter((path) => path === "/api/auth/logout")).toHaveLength(mode === "local" ? 1 : 0);
  });
});
