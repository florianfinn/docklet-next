// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #561 tests: the phone view menu switches layout and mode like the desktop toggles.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { renderWithApp } from "@/test/render";
import { OverviewViewControls } from "./overview-mode-toggle";

const labels = { label: "Modus", compact: "Compact", advanced: "Advanced", layout: "Layout", table: "Tabelle", hosts: "Hostblöcke" };

it("the view menu offers layout and mode and reports the choice", async () => {
  const onLayout = vi.fn(), onMode = vi.fn();
  renderWithApp(<OverviewViewControls layout="hosts" onLayout={onLayout} mode="compact" onMode={onMode} labels={labels} />);
  await userEvent.click(screen.getByRole("button", { name: catalogs.de.common.actions.view }));
  const menu = await screen.findByRole("menu");
  expect(within(menu).getByRole("menuitemradio", { name: "Hostblöcke" }).getAttribute("aria-checked")).toBe("true");
  await userEvent.click(within(menu).getByRole("menuitemradio", { name: "Tabelle" }));
  expect(onLayout).toHaveBeenCalledWith("table");
  await userEvent.click(screen.getByRole("button", { name: catalogs.de.common.actions.view }));
  await userEvent.click(within(await screen.findByRole("menu")).getByRole("menuitemradio", { name: "Advanced" }));
  expect(onMode).toHaveBeenCalledWith("advanced");
});
