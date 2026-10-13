import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

describe("label-only edits in the workspace", () => {
  it("enable Save and the tab dot, and clear both after saving", async () => {
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const modes = screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" });
    await userEvent.click(within(modes).getByRole("radio", { name: "Easy" }));
    await userEvent.click(within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getByRole("button", { name: /Stack/ }));

    const save = () => screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement;
    expect(save().disabled).toBe(true);
    await userEvent.type(await screen.findByPlaceholderText("z. B. produktiv"), "NeuesLabel");
    await userEvent.click(screen.getByRole("button", { name: "Label hinzufügen" }));
    expect(save().disabled).toBe(false);
    const dot = () => screen.getAllByRole("tab").some((t) => /Editor/.test(t.textContent ?? "") && t.querySelector("i.rounded-full") !== null);
    expect(dot()).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true));
    expect(dot()).toBe(false);
  });
});
