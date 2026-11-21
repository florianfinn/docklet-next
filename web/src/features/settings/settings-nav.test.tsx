// Source: docklet-next/issues/545
// Revision: 0c0cb0d3e3b439684308e9d13b6ded2cbe1efde8
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original narrow-screen settings navigation guard.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const routes = [{ path: "/settings/:section", element: <SettingsPage /> }];

describe("settings navigation on narrow screens", () => {
  it("offers one grouped section picker instead of wrapped link rows", async () => {
    const { router } = renderRoutes(routes, ["/settings/general"]);
    const picker = await screen.findByTestId("settings-section-picker");
    const list = screen.getByTestId("settings-section-list");
    // The picker is the narrow-screen control; the link list is the lg sidebar only.
    expect(picker.closest(".lg\\:hidden") !== null).toBe(true);
    expect(list.classList.contains("hidden") && list.classList.contains("lg:flex")).toBe(true);
    expect(picker.textContent?.includes("Allgemein")).toBe(true);

    picker.focus();
    await userEvent.keyboard("{Enter}");
    const listbox = await screen.findByRole("listbox");
    expect(["Hub", "Betrieb", "System"].every((group) => within(listbox).queryByText(group) !== null)).toBe(true);
    await userEvent.click(within(listbox).getByRole("option", { name: "Updates" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/updates"));
  });

  it("keeps the unsaved-changes guard when switching through the picker", async () => {
    const { router } = renderRoutes(routes, ["/settings/general"]);
    const name = (await screen.findByLabelText("Name des Hubs")) as HTMLInputElement;
    await userEvent.type(name, "-Entwurf");
    const picker = screen.getByTestId("settings-section-picker");
    picker.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.click(within(await screen.findByRole("listbox")).getByRole("option", { name: "Updates" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect(router.state.location.pathname).toBe("/settings/general");
  });
});
