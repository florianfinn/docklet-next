import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { ControlsSection } from "@/features/design-lab/ControlsSection";
import ContainersPage from "@/features/containers/ContainersPage";
import StacksPage from "@/features/stacks/overview/StacksPage";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { uiStyleStore, useApplyUiStyle } from "@/shared/preferences/ui-style-store";
import { renderWithApp } from "@/test/render";

import { LoadView } from "./container-load";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  preferencesStore.reset();
  uiStyleStore.reset();
  simulationStore.reset();
  localStorage.clear();
  delete document.documentElement.dataset.toggleStyle;
});

const checked = (el: HTMLElement) => el.getAttribute("aria-checked") === "true";

describe("Compact/Advanced view controls", () => {
  it("marks the active mode and shows selection only in Advanced", async () => {
    renderWithApp(<StacksPage />);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    const group = screen.getByRole("radiogroup", { name: "Darstellung" });
    const compact = within(group).getByRole("radio", { name: "Compact" });
    const advanced = within(group).getByRole("radio", { name: "Advanced" });
    const controls = group.closest('[data-slot="view-controls"]') as HTMLElement;
    expect(checked(compact) && !checked(advanced)).toBe(true);
    // Compact has no selection at all and no reserved selection button: layout and mode toggles only.
    expect(controls.querySelectorAll("button").length).toBe(4);
    expect(within(controls).getByRole("radiogroup", { name: "Anordnung" }) !== null).toBe(true);
    expect(screen.queryByRole("button", { name: /^Auswahl/ }) === null).toBe(true);
    expect(screen.queryAllByRole("checkbox").length).toBe(0);

    await userEvent.click(advanced);
    expect(checked(advanced) && !checked(compact)).toBe(true);
    expect(group.style.getPropertyValue("--seg-index")).toBe("1");
    expect(within(group).getByRole("radio", { name: "Advanced" }) === advanced).toBe(true);
    await userEvent.click(screen.getByRole("checkbox", { name: "Media auswählen" }));
    expect(screen.getByRole("toolbar", { name: "1 Stack ausgewählt" }) instanceof HTMLElement).toBe(true);

    // Back to Compact drops the selection instead of hiding it.
    await userEvent.click(compact);
    expect(screen.queryAllByRole("checkbox").length).toBe(0);
    expect(screen.queryByRole("toolbar") === null).toBe(true);
    await userEvent.click(advanced);
    expect(checked(screen.getByRole("checkbox", { name: "Media auswählen" }))).toBe(false);
    expect(screen.queryByRole("toolbar") === null).toBe(true);
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.preferences") ?? "{}").data.stacksSelecting === undefined).toBe(true);
  });

  it("clears a container selection when leaving Advanced and keeps rows clickable", async () => {
    preferencesStore.set((p) => ({ ...p, containersMode: "advanced" }));
    const { router } = renderWithApp(<ContainersPage />);
    const box = await screen.findByRole("checkbox", { name: "duplicati auswählen" });
    await userEvent.click(box);
    expect(router.state.location.search).toBe("");
    expect(screen.getByRole("toolbar", { name: "1 Container ausgewählt" }) instanceof HTMLElement).toBe(true);
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Darstellung" })).getByRole("radio", { name: "Compact" }));
    expect(screen.queryByRole("toolbar") === null && screen.queryAllByRole("checkbox").length === 0).toBe(true);
    expect(preferencesStore.get().containersMode).toBe("compact");
  });
});

describe("container load display", () => {
  it("shows labelled CPU/RAM rings with thresholds and an unavailable state", () => {
    renderWithApp(
      <>
        <LoadView values={{ cpu: 12.4, memMb: 922 }} memLimitMb={1024} display="rings" />
        <LoadView values={null} memLimitMb={1024} display="rings" />
      </>,
    );
    const cpu = screen.getByRole("meter", { name: "CPU" });
    const ram = screen.getByRole("meter", { name: "RAM" });
    expect(cpu.getAttribute("aria-valuenow")).toBe("12");
    expect(ram.getAttribute("aria-valuetext")).toBe("922 MB von 1 GB (90 % des Limits, simuliert)");
    // 90 % of the limit is critical: the arc uses the destructive colour.
    expect(ram.querySelector(".ring-arc")?.getAttribute("stroke")).toBe("var(--destructive)");
    expect(screen.getAllByRole("img", { name: /nicht verfügbar/ }).length).toBe(2);
  });

  it("follows the design-lab choice between rings and bars", async () => {
    uiStyleStore.set((s) => ({ ...s, metricDisplay: "rings" }));
    renderWithApp(<LoadView values={{ cpu: 5, memMb: 100 }} memLimitMb={512} />);
    expect(screen.getAllByRole("meter").length).toBe(2);
    act(() => uiStyleStore.set((s) => ({ ...s, metricDisplay: "bars" })));
    await waitFor(() => expect(screen.queryAllByRole("meter").length).toBe(0));
    expect(screen.getByText(/100 MB/) !== null).toBe(true);
  });
});

function ApplyStyle() {
  useApplyUiStyle();
  return null;
}

describe("design lab control styles", () => {
  it("applies the chosen toggle style live and persists it", async () => {
    uiStyleStore.set((s) => ({ ...s, toggleStyle: "pill", metricDisplay: "rings" }));
    renderWithApp(
      <>
        <ApplyStyle />
        <ControlsSection />
      </>,
    );
    expect(document.documentElement.dataset.toggleStyle).toBe("pill");
    await userEvent.click(screen.getByRole("radio", { name: /Leuchtschalter/ }));
    expect(document.documentElement.dataset.toggleStyle).toBe("glow");
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.ui-style") ?? "{}").data.toggleStyle).toBe("glow");
    await userEvent.click(screen.getByRole("radio", { name: /Balken mit Text/ }));
    expect(uiStyleStore.get().metricDisplay).toBe("bars");
  });
});
