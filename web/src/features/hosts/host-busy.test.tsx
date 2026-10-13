import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import HostsPage from "./HostsPage";

// Runs take several seconds, so a loaded test worker still observes the running state;
// the end of each run is awaited with a generous timeout instead of a fixed delay.
beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 0.5 }));
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const card = (id: string) => document.querySelector<HTMLElement>(`[data-host="${id}"]`) as HTMLElement;

describe("host action buttons", () => {
  it("show the connection check as busy on the card and in the settings dialog until the run ends", { timeout: 40_000 }, async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/hosts", element: <HostsPage /> }], ["/hosts"], adapter);
    await waitFor(() => expect(card("atlas") !== null).toBe(true));
    await userEvent.click(within(card("atlas")).getByRole("button", { name: "Verbindung prüfen" }));
    const busy = await within(card("atlas")).findByRole("button", { name: "Wird geprüft …" });
    expect(busy.getAttribute("aria-busy")).toBe("true");
    // Other hosts stay idle.
    expect(within(card("boreal")).getByRole("button", { name: "Verbindung prüfen" }).hasAttribute("aria-busy")).toBe(false);

    await userEvent.click(within(card("atlas")).getByRole("button", { name: "Einstellungen von Atlas" }));
    const dialog = await screen.findByRole("dialog", { name: "Host Atlas" });
    await userEvent.click(within(dialog).getByRole("tab", { name: "Agent-Container" }));
    expect((await within(dialog).findByRole("button", { name: "Wird geprüft …" })).getAttribute("aria-busy")).toBe("true");

    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Verbindung prüfen" }).hasAttribute("aria-busy")).toBe(false), { timeout: 20_000 });
    expect((await adapter.listActions()).find((r) => r.kind === "reconnect" && r.hostId === "atlas")?.status).toBe("succeeded");
  });

  it("shows the agent restart as busy while its run is running", { timeout: 40_000 }, async () => {
    renderRoutes([{ path: "/hosts", element: <HostsPage /> }], ["/hosts"]);
    await waitFor(() => expect(card("atlas") !== null).toBe(true));
    await userEvent.click(within(card("atlas")).getByRole("button", { name: "Einstellungen von Atlas" }));
    const dialog = await screen.findByRole("dialog", { name: "Host Atlas" });
    await userEvent.click(within(dialog).getByRole("tab", { name: "Agent-Container" }));
    await userEvent.click(await within(dialog).findByRole("button", { name: "Agent neu starten" }));
    const restarting = await within(dialog).findByRole("button", { name: "Startet neu …" });
    expect(restarting.getAttribute("aria-busy")).toBe("true");
    // The connection check is a separate action and stays available.
    expect(within(dialog).getByRole("button", { name: "Verbindung prüfen" }).hasAttribute("aria-busy")).toBe(false);
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "Agent neu starten" }).hasAttribute("aria-busy")).toBe(false), { timeout: 20_000 });
  });
});
