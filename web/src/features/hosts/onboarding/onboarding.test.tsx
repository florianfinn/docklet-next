import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { AppShell } from "@/app/shell/AppShell";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { renderRoutes } from "@/test/render";

import HostsPage from "../HostsPage";
import OnboardingPage from "./OnboardingPage";
import { onboardingStore } from "./onboarding-draft";

const routes = [
  {
    element: <AppShell />,
    children: [
      { path: "/hosts", element: <HostsPage /> },
      { path: "/hosts/new", element: <OnboardingPage /> },
    ],
  },
];

beforeEach(() => {
  // The host list design of the sidebar filter shows each host as its own button.
  setUiStyle({ hostFilterStyle: "list" });
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 3 }));
});

afterEach(() => {
  onboardingStore.set(null);
  preferencesStore.reset();
  uiStyleStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const next = () => userEvent.click(screen.getByRole("button", { name: "Weiter" }));

describe("host onboarding", () => {
  it("validates, produces edge artifacts and connects the host into the shared host list", async () => {
    const adapter = createDemoAdapter();
    const { router } = renderRoutes(routes, ["/hosts/new"], adapter);
    const name = await screen.findByLabelText("Hostname in Docklet");

    await next();
    expect(screen.getByText("2 Felder prüfen") !== null).toBe(true);
    expect(router.state.location.search).toBe("");

    await userEvent.type(name, "Werkstatt");
    await userEvent.click(screen.getByRole("radio", { name: /Edge/ }));
    await next();
    await screen.findByLabelText("Projektverzeichnis");
    const install = screen.getByLabelText("Installationsverzeichnis des Agents") as HTMLInputElement;
    // Default: the agent's own folder inside the project directory.
    expect(install.value).toBe("/home/docker/docklet-agent");
    // No pre-install check from the hub: the installer checks locally and the agent reports later.
    expect(screen.queryByRole("button", { name: /Verzeichnisrechte prüfen/ }) === null).toBe(true);
    expect(screen.getByRole("heading", { name: "Geprüft wird bei der Installation – lokal auf dem Zielhost" }) !== null).toBe(true);
    const tryInstall = async (value: string) => {
      await userEvent.clear(install);
      await userEvent.type(install, value);
      await next();
    };
    await tryInstall("/home/docker");
    expect(screen.getByText(/^Nicht das Projektverzeichnis selbst/) !== null).toBe(true);
    await tryInstall("/home");
    expect(screen.getByText(/^Darf das Projektverzeichnis nicht enthalten/) !== null).toBe(true);
    await tryInstall("/home/docker/media/agent");
    expect(screen.getByText(/^Höchstens ein eigener Ordner direkt im Projektverzeichnis/) !== null).toBe(true);
    await tryInstall("/home/docker/docklet-agent");

    await screen.findByText(/^DEMO-NUR-ANSICHT-WERKSTATT-/);
    await userEvent.click(screen.getByRole("radio", { name: /Compose/ }));
    const compose = screen.getByLabelText("/home/docker/docklet-agent/compose.yaml");
    expect(compose.textContent?.includes("- /home/docker:/home/docker")).toBe(true);
    expect(compose.textContent?.includes('io.docklet.system: "agent"')).toBe(true);
    expect(compose.textContent?.includes("ports:")).toBe(false);

    // Going back keeps the draft; directories are still editable before registration.
    await userEvent.click(screen.getByRole("button", { name: "Zurück" }));
    expect((screen.getByLabelText("Projektverzeichnis") as HTMLInputElement).value).toBe("/home/docker");
    await next();
    await userEvent.click(await screen.findByRole("button", { name: "Host anlegen und auf Agent warten" }));

    const filterEntry = await screen.findByRole("button", { name: /^Werkstatt, Wartet auf Agent/ });
    expect(filterEntry !== null).toBe(true);
    await userEvent.click(await screen.findByRole("button", { name: "Agent-Start simulieren" }));
    await screen.findByText("Werkstatt ist verbunden", undefined, { timeout: 8000 });
    await screen.findByRole("button", { name: /^Werkstatt, Online/ });

    const host = (await adapter.listHosts()).find((h) => h.name === "Werkstatt");
    expect([host?.status, host?.connection, host?.installDir, host?.projectDir]).toEqual(["online", "edge", "/home/docker/docklet-agent", "/home/docker"]);
    // The agent reports the installer's local check only after connecting.
    expect([host?.agentReport?.projectDirOwner.name, host?.agentReport?.fileOwnerSource, host?.agentReport?.socketGidSource]).toEqual(["docker:docker", "directory-owner", "detected"]);
    // Its compose project sits in the project directory but is a system component, not a stack or container.
    const system = (await adapter.listSystemComponents()).find((c) => c.hostId === "werkstatt");
    expect([system?.insideProjectDir, system?.labels["io.docklet.system"]]).toEqual([true, "agent"]);
    expect((await adapter.listStacks()).some((s) => s.hostId === "werkstatt") || (await adapter.listContainers()).some((c) => c.hostId === "werkstatt")).toBe(false);

    await userEvent.click(screen.getAllByRole("link", { name: "Zur Hostliste" })[0] as HTMLElement);
    const card = await waitFor(() => {
      const el = document.querySelector<HTMLElement>('[data-host="werkstatt"]');
      if (!el) throw new Error("card missing");
      return el;
    });
    expect(within(card).getByText("In dieser Sitzung hinzugefügt") !== null).toBe(true);
    expect(within(card).getByText(/^docker:docker \(1000:1000\)/) !== null).toBe(true);
    expect(await within(card).findByText(/^Liegt im Projektverzeichnis und wird per Label/) !== null).toBe(true);
  }, 20_000);
});
