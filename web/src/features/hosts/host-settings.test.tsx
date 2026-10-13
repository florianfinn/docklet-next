import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { errorCode } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import ContainersPage from "@/features/containers/ContainersPage";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import { hostDraftError } from "./host-draft";
import HostsPage from "./HostsPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

describe("host settings", () => {
  it("validates name and colour before the adapter is asked", () => {
    expect([hostDraftError(" ", "#22c55e"), hostDraftError("x".repeat(41), "#22c55e"), hostDraftError("Atlas", "grün"), hostDraftError("Atlas Nord", "#22c55e")]).toEqual(["nameEmpty", "nameLong", "colorInvalid", null]);
  });

  it("renames and recolours a host for every view, refuses duplicates and keeps the id", async () => {
    const adapter = createDemoAdapter();
    expect(await adapter.updateHost("atlas", { name: "boreal", color: "#22c55e" }).catch(errorCode)).toBe("exists");
    expect(await adapter.updateHost("atlas", { name: "Atlas", color: "pink" }).catch(errorCode)).toBe("invalid-color");
    const saved = await adapter.updateHost("atlas", { name: "Atlas Nord", color: "#EC4899" });
    expect([saved.id, saved.name, saved.color]).toEqual(["atlas", "Atlas Nord", "#ec4899"]);
    // Containers and stacks keep their host id; the name and colour come from the one host list.
    const containers = await adapter.listContainers();
    expect(containers.some((c) => c.hostId === "atlas")).toBe(true);
    expect((await adapter.listHosts()).find((h) => h.id === "atlas")?.name).toBe("Atlas Nord");
  });

  it("edits name and colour in an accessible dialog, cancel drops the draft, save updates the page", async () => {
    const adapter = createDemoAdapter();
    renderRoutes(
      [
        { path: "/hosts", element: <HostsPage /> },
        { path: "/containers", element: <ContainersPage /> },
      ],
      ["/hosts"],
      adapter,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Einstellungen von Atlas" }));
    let dialog = await screen.findByRole("dialog", { name: "Host Atlas" });
    const name = within(dialog).getByLabelText("Anzeigename");
    await userEvent.clear(name);
    await userEvent.type(name, "Verworfen");
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    await userEvent.click(screen.getByRole("button", { name: "Einstellungen von Atlas" }));
    dialog = await screen.findByRole("dialog", { name: "Host Atlas" });
    expect((within(dialog).getByLabelText("Anzeigename") as HTMLInputElement).value).toBe("Atlas");
    expect((within(dialog).getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true);

    await userEvent.clear(within(dialog).getByLabelText("Anzeigename"));
    await userEvent.type(within(dialog).getByLabelText("Anzeigename"), "Atlas Nord");
    await userEvent.click(within(dialog).getByRole("radio", { name: "Farbe #ec4899" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBe(null));
    expect(await screen.findByRole("heading", { level: 2, name: "Atlas Nord" })).not.toBe(null);
    expect((await adapter.listHosts()).find((h) => h.id === "atlas")?.color).toBe("#ec4899");
  });

  it("shows the agent's own container with a simulated restart only here", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/hosts", element: <HostsPage /> }], ["/hosts"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Einstellungen von Cedar" }));
    const dialog = await screen.findByRole("dialog", { name: "Host Cedar" });
    await userEvent.click(within(dialog).getByRole("tab", { name: "Agent-Container" }));
    expect(within(dialog).getAllByText("docklet-agent").length).toBeGreaterThan(0);
    // Disconnected host: the restart needs a connection.
    expect((within(dialog).getByRole("button", { name: "Agent neu starten" }) as HTMLButtonElement).disabled).toBe(true);
    expect((await adapter.listContainers()).some((c) => c.name === "docklet-agent")).toBe(false);
    const id = await adapter.restartAgent("atlas");
    expect((await adapter.listActions()).find((r) => r.id === id)?.kind).toBe("agent-restart");
  });
});
