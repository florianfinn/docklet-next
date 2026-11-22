// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #561 tests: on phones every user action is reachable from the card menu.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";
import { UsersSection } from "./UsersSection";

beforeEach(() => { setAppLocale("en-US"); simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { setAppLocale("de"); simulationStore.reset(); });

it("the card menu offers every user action and keeps the self and last-admin protections", async () => {
  const adapter = createDemoAdapter();
  const admin = (await adapter.listUsers())[0]!;
  const other = { ...admin, id: "other", username: "other", isAdmin: false };
  vi.spyOn(adapter, "listUsers").mockResolvedValue([admin, other]);
  renderWithApp(<UsersSection mode="local" />, adapter);
  // The last enabled administrator: disable and delete stay locked in the menu as on the desktop row.
  await userEvent.click(await screen.findByRole("button", { name: "More actions: admin" }));
  let menu = await screen.findByRole("menu");
  expect(["Edit", "Assign stacks", "Disable", "Reset password", "Delete"].map((name) => within(menu).queryByRole("menuitem", { name: new RegExp(`^${name}`) }) !== null)).toEqual([true, true, true, true, true]);
  expect(within(menu).getByRole("menuitem", { name: /^Disable/ }).getAttribute("aria-disabled")).toBe("true");
  expect(within(menu).getByRole("menuitem", { name: /^Delete/ }).getAttribute("aria-disabled")).toBe("true");
  await userEvent.keyboard("{Escape}");
  // Another user: delete opens the same confirmation as the row button.
  await userEvent.click(screen.getByRole("button", { name: "More actions: other" }));
  menu = await screen.findByRole("menu");
  await userEvent.click(within(menu).getByRole("menuitem", { name: /^Delete/ }));
  expect((await screen.findAllByRole("alertdialog")).length > 0 || screen.queryAllByRole("dialog").length > 0).toBe(true);
});
