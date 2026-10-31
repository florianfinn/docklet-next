// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original production Hosts settings gestures through the narrow connected read. M14 host maintenance and availability.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { ConnectedHostDefaultsSection } from "@/features/settings/sections/SimpleSections";
import { renderWithApp } from "@/test/render";
const defaults = { projectDir: "/srv/projects", installDir: "/srv/agent", connection: "edge" as const, puid: "", pgid: "", detectSocketGroup: true };
afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); });
it.each([["en-US", 1440], ["en-US", 390], ["de", 1440], ["de", 390]] as const)("H4 loads and saves host defaults via real form gestures in %s at %s", async (locale, width) => {
  setAppLocale(locale); window.innerWidth = width;
  const demo = createDemoAdapter();
  const connected = vi.fn(async (section?: string) => section ? { hostDefaults: defaults } : demo.getConnectedSettings());
  const save = vi.fn(async () => {});
  const full = vi.fn(async () => { throw new Error("full-settings-must-not-be-read"); });
  const adapter = { ...demo, kind: "http", getConnectedSettings: connected, getSettings: full, saveSettings: save } as unknown as DockletAdapter;
  renderWithApp(<ConnectedHostDefaultsSection />, adapter);
  const input = await screen.findByLabelText(locale === "de" ? "Standard-Projektverzeichnis" : "Default project directory");
  expect(connected).toHaveBeenCalledWith("hostDefaults"); expect(full).not.toHaveBeenCalled();
  await userEvent.clear(input); await userEvent.type(input, "/srv/workloads");
  await userEvent.click(screen.getByRole("button", { name: locale === "de" ? "Speichern" : "Save" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("hostDefaults", { ...defaults, projectDir: "/srv/workloads" }));
});
