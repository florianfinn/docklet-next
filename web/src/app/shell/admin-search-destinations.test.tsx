// Source: docklet-next/issues/356
// Revision: 199e12dc93dcb6cc44dd708ac816481c81b6503f
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original administrator palette destination test (M19 review S4R1-1).

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";
import { routes } from "@/app/router";

beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());

// Every administrator resource group opens a mounted page under the real routes.
it.each(["Hosts", "Container", "Images", "Volumes", "Netzwerke"])("admin palette %s entry opens a mounted page", async (group) => {
  const view = renderRoutes(routes, ["/dashboard"], createDemoAdapter());
  await userEvent.click(await screen.findByRole("button", { name: "Suchen" }));
  const dialog = await screen.findByRole("dialog");
  const heading = await within(dialog).findByText(group, { selector: "[cmdk-group-heading]" });
  const groupNode = heading.closest("[cmdk-group]") as HTMLElement;
  await waitFor(() => expect(within(groupNode).queryAllByRole("option").length > 1).toBe(true), { timeout: 4000 });
  await userEvent.click(within(groupNode).getAllByRole("option")[0]!);
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  if (group === "Hosts") expect(view.router.state.location.pathname).toBe("/hosts");
  await waitFor(() => expect(screen.queryByText("Seite nicht gefunden") === null).toBe(true));
  expect(screen.queryByText("Seite nicht gefunden") === null).toBe(true);
});
