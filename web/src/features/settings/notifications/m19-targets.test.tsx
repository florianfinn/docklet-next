// Source: docklet-next/issues/351
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original UI lifecycle and role boundary proofs.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { createNotificationSettings } from "@/data/http/notification-settings";
import invalid from "@/data/http/testdata/autoheal-v15.json";
import { renderRoutes } from "@/test/render";
import SettingsPage from "../SettingsPage";
beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());
it("same-channel targets keep edit, test, delete and pending state isolated by ID", async () => {
  const adapter = createDemoAdapter(), original = (await adapter.listNotificationTargets()).find((row) => row.kind === "target" && row.provider === "ntfy")!;
  if (original.kind !== "target") throw Error("fixture");
  const second = await adapter.saveNotificationTarget({ ...original, id: undefined, name: "Second ntfy", config: { ...original.config, topic: "second-topic" } });
  const test = vi.spyOn(adapter, "testNotificationTarget"), remove = vi.spyOn(adapter, "deleteNotificationTarget"), save = vi.spyOn(adapter, "saveNotificationTarget");
  renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/notifications"], adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Bearbeiten: Second ntfy" }));
  const dialog = await screen.findByRole("dialog"); await userEvent.clear(within(dialog).getByLabelText("Topic")); await userEvent.type(within(dialog).getByLabelText("Topic"), "changed-second");
  await userEvent.click(within(dialog).getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  expect(save.mock.calls[0]?.[0].id).toBe(second.id);
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
   expect((await adapter.listNotificationTargets()).find((row) => row.id === original.id)).toEqual(original);
  const row = screen.getByRole("button", { name: "Bearbeiten: Second ntfy" }).closest("li")!;
  await userEvent.click(within(row).getByRole("button", { name: "Test senden" })); await waitFor(() => expect(test).toHaveBeenCalledWith(second.id));
  await userEvent.click(screen.getByRole("button", { name: "Entfernen: Second ntfy" })); await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Entfernen" }));
  await waitFor(() => expect(remove).toHaveBeenCalledWith(second.id)); expect((await adapter.listNotificationTargets()).some((entry) => entry.id === original.id)).toBe(true);
});
it("add offers an existing channel and defaults a unique name", async () => {
  renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/notifications"]);
  await waitFor(() => expect((screen.getByRole("button", { name: "Ziel hinzufügen" }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(screen.getByRole("button", { name: "Ziel hinzufügen" })); await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "ntfy" }));
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("ntfy 2");
});
it("Direct and Edge producer shapes render an invalid cooldown hint instead of unavailable", async () => {
  const adapter = createDemoAdapter(), hosts = (await adapter.listHosts()).filter((host) => host.id === "atlas" || host.id === "delta").map((host, index) => ({ ...host, connection: index ? "edge" as const : "direct" as const, status: "online" as const }));
  adapter.listHosts = async () => hosts;
  const fetcher = vi.fn(async () => new Response(JSON.stringify(invalid)));
  const settings = createNotificationSettings({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "" }, generation: () => 0, capability: adapter.getCapability });
  adapter.getAutoHealSettings = settings.getAutoHealSettings;
  renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/autoHeal"], adapter);
  await waitFor(() => expect(fetcher.mock.calls.length).toBe(2));
  await waitFor(() => expect(screen.queryAllByText("Wird geladen …").length).toBe(0));
  expect(screen.queryAllByText(/Auto Heal ist auf diesem Host pausiert/).length).toBe(2);
  expect(fetcher.mock.calls.length).toBe(2); expect(screen.queryByText("Nicht verfügbar") === null).toBe(true);
  await act(async () => {});
});
