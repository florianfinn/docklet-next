// Source: docklet-next/issues/349
// Revision: 199e12dc93dcb6cc44dd708ac816481c81b6503f
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bulk container restart test (M19 review S4R1-10).

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { ServiceRestartDialog } from "@/shared/components/service-restart";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";
import ContainersPage from "./ContainersPage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); preferencesStore.reset(); localStorage.clear(); });
const routes = [{ path: "/containers", element: <><ContainersPage /><ServiceRestartDialog /></> }];
// One confirmation covers the Compose containers of a bulk restart; cancel admits nothing and keeps the selection (M19 review S4R1-10).
describe("bulk container restart", () => {
  it("asks once for Compose containers, then admits every selected container", async () => {
    preferencesStore.set((p) => ({ ...p, containersMode: "advanced" }));
    const adapter = createDemoAdapter();
    const admit = vi.spyOn(adapter, "admitRuntimeCommand"), single = vi.spyOn(adapter, "runContainerCommand");
    renderRoutes(routes, ["/containers"], adapter);
    for (const name of ["paperless-webserver-1", "paperless-broker-1", "uptime-kuma"]) await userEvent.click(await screen.findByLabelText(`${name} auswählen`));
    expect(screen.getByText("3 Container ausgewählt") instanceof HTMLElement).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Neustart" }));
    const dialog = await screen.findByRole("alertdialog", { name: "2 Compose-Services einzeln neu starten?" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect([admit.mock.calls.length, single.mock.calls.length]).toEqual([0, 0]);
    expect(screen.getByText("3 Container ausgewählt") instanceof HTMLElement).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Neustart" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "2 Services neu starten" }));
    await waitFor(() => expect(admit.mock.calls.length).toBe(3));
    expect(admit.mock.calls.map(([request]) => request.command)).toEqual(["restart", "restart", "restart"]);
    expect(screen.queryAllByRole("alertdialog").length).toBe(0);
  });
});
