// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original reduced navigation, route denial and sign-out mode coverage.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { AuthGate } from "@/app/AuthGate";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { AuthMode } from "@/domain/settings";
import { renderRoutes, renderWithApp } from "@/test/render";
import { AppShell } from "./AppShell";
import StacksPage from "@/features/stacks/overview/StacksPage";
import { UserShell } from "./UserShell";
import { SidebarProvider } from "@/shared/ui/sidebar";
import { FreeUserMenu } from "./free/FreeUserMenu";
import { LogoutDialog } from "./LogoutDialog";

beforeEach(() => { setAppLocale("en-US"); simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { setAppLocale("de"); simulationStore.reset(); });

it("W3 renders only Stacks and profile; denied routes never mount their data consumers", async () => {
  const demo = createDemoAdapter();
  const adapter = { ...demo, kind: "http" as const, getCurrentUser: vi.fn(async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false })) };
  const forbidden = vi.fn();
  function AdminPage() { forbidden(); return <h1>Forbidden data</h1>; }
  const { router } = renderRoutes([{ element: <AuthGate />, children: [{ element: <AppShell />, children: [{ path: "/settings/general", element: <AdminPage /> }, { path: "/stacks", element: <h1>Assigned stacks</h1> }, { path: "/profile", element: <h1>Own profile</h1> }] }] }], ["/settings/general"], adapter);
  await screen.findByRole("heading", { name: "Not available" });
  expect(forbidden).not.toHaveBeenCalled();
  const links = [...document.querySelectorAll("a")].map((a) => a.getAttribute("href"));
  expect(links.every((href) => href === "/stacks")).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Profile and hub settings" }));
  expect(screen.queryByRole("menuitem", { name: "Hub settings" }) === null).toBe(true);
  expect(screen.queryByRole("menuitem", { name: "Profile" }) !== null).toBe(true);
  await userEvent.keyboard("{Escape}");
  await act(() => router.navigate("/stacks"));
  await screen.findByRole("heading", { name: "Assigned stacks" });
});

it.each(["none", "local", "proxy"] as const)("W5 offers sign-out only in local/proxy (%s)", async (mode) => {
  const demo = createDemoAdapter(); demo.getAuthMode = async () => mode;
  renderWithApp(<SidebarProvider><FreeUserMenu onNavigate={() => {}} /></SidebarProvider>, demo);
  await userEvent.click(await screen.findByRole("button", { name: "Profile and hub settings" }));
  await waitFor(() => expect(screen.queryByRole("menuitem", { name: "Sign out" }) !== null).toBe(mode !== "none"));
});

it.each([{ mode: "local", url: "", calls: 1 }, { mode: "proxy", url: "", calls: 0 }, { mode: "proxy", url: "https://proxy.example.com/logout", calls: 1 }] as const)("W5 executes $mode logout with URL '$url'", async ({ mode, url, calls }) => {
  const demo = createDemoAdapter();
  const logout = vi.fn(async () => {});
  const adapter = { ...demo, kind: "http" as const, getAuthMode: async () => mode as AuthMode, getAuthConfiguration: async () => ({ mode, ...(url ? { logoutUrl: url } : {}) }), logout };
  const assign = vi.spyOn(window.location, "assign").mockImplementation(() => {});
  renderWithApp(<LogoutDialog open onOpenChange={() => {}} />, adapter);
  const confirm = await screen.findByRole("button", { name: "Got it" });
  await waitFor(() => expect(confirm.hasAttribute("disabled")).toBe(false));
  if (mode === "proxy" && !url) expect(screen.queryByText("Sign out at your reverse proxy") !== null).toBe(true);
  await userEvent.click(confirm);
  await waitFor(() => expect(logout).toHaveBeenCalledTimes(calls));
  if (url) await waitFor(() => expect(assign).toHaveBeenCalledWith(url));
  else expect(assign).not.toHaveBeenCalled();
  assign.mockRestore();
});

it("none mode has no sign-out dialog action", () => {
  const adapter = createDemoAdapter(); adapter.getAuthMode = async () => "none";
  renderWithApp(<UserShell />, adapter);
  expect(screen.queryByRole("alertdialog") === null).toBe(true);
});


it("W1 actual user stack list never loads the host container inventory or administrator settings", async () => {
  const demo = createDemoAdapter();
  const name = (await demo.listStacks())[0]!.name;
  const adapter = { ...demo, kind: "http" as const, getCurrentUser: vi.fn(async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false })), listActions: async () => [], subscribeActions: () => () => {} };
  const containers = vi.spyOn(adapter, "listContainers"), settings = vi.spyOn(adapter, "getConnectedSettings");
  renderRoutes([{ element: <AuthGate />, children: [{ element: <AppShell />, children: [{ path: "/stacks", element: <StacksPage /> }] }] }], ["/stacks"], adapter);
  await screen.findByRole("heading", { name: "Stacks" });
  await screen.findByText(name);
  expect(containers).not.toHaveBeenCalled();
  expect(settings).not.toHaveBeenCalled();
});
