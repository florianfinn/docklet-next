// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original UI lifecycle and role boundary proofs. M19 web consumer integration.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AuthGate } from "@/app/AuthGate";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { renderRoutes, renderWithApp } from "@/test/render";
import { GlobalSearch } from "./GlobalSearch";
import { AppShell } from "./AppShell";
import { matchSearch } from "./search-model";
beforeEach(() => { setAppLocale("en-US"); simulationStore.set((state) => ({ ...state, latency: "instant", live: false })); });
afterEach(() => { setAppLocale("de"); simulationStore.reset(); });
it("matching searches all candidates before limiting each group to eight", () => {
  const entries = Array.from({ length: 20 }, (_, i) => ({ id: String(i), label: `Stack ${i}`, host: i > 9 ? "Needle host" : "Other" }));
  expect(matchSearch(entries, "Needle").map((row) => row.id)).toEqual(["10", "11", "12", "13", "14", "15", "16", "17"]);
});
it("keyboard opens with Ctrl/Meta K, Escape closes and editor keeps its shortcut", async () => {
  renderWithApp(<><GlobalSearch /><div data-slot="code-editor"><textarea aria-label="Code" /></div></>);
  const code = screen.getByLabelText("Code"); code.focus(); await userEvent.keyboard("{Control>}k{/Control}"); expect(screen.queryByRole("dialog") === null).toBe(true);
  code.blur(); document.body.focus(); await userEvent.click(screen.getByRole("button", { name: "Search" })); await screen.findByRole("dialog"); await userEvent.keyboard("{Escape}");
  await userEvent.keyboard("{Control>}k{/Control}"); expect(screen.queryByRole("dialog") !== null).toBe(true); await userEvent.keyboard("{Escape}");
  await userEvent.keyboard("{Meta>}k{/Meta}"); expect(screen.queryByRole("dialog") !== null).toBe(true); const dialog = screen.getByRole("dialog");
  const input = within(dialog).getByRole("combobox"); await userEvent.type(input, "Profile"); await userEvent.keyboard("{ArrowDown}{Enter}"); await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
});
it("identity change closes current results", async () => {
  const adapter = createDemoAdapter(); let end = () => {}; let version = 0;
  registerIdentityLifetime(adapter.subscribeChanges, { active: () => true, version: () => version, listen: (fn) => { end = fn; return () => {}; } });
  renderWithApp(<GlobalSearch />, adapter); await userEvent.click(screen.getByRole("button", { name: "Search" })); await screen.findByRole("dialog");
  await act(() => { version++; end(); }); expect(screen.queryByRole("dialog") === null).toBe(true);
});
it("user destinations all resolve in UserShell and no admin-only query is sent", async () => {
  const demo = createDemoAdapter(), stack = (await demo.listStacks())[0]!;
  const adapter = { ...demo, kind: "http" as const, getCurrentUser: vi.fn(async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false })), listStacks: vi.fn(async () => [stack]), getAuthMode: async () => "local" as const };
  const forbidden = [vi.spyOn(adapter, "listContainers"), vi.spyOn(adapter, "listImages"), vi.spyOn(adapter, "listVolumes"), vi.spyOn(adapter, "getTopology"), vi.spyOn(adapter, "getConnectedSettings")];
  const view = renderRoutes([{ element: <AuthGate />, children: [{ element: <AppShell />, children: ["/dashboard", "/stacks", "/profile", "/stacks/:id"].map((path) => ({ path, element: <h1>Mounted destination</h1> })) }] }], ["/dashboard"], adapter);
  await screen.findByRole("heading", { name: "Mounted destination" });
  for (const [label, path] of [["Dashboard", "/dashboard"], ["Stacks", "/stacks"], ["Profile", "/profile"], ["Show all in Stacks", "/stacks"], [stack.name, `/stacks/${encodeURIComponent(stack.id)}`]]) {
    await userEvent.click(screen.getByRole("button", { name: "Search" })); const dialog = await screen.findByRole("dialog");
    const options = await within(dialog).findAllByRole("option");
    expect(options.every((option) => ["Dashboard", "Stacks", "Profile", "Show all in Stacks", "Toggle theme", "Sign out"].includes(option.textContent ?? "") || option.textContent?.startsWith(stack.name))).toBe(true);
    expect(options.map((option) => option.textContent).some((text) => text?.includes("New stack") || text === "Hosts")).toBe(false);
    const option = options.find((entry) => entry.textContent === label || entry.textContent?.startsWith(label + "Atlas")); expect(!!option).toBe(true);
    await userEvent.click(option!); await waitFor(() => expect(view.router.state.location.pathname).toBe(path));
    expect(screen.queryByRole("heading", { name: "Not available" }) === null).toBe(true);
  }
  await userEvent.click(screen.getByRole("button", { name: "Search" })); const dialog = await screen.findByRole("dialog");
  await userEvent.click(within(dialog).getByRole("option", { name: "Toggle theme" }));
  await userEvent.click(screen.getByRole("button", { name: "Search" }));
  await userEvent.click(within(await screen.findByRole("dialog")).getByRole("option", { name: "Sign out" })); await screen.findByRole("alertdialog");
  forbidden.forEach((spy) => expect(spy).toHaveBeenCalledTimes(0));
});

it("simple-user search sends zero administrator inventory requests", async () => {
  const demo = createDemoAdapter();
  const adapter = { ...demo, kind: "http" as const, getCurrentUser: async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false }) };
  const forbidden = [vi.spyOn(adapter, "listContainers"), vi.spyOn(adapter, "listImages"), vi.spyOn(adapter, "listVolumes"), vi.spyOn(adapter, "getTopology")];
  renderRoutes([{ element: <AuthGate />, children: [{ path: "/", element: <GlobalSearch /> }] }], ["/"], adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Search" }));
  await screen.findByRole("option", { name: "Profile" });
  forbidden.forEach((spy) => expect(spy).toHaveBeenCalledTimes(0));
});
