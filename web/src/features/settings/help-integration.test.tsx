import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { helpStore } from "@/shared/help/help-store";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  helpStore.reset();
  localStorage.clear();
});

const routes = [{ path: "/settings/:section", element: <SettingsPage /> }];

describe("explanations in hub settings", () => {
  it("explains fields beside their labels and hides only those explanations once switched off", async () => {
    const { router } = renderRoutes(routes, ["/settings/general"]);
    const address = await screen.findByLabelText("Interne Hub-Adresse");
    expect(address.id).toBe("gen-address");
    const info = screen.getByRole("button", { name: "Erklärung: Interne Hub-Adresse" });
    info.focus();
    expect(((await screen.findByRole("tooltip")).textContent ?? "").includes("Edge-Agents")).toBe(true);
    expect(screen.getAllByRole("button", { name: /^Erklärung: / }).length >= 5).toBe(true);

    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("link", { name: "Erklärungen" }));
    await userEvent.click(within(await screen.findByRole("radiogroup", { name: "Erklärungen" })).getByRole("radio", { name: "Aus" }));
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(helpStore.get().mode).toBe("off"));

    await router.navigate("/settings/general");
    await screen.findByLabelText("Interne Hub-Adresse");
    expect(screen.queryAllByRole("button", { name: /^Erklärung: / }).length).toBe(0);
  });
});
