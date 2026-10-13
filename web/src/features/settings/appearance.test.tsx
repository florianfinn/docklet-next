import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { DEFAULT_TOKENS, labStore, setTokens, tokenPreviewStore } from "@/features/design-lab/design-tokens";
import { useApplyDesignTokens } from "@/features/design-lab/use-design-lab";
import { PALETTE_PRESETS, paletteTokens, type Palette } from "@/shared/theme/palette";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

function ApplyTokens() {
  useApplyDesignTokens();
  return null;
}

const routes = [
  {
    path: "/settings/:section",
    element: (
      <>
        <ApplyTokens />
        <SettingsPage />
      </>
    ),
  },
];

const css = (name: string) => document.documentElement.style.getPropertyValue(name);
const preset = (id: string) => PALETTE_PRESETS.find((p) => p.id === id)!.palette;
/** Shipped default palette (favourite code): purple primary, cyan secondary. */
const SHIPPED = DEFAULT_TOKENS.palette;
const accent = (palette: Palette) => paletteTokens(palette, "dark").vars["--brand-accent"];
/** Control fill the palette derives for the active scheme: the primary colour, adjusted only for 3:1 contrast. */
const control = (palette: Palette) => paletteTokens(palette, document.documentElement.classList.contains("dark") ? "dark" : "light").vars["--control"];

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  labStore.reset();
  tokenPreviewStore.set(null);
  simulationStore.reset();
  localStorage.clear();
});

describe("appearance palette", () => {
  it("previews a preset on buttons and controls, discards it and saves only on request", async () => {
    renderRoutes(routes, ["/settings/appearance"]);
    const presets = await screen.findByRole("radiogroup", { name: "Vorlagen" });
    expect(css("--primary")).toBe(SHIPPED.primary);
    const motion = labStore.get().tokens.motion;

    await userEvent.click(within(presets).getByRole("radio", { name: /Mitternacht/ }));
    // Live preview: primary drives buttons and controls, secondary the accents; nothing is saved yet.
    expect(css("--primary") === "#4f39f6" && css("--control") === control(preset("midnight")) && css("--brand-accent") === "#00bbfd").toBe(true);
    expect(labStore.get().tokens.palette.primary).toBe(SHIPPED.primary);
    expect(screen.getByRole("status").textContent).toBe("Ungespeicherte Änderungen");

    await userEvent.click(screen.getByRole("button", { name: "Verwerfen" }));
    expect(css("--primary") === SHIPPED.primary && css("--control") === control(SHIPPED) && css("--brand-accent") === accent(SHIPPED)).toBe(true);
    expect(tokenPreviewStore.get()).toBe(null);

    // Exactly three role editors; the former controls and accent fields are gone.
    expect(screen.getAllByRole("textbox", { name: /als Hex-Wert$/ }).map((el) => el.getAttribute("aria-label"))).toEqual(["Primär als Hex-Wert", "Sekundär als Hex-Wert", "Hintergrund & Flächen als Hex-Wert"]);
    expect(screen.queryByText("Bedienelemente") === null && screen.queryByText("Sekundär & Akzent") === null).toBe(true);

    await userEvent.click(within(presets).getByRole("radio", { name: /Mitternacht/ }));
    const secondary = screen.getByRole("textbox", { name: "Sekundär als Hex-Wert" });
    await userEvent.clear(secondary);
    await userEvent.type(secondary, "#00b6d4");
    // Accents, chart series and the skeleton tint follow the secondary role; controls stay with primary.
    expect(css("--control") === control(preset("midnight")) && css("--brand-accent") === "#00b6d4" && css("--chart-1") === "#00b6d4" && css("--skeleton").startsWith("oklch(")).toBe(true);
    expect(screen.getByText(/^Eigene Palette/) !== null).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Alles gespeichert"));
    expect(labStore.get().tokens.palette).toEqual({ ...preset("midnight"), secondary: "#00b6d4" });
    expect(labStore.get().tokens.motion).toBe(motion);
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.design-lab") ?? "{}").data.tokens.palette).toEqual({ primary: "#4f39f6", secondary: "#00b6d4", surface: "#131721" });
    expect(tokenPreviewStore.get()).toBe(null);
  });

  it("lets a design-lab change replace the matching draft field instead of undoing it", async () => {
    renderRoutes(routes, ["/settings/appearance"]);
    const presets = await screen.findByRole("radiogroup", { name: "Vorlagen" });
    await userEvent.click(within(presets).getByRole("radio", { name: /Magenta/ }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Dichte" })).getByRole("radio", { name: "Großzügig" }));
    expect(css("--primary")).toBe("#a600b4");

    // The lab changes the palette directly; the settings draft adopts it and keeps its density edit.
    act(() => setTokens({ palette: preset("glacier") }));
    await waitFor(() => expect(css("--primary")).toBe("#007491"));
    expect(within(presets).getByRole("radio", { name: /Gletscher/ }).getAttribute("aria-checked")).toBe("true");
    expect(tokenPreviewStore.get()).toEqual({ density: "spacious" });

    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(labStore.get().tokens.density).toBe("spacious"));
    expect(labStore.get().tokens.palette).toEqual(preset("glacier"));
  });

  it("drops the preview when leaving the page without saving", async () => {
    const { router } = renderRoutes(routes, ["/settings/appearance"]);
    await userEvent.click(within(await screen.findByRole("radiogroup", { name: "Vorlagen" })).getByRole("radio", { name: /Gletscher/ }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Farbschema" })).getByRole("radio", { name: "Hell" }));
    expect(css("--primary") === "#007491" && !document.documentElement.classList.contains("dark")).toBe(true);
    await userEvent.click(screen.getByRole("link", { name: "Allgemein" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Verwerfen und wechseln" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/general"));
    expect(css("--primary") === SHIPPED.primary && document.documentElement.classList.contains("dark")).toBe(true);
    expect(labStore.get().tokens.theme).toBe("dark");
  });
});
