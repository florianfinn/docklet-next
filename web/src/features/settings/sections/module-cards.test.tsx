// Source: docklet-next/issues/522
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original module card tests.

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";

import { defaultModules, type ModuleChoices } from "@/domain/modules";
import { renderWithApp } from "@/test/render";

import { ModuleChoicesFields } from "./ModuleChoicesFields";

afterEach(cleanup);

function Cards() {
  const [value, setValue] = useState<ModuleChoices>(defaultModules());
  return <ModuleChoicesFields value={value} onChange={setValue} />;
}

it("toggles a module from anywhere on its card and marks the enabled card", async () => {
  renderWithApp(<Cards />);
  const toggle = await screen.findByRole("switch", { name: "Proxy" });
  const card = toggle.closest('[data-slot="module-card"]')!;
  expect([toggle.getAttribute("aria-checked"), card.classList.contains("border-primary")]).toEqual(["false", false]);
  expect(toggle.getAttribute("aria-describedby") === screen.getByText(/Reverse Proxy/).id).toBe(true);
  await userEvent.click(screen.getByText(/Reverse Proxy/));
  expect([toggle.getAttribute("aria-checked"), card.classList.contains("border-primary")]).toEqual(["true", true]);
  await userEvent.click(toggle);
  expect(toggle.getAttribute("aria-checked")).toBe("false");
  expect(card.parentElement!.className.includes("sm:grid-cols-2")).toBe(true);
});
