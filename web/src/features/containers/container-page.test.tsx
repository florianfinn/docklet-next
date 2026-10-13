import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { ServiceRestartDialog } from "@/shared/components/service-restart";
import { renderRoutes } from "@/test/render";

import ContainerPage from "./ContainerPage";
import ContainersPage from "./ContainersPage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const routes = [
  {
    path: "/containers",
    element: (
      <>
        <ContainersPage />
        <ServiceRestartDialog />
      </>
    ),
  },
  {
    path: "/containers/:containerId",
    element: (
      <>
        <ContainerPage />
        <ServiceRestartDialog />
      </>
    ),
  },
  { path: "/stacks/:stackId", element: <p>Stack-Seite</p> },
];

const restarts = async (adapter: ReturnType<typeof createDemoAdapter>) => (await adapter.listActions()).filter((r) => r.kind === "restart").length;

describe("container inspector and page", () => {
  it("keeps the side sheet to an overview, without lifecycle or edit buttons, and its page link closes it", async () => {
    const { router } = renderRoutes(routes, ["/containers?c=c-uptime-kuma"]);
    const sheet = await screen.findByRole("dialog", { name: "uptime-kuma" });
    expect(["Stoppen", "Neustart", "Bearbeiten & neu erstellen"].map((name) => within(sheet).queryByRole("button", { name }))).toEqual([null, null, null]);
    // Non-modal: no overlay shields toasts or the page.
    expect(document.querySelector('[data-slot="sheet-overlay"]')).toBe(null);
    await userEvent.click(within(sheet).getByRole("link", { name: "Container-Seite öffnen" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/containers/c-uptime-kuma"));
    expect([router.state.location.search, screen.queryByRole("dialog", { name: "uptime-kuma" })]).toEqual(["", null]);
    expect(await screen.findByRole("heading", { level: 1, name: "uptime-kuma" })).toBeTruthy();
    expect(screen.getByRole("log", { name: "Logs" }) instanceof HTMLElement).toBe(true);
  });

  it("restarts a standalone container at once, without a confirmation", async () => {
    const adapter = createDemoAdapter();
    renderRoutes(routes, ["/containers/c-uptime-kuma"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Neustart" }));
    expect(screen.queryByRole("alertdialog")).toBe(null);
    await waitFor(async () => expect(await restarts(adapter)).toBeGreaterThan(0));
    // Standalone edit uses the recreate dialog.
    await userEvent.click(screen.getByRole("button", { name: "Bearbeiten & neu erstellen" }));
    expect(await screen.findByRole("dialog", { name: /bearbeiten & neu erstellen/ })).toBeTruthy();
  });

  it("asks before restarting one Compose service, names its dependents, and only confirm starts it", async () => {
    const adapter = createDemoAdapter();
    const before = await restarts(adapter);
    renderRoutes(routes, ["/containers/c-paperless-webserver"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Neustart" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Nur Service „webserver“ neu starten?" });
    await waitFor(() => expect(within(dialog).getByText(/Wird gebraucht von: broker/) instanceof HTMLElement).toBe(true));
    expect(within(dialog).getByRole("link", { name: "Stack Paperless öffnen" }).getAttribute("href")).toBe("/stacks/paperless");
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect(await restarts(adapter)).toBe(before);

    await userEvent.click(screen.getByRole("button", { name: "Neustart" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Nur diesen Service neu starten" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBe(null));
    expect(await restarts(adapter)).toBe(before + 1);
    // Compose edit leads to the stack editor.
    expect(screen.getByRole("link", { name: "Im Stack bearbeiten" }).getAttribute("href")).toBe("/stacks/paperless?tab=editor");
  });

  it("keeps Unraid containers locked for editing with their provenance note", async () => {
    renderRoutes(routes, ["/containers/c-duplicati"]);
    expect((await screen.findByRole("button", { name: "Bearbeiten gesperrt" })).hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Von Unraid verwaltet") instanceof HTMLElement).toBe(true);
  });

  it("asks from the row menu too, for Compose containers only", async () => {
    renderRoutes(routes, ["/containers"]);
    const menu = (await screen.findAllByRole("button", { name: /: paperless-webserver/ }))[0]!;
    await userEvent.click(menu);
    await userEvent.click(await screen.findByRole("menuitem", { name: "Neustart" }));
    expect(await screen.findByRole("alertdialog", { name: "Nur Service „webserver“ neu starten?" })).toBeTruthy();
  });
});
