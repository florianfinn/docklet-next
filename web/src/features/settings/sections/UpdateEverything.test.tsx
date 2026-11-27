// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "use-intl";
import { MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import type { SelfUpdateCheck } from "@/domain/self-update";
import type { FleetStatus } from "@/domain/fleet-update";
import type { Host } from "@/domain/model";
import { UpdateEverything } from "./UpdateEverything";

const active: FleetStatus = { active: true, fleetId: "fleet", runId: "parent", phase: "agents", targetVersion: "1.0.1", hub: { environmentId: "0", opId: "hub", state: "skipped", outcome: "current" }, agents: [{ environmentId: "example", opId: "agent", runId: "child", state: "skipped", outcome: "skipped", code: "update.hub-failed" }] };
const cleanups: (() => void)[] = [];
afterEach(() => { cleanup(); cleanups.splice(0).forEach(f => f()); vi.useRealTimers(); });
function mount(hubState: "available" | "current" = "available", agentAvailable = true, status: FleetStatus = { active: false, agents: [] }, admin = true, currentAgents = 0) {
  let epoch = 0, enabled = true; const listeners = new Set<() => void>();
  const adapter = { ...createDisconnectedAdapter(), kind: "http" as const,
    getCurrentUser: vi.fn(async () => ({ id: "operator", isAdmin: admin }) as never),
    listHosts: vi.fn(async () => [{ id: "0", name: "Hub" }, { id: "example", name: "Example agent" }, ...Array.from({ length: currentAgents }, (_, i) => ({ id: `current-${i}`, name: `Current ${i}` }))] as Host[]),
    getCapability: vi.fn(async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: admin })),
    getSelfUpdate: vi.fn(async (host: string): Promise<SelfUpdateCheck> => ({ version: "1.0.0", eligibility: { state: host === "0" ? hubState : host.startsWith("current-") ? "current" : agentAvailable ? "available" : "current", candidate: { version: "1.0.1" }, manual: [] } })),
    getFleetUpdate: vi.fn(async () => status),
    updateEverything: vi.fn(async () => ({ fleetId: "fleet", runId: "parent", hubPhase: "current" as const })),
  };
  registerIdentityLifetime(adapter.subscribeChanges, { active: () => enabled, version: () => epoch, listen: f => { listeners.add(f); return () => listeners.delete(f); } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => client.clear());
  render(<QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><IntlProvider locale="de" messages={catalogs.de}><MemoryRouter><UpdateEverything /></MemoryRouter></IntlProvider></AdapterContext.Provider></QueryClientProvider>);
  return { adapter, client, logout: () => { enabled = false; epoch++; listeners.forEach(f => f()); } };
}
it.each([["available", false, true], ["current", true, true], ["current", false, false]] as const)("visibility: hub %s, agent available %s", async (hub, agent, visible) => {
  const f = mount(hub, agent); await waitFor(() => expect(f.adapter.getSelfUpdate).toHaveBeenCalledTimes(2));
  await act(async () => {});
  expect(screen.queryByRole("button", { name: "Alles aktualisieren" }) !== null).toBe(visible);
});
it("hides the control for non-administrators", async () => {
  const f = mount("available", true, undefined, false); await waitFor(() => expect(f.adapter.getCurrentUser).toHaveBeenCalled());
  expect(screen.queryByRole("button", { name: "Alles aktualisieren" }) !== null).toBe(false);
  expect(f.adapter.getSelfUpdate).not.toHaveBeenCalled(); expect(f.adapter.getFleetUpdate).not.toHaveBeenCalled();
});
it("confirms the shown target once with no-rollback and backup link", async () => {
  const f = mount(); const button = await screen.findByRole("button", { name: "Alles aktualisieren" });
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); fireEvent.click(button);
  const dialog = screen.getByRole("alertdialog");
  expect(dialog.textContent).toContain("1.0.1"); expect(dialog.textContent).toContain(catalogs.de.settings.updates.selfUpdate.noRollback);
  expect(within(dialog).getByRole("link").getAttribute("href")).toBe("/hub-backups");
  fireEvent.click(within(dialog).getByRole("button", { name: catalogs.de.settings.updates.selfUpdate.confirm }));
  await waitFor(() => expect(f.adapter.updateEverything).toHaveBeenCalledExactlyOnceWith("1.0.1"));
});
it("renders names, German reason, child link and settled result; dismissal hides it", async () => {
  mount("current", true, { ...active, active: false, phase: "settled", outcome: "partial" });
  await screen.findByText(catalogs.de.runMessages.fleet.partial);
  // The skipped state is named once; the reason line says why.
  expect(screen.getByText(/Example agent/).textContent?.split("Übersprungen").length).toBe(2);
  expect(screen.queryByText(catalogs.de.images.errors["update_hub-failed"]) !== null).toBe(true);
  expect(screen.queryByRole("button", { name: "Update-Aktivität für Example agent anzeigen" }) !== null).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Ausblenden" }));
  expect(screen.queryByText(catalogs.de.runMessages.fleet.partial) !== null).toBe(false);
});
it("polls active status at 3 seconds, stops at settlement and clears on logout", async () => {
  vi.useFakeTimers();
  const f = mount("current", true, active);
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  f.adapter.getFleetUpdate.mockResolvedValue({ ...active, active: false, phase: "settled", outcome: "success" });
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(f.adapter.getFleetUpdate).toHaveBeenCalledTimes(2);
  expect(screen.queryByText(catalogs.de.runMessages.fleet.success) !== null).toBe(true);
  await act(async () => { await vi.advanceTimersByTimeAsync(9000); });
  expect(f.adapter.getFleetUpdate).toHaveBeenCalledTimes(2);
  act(() => f.logout()); expect(screen.queryByText(/Example agent/) !== null).toBe(false);
  await act(async () => { await vi.advanceTimersByTimeAsync(6000); }); expect(f.adapter.getFleetUpdate).toHaveBeenCalledTimes(2);
});
it.each([["current", 1], ["available", 1]] as const)("counts only the agents that will follow (hub %s)", async (hub, count) => {
  mount(hub, true, undefined, true, 2);
  const button = await screen.findByRole("button", { name: "Alles aktualisieren" });
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); fireEvent.click(button);
  const text = screen.getByRole("alertdialog").textContent ?? "";
  expect(text.includes(`${count} Agenten`)).toBe(true);
  expect(text.includes("3 Agenten")).toBe(false);
});
