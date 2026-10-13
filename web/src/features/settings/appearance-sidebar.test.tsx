import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { AppShell } from "@/app/shell/AppShell";
import { simulationStore } from "@/data/demo/simulation";
import { labStore, tokenPreviewStore } from "@/features/design-lab/design-tokens";
import { captureSnapshot, decodeDesignCode, encodeDesignCode } from "@/features/design-lab/favorite-code";
import { DEFAULT_VISUAL_STYLE, setVisualStyle, sidebarWidthCss, sidebarWidthPreviewStore, visualStyleStore } from "@/shared/preferences/visual-style-store";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

const controls = import.meta.glob<string>("/src/styles/controls.css", { query: "?raw", import: "default", eager: true })["/src/styles/controls.css"] ?? "";
const routes = [{ element: <AppShell />, children: [{ path: "/settings/:section", element: <SettingsPage /> }] }];
const shellWidth = () => document.querySelector<HTMLElement>('[data-slot="sidebar-wrapper"]')?.style.getPropertyValue("--sidebar-width");

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  visualStyleStore.reset();
  sidebarWidthPreviewStore.set(null);
  labStore.reset();
  tokenPreviewStore.set(null);
  simulationStore.reset();
  localStorage.clear();
});

describe("sidebar width", () => {
  it("is one clamped UI preference with the shipped 17.5 rem default that never takes more than 40 % of the window", () => {
    expect([DEFAULT_VISUAL_STYLE.sidebarWidth, sidebarWidthCss(17), sidebarWidthCss(40), sidebarWidthCss(3), sidebarWidthCss(Number.NaN)]).toEqual([
      17.5,
      "min(17rem, max(14rem, 40vw))",
      "min(22rem, max(14rem, 40vw))",
      "min(14rem, max(14rem, 40vw))",
      "min(17.5rem, max(14rem, 40vw))",
    ]);
    setVisualStyle({ sidebarWidth: 99 });
    expect(visualStyleStore.get().sidebarWidth).toBe(22);
  });

  it("travels in the design code and the factory favourite", () => {
    setVisualStyle({ sidebarWidth: 19.5 });
    const decoded = decodeDesignCode(encodeDesignCode(captureSnapshot()));
    expect(decoded.ok && decoded.snapshot.visual?.sidebarWidth).toBe(19.5);
    const foreign = decodeDesignCode(encodeDesignCode({ visual: { ...DEFAULT_VISUAL_STYLE, sidebarWidth: 80 } }));
    expect(foreign.ok && foreign.snapshot.visual?.sidebarWidth).toBe(22);
  });

  it("previews in the shell from hub settings, saves on request and discards otherwise", async () => {
    renderRoutes(routes, ["/settings/appearance"]);
    await screen.findByText("Breite der Seitenleiste");
    const slider = within(document.getElementById("ap-sidebar-width")!).getByRole("slider", { name: "Breite der Seitenleiste" });
    expect([shellWidth(), slider.getAttribute("aria-valuetext")]).toEqual([sidebarWidthCss(17.5), "17.5 rem"]);

    fireEvent.keyDown(slider, { key: "ArrowRight" });
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    // Live preview in the shell, nothing stored yet.
    await waitFor(() => expect(shellWidth()).toBe(sidebarWidthCss(18)));
    expect([visualStyleStore.get().sidebarWidth, screen.getByRole("status").textContent]).toEqual([17.5, "Ungespeicherte Änderungen"]);

    await userEvent.click(screen.getByRole("button", { name: "Verwerfen" }));
    await waitFor(() => expect(shellWidth()).toBe(sidebarWidthCss(17.5)));

    fireEvent.keyDown(slider, { key: "End" });
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(visualStyleStore.get().sidebarWidth).toBe(22));
    expect([shellWidth(), sidebarWidthPreviewStore.get(), JSON.parse(localStorage.getItem("docklet-react-opus.visual-style") ?? "{}").data.sidebarWidth]).toEqual([sidebarWidthCss(22), null, 22]);

    // The design lab writes the same setting; the clean draft follows it.
    act(() => setVisualStyle({ sidebarWidth: 15 }));
    await waitFor(() => expect(slider.getAttribute("aria-valuetext")).toBe("15 rem"));
    expect(shellWidth()).toBe(sidebarWidthCss(15));
  });
});

describe("navigation rows", () => {
  it("share one row style in the sidebar and the settings navigation, with one marked page each", async () => {
    renderRoutes(routes, ["/settings/notifications"]);
    const settingsNav = await screen.findByRole("navigation", { name: "Bereiche der Hub-Einstellungen" });
    const rows = [...settingsNav.querySelectorAll("a")];
    expect(rows.length > 0 && rows.every((a) => a.classList.contains("nav-row"))).toBe(true);
    expect(rows.filter((a) => a.getAttribute("aria-current") === "page").map((a) => a.textContent)).toEqual(["Benachrichtigungen"]);
    // No washed-out text colour on the active settings row any more.
    expect(rows.some((a) => /text-primary-text|bg-primary\/12/.test(a.className))).toBe(false);

    const sidebarRows = [...document.querySelectorAll<HTMLElement>('[data-slot="sidebar"] [data-slot="sidebar-menu-button"].nav-row')];
    const active = sidebarRows.filter((el) => el.dataset.active === "true");
    expect([sidebarRows.length >= 10, active.map((el) => el.textContent)]).toEqual([true, ["Benachrichtigungen"]]);
    // controls.css overrides the shadcn sidebar-accent fills (unlayered CSS beats Tailwind utilities).
    expect(/\.dark \{[^}]*--nav-active: color-mix\(in oklch, var\(--primary\) 26%/.test(controls)).toBe(true);
    expect(/\.nav-row:is\(\[data-active="true"\], \[aria-current="page"\]\) \{[^}]*background-color: var\(--nav-active\)[^}]*color: var\(--foreground\)/.test(controls)).toBe(true);
    expect(/\.nav-row:is\(\[data-active="true"\], \[aria-current="page"\]\) > svg \{\s*color: var\(--control-text\)/.test(controls)).toBe(true);
  });
});
