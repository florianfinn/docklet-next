// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 coverage of the proxy page view state (U2, break-map row U-2).

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import ProxyPage from "./ProxyPage";
import { fixture, instance, Providers } from "./proxy.test-support";

afterEach(() => vi.unstubAllGlobals());

async function page(f = fixture()) {
  vi.stubGlobal("fetch", f.fetcher);
  render(
    <Providers>
      <ProxyPage />
    </Providers>,
  );
  return f;
}
async function choose(name: RegExp | string) {
  const user = userEvent.setup();
  screen.getByRole("combobox", { name: "Configured instances" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name }));
}
const own = new RegExp(`^${instance.name} · `);

it("U-2 one instance: Add, then choosing that instance in the real Select returns to its workspace", async () => {
  await page();
  await screen.findByRole("tab", { name: "Routes" });
  await userEvent.click(screen.getByRole("button", { name: "Add instance" }));
  await screen.findByRole("heading", { name: "Set up proxy" });
  expect(screen.getByRole("combobox", { name: "Configured instances" }).textContent).toBe("New instance…");
  await choose(own);
  await screen.findByRole("tab", { name: "Routes" });
  expect(screen.queryByRole("heading", { name: "Set up proxy" }) === null).toBe(true);
});

it("U-2 the New instance item enters setup, and Back returns to the last instance", async () => {
  await page();
  await screen.findByRole("tab", { name: "Routes" });
  await choose("New instance…");
  await screen.findByRole("heading", { name: "Set up proxy" });
  await userEvent.click(screen.getByRole("button", { name: "Back to instances" }));
  await screen.findByRole("tab", { name: "Routes" });
});

it("U-2 without instances setup is the page: no instance Select and no Add", async () => {
  const f = fixture();
  f.state.instances = [];
  await page(f);
  await screen.findByRole("heading", { name: "Set up proxy" });
  expect(screen.queryByRole("combobox", { name: "Configured instances" }) === null).toBe(true);
  expect(screen.queryByRole("button", { name: "Add instance" }) === null).toBe(true);
  expect(screen.queryByRole("button", { name: "Back to instances" }) === null).toBe(true);
});
