import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import { restartSetup } from "./setup-draft";
import SetupPage from "./SetupPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  restartSetup();
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const next = () => userEvent.click(screen.getByRole("button", { name: "Weiter" }));

describe("first-run setup", () => {
  it("validates each step, keeps the draft on back navigation and saves without secrets", async () => {
    const adapter = createDemoAdapter();
    const { router } = renderRoutes([{ path: "/setup", element: <SetupPage /> }, { path: "/dashboard", element: <p>Dashboard</p> }], ["/setup"], adapter);

    // The wizard starts with access: sign-in mode and administrator.
    await userEvent.click(await screen.findByRole("radio", { name: /Reverse Proxy/ }));
    await userEvent.click(screen.getByRole("button", { name: "Authentik-Beispiel einsetzen" }));
    expect((screen.getByLabelText("Header für Benutzername") as HTMLInputElement).value).toBe("X-authentik-username");
    await userEvent.click(screen.getByRole("radio", { name: /Lokaler Admin/ }));
    await userEvent.type(screen.getByLabelText("Neues Passwort"), "ein-sehr-langes-passwort");
    await userEvent.type(screen.getByLabelText("Passwort wiederholen"), "ein-sehr-langes-passwort");
    await next();

    // Discovery prefills the LAN address of the hub container; clearing it is a manual choice that stays.
    await waitFor(() => expect((screen.getByLabelText("IP-Adresse") as HTMLInputElement).value).toBe("192.0.2.10:8080"));
    await userEvent.clear(screen.getByLabelText("IP-Adresse"));
    await next();
    expect(screen.getByText("1 Feld prüfen") !== null).toBe(true);
    // The internal address comes first; a domain stays optional and is not required.
    expect((screen.getByLabelText("Domain") as HTMLInputElement).value).toBe("");
    await userEvent.type(screen.getByLabelText("IP-Adresse"), "192.0.2.10:8080");
    await next();

    expect(await screen.findByLabelText("Standard-Projektverzeichnis") !== null).toBe(true);
    await next();
    await userEvent.click(await screen.findByRole("switch", { name: "Proxy" }));
    await userEvent.click(screen.getByRole("button", { name: "Zurück" }));
    await next();
    expect(screen.getByRole("switch", { name: "Proxy" }).getAttribute("aria-checked")).toBe("true");
    await next();

    await userEvent.click(await screen.findByRole("button", { name: "Überspringen" }));
    await userEvent.click(await screen.findByRole("button", { name: "Bearbeiten: Hub" }));
    expect((screen.getByLabelText("IP-Adresse") as HTMLInputElement).value).toBe("192.0.2.10:8080");
    expect(screen.getByText(/Links zeigen auf http:\/\/192\.0\.2\.10:8080/) !== null).toBe(true);
    await waitFor(() => expect(router.state.location.search).toBe("?step=hub"));
    await userEvent.click(screen.getByRole("button", { name: /Prüfen/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Einrichtung abschließen" }));

    // Finish shows the Docklet loading animation and then opens the dashboard; there is no Done step.
    expect(await screen.findByText("Docklet wird vorbereitet …") !== null).toBe(true);
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"), { timeout: 4000 });
    const saved = await adapter.getSettings();
    expect(await adapter.getModuleStates!()).toEqual({ proxy: true });
    expect([saved.setupCompleted, saved.general.hubAddress, saved.general.domainUrl, saved.auth.mode, saved.auth.local.passwordSet]).toEqual([true, "192.0.2.10:8080", "", "local", true]);
    expect(JSON.stringify(saved).includes("ein-sehr-langes-passwort")).toBe(false);

  });
});
