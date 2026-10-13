import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import ContainersPage from "@/features/containers/ContainersPage";
import { availableOrigins } from "@/features/containers/container-filters";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import { pageWidth } from "@/app/shell/page-width";
import { hiddenStore, mergeHidden, splitHidden } from "@/shared/preferences/hidden-store";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import StacksPage from "./StacksPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  hiddenStore.reset();
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const rowOf = (name: string) => screen.getByRole("link", { name: `Stack ${name} öffnen` }).closest("tr") as HTMLElement;

describe("context menu and hide from overview", () => {
  it("opens the ellipsis actions on right click, hides a stack into a collapsed category and restores it", async () => {
    renderRoutes([{ path: "/stacks", element: <StacksPage /> }], ["/stacks"]);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    await userEvent.click(within(rowOf("Media")).getByRole("button", { name: "Weitere Aktionen: Media" }));
    const dropdown = within(await screen.findByRole("menu")).getAllByRole("menuitem").map((m) => m.textContent);
    await userEvent.keyboard("{Escape}");
    fireEvent.contextMenu(rowOf("Media"));
    const menu = await screen.findByRole("menu", { name: "Media" });
    expect(within(menu).getAllByRole("menuitem").map((m) => m.textContent)).toEqual(dropdown);

    await userEvent.click(within(menu).getByRole("menuitem", { name: "Aus Übersicht ausblenden" }));
    await waitFor(() => expect(screen.queryByRole("link", { name: "Stack Media öffnen" })).toBe(null));
    // Presentation only: persisted, counted in the footer, nothing stopped; the category is collapsed.
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.hidden") ?? "{}").data.stacks).toEqual(["media"]);
    expect(screen.getByText(/^8 von 9 Stacks · 1 ausgeblendet/) !== null).toBe(true);
    const trigger = screen.getByRole("button", { name: /^Ausgeblendet/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    await userEvent.click(trigger);
    await userEvent.click(screen.getByRole("button", { name: "Media wieder einblenden" }));
    expect(await screen.findByRole("link", { name: "Stack Media öffnen" })).not.toBe(null);
    expect(hiddenStore.get().stacks).toEqual([]);
  });

  it("keeps a host block whose stacks are all hidden, with its own restorable category", async () => {
    hiddenStore.set({ stacks: ["dns", "git"], containers: [] });
    preferencesStore.set((p) => ({ ...p, stacksLayout: "hosts" }));
    renderRoutes([{ path: "/stacks", element: <StacksPage /> }], ["/stacks"]);
    const delta = await screen.findByRole("region", { name: "Delta" }).catch(() => document.querySelector<HTMLElement>('[data-host-block="delta"]')!);
    expect(within(delta).getByRole("button", { name: /^Ausgeblendet auf Delta/ }) !== null).toBe(true);
    expect(within(delta).queryByRole("link", { name: "Stack DNS öffnen" })).toBe(null);
    // A search for a hidden stack keeps its block reachable instead of claiming there is nothing.
    await userEvent.type(screen.getByRole("searchbox"), "dns");
    expect(document.querySelector('[data-host-block="delta"] [data-slot="hidden-section"]') !== null).toBe(true);
  });

  it("sanitises stored hidden ids and splits rows without reordering them", () => {
    expect(mergeHidden({ stacks: ["a", "a", 3, ""], containers: "x", legacy: true })).toEqual({ stacks: ["a"], containers: [] });
    const rows = [{ id: "a" }, { id: "b" }, { id: "c" }];
    expect(splitHidden(rows, ["b"])).toEqual({ shown: [{ id: "a" }, { id: "c" }], hidden: [{ id: "b" }] });
  });
});

describe("origin filters", () => {
  it("offer only origins present on the selected hosts", async () => {
    expect(availableOrigins(STANDALONE.filter((c) => c.hostId === "boreal"))).toEqual(["standalone"]);
    preferencesStore.set((p) => ({ ...p, hostSelection: ["delta"] }));
    renderRoutes([{ path: "/containers", element: <ContainersPage /> }], ["/containers?origin=unraid"]);
    const group = await screen.findByRole("radiogroup", { name: "Herkunft" });
    await waitFor(() => expect(within(group).getAllByRole("radio").map((r) => r.textContent)).toEqual(["Alle", "Compose"]));
    // The preselected origin from the link does not exist on Delta and falls back to all.
    expect(within(group).getByRole("radio", { name: "Alle" }).getAttribute("aria-checked")).toBe("true");
  });
});

describe("page bounds", () => {
  it("keeps the topology full width and bounds lists, details and forms", () => {
    expect(["/networks", "/stacks", "/images", "/stacks/home", "/volumes/v1", "/dashboard", "/settings/general"].map(pageWidth)).toEqual(["full", "lists", "lists", "detail", "detail", "dashboard", "narrow"]);
  });
});
