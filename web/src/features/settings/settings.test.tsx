import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const routes = [{ path: "/settings/:section", element: <SettingsPage /> }];

describe("hub settings", () => {
  it("tracks unsaved changes, discards them and saves through the adapter", async () => {
    const adapter = createDemoAdapter();
    renderRoutes(routes, ["/settings/general"], adapter);
    const name = (await screen.findByLabelText("Name des Hubs")) as HTMLInputElement;
    expect(screen.getByRole("status").textContent).toBe("Alles gespeichert");

    await userEvent.clear(name);
    await userEvent.type(name, "Werkstatt-Hub");
    expect(screen.getByRole("status").textContent).toBe("Ungespeicherte Änderungen");
    await userEvent.click(screen.getByRole("button", { name: "Verwerfen" }));
    expect(name.value).toBe("Docklet");

    await userEvent.clear(name);
    await userEvent.type(name, "Werkstatt-Hub");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Alles gespeichert"));
    expect((await adapter.getSettings()).general.hubName).toBe("Werkstatt-Hub");
  });

  it("keeps text typed while the save is running", async () => {
    const adapter = createDemoAdapter();
    simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderRoutes(routes, ["/settings/general"], adapter);
      const name = (await screen.findByLabelText("Name des Hubs")) as HTMLInputElement;
      await user.clear(name);
      await user.type(name, "Eins");
      await user.click(screen.getByRole("button", { name: "Speichern" }));
      await user.type(name, "Zwei");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(name.value).toBe("EinsZwei");
      expect(screen.getByRole("status").textContent).toBe("Ungespeicherte Änderungen");
    } finally {
      vi.useRealTimers();
    }
  });

  it("asks before leaving a section with unsaved changes and keeps the draft on cancel", async () => {
    const { router } = renderRoutes(routes, ["/settings/general"]);
    const name = (await screen.findByLabelText("Name des Hubs")) as HTMLInputElement;
    await userEvent.type(name, "-Entwurf");
    await userEvent.click(screen.getByRole("link", { name: "Updates" }));

    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText("Ungespeicherte Änderungen verwerfen?") !== null).toBe(true);
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect(router.state.location.pathname).toBe("/settings/general");
    expect((screen.getByLabelText("Name des Hubs") as HTMLInputElement).value).toBe("Docklet-Entwurf");

    await userEvent.click(screen.getByRole("link", { name: "Updates" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Verwerfen und wechseln" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/updates"));
  });

  it("never stores the entered local admin password", async () => {
    const adapter = createDemoAdapter();
    renderRoutes(routes, ["/settings/auth"], adapter);
    const pw = await screen.findByLabelText("Neues Passwort");
    await userEvent.type(pw, "kurz");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    expect(screen.getByText("Mindestens 12 Zeichen") !== null).toBe(true);

    await userEvent.type(pw, "-aber-jetzt-lang");
    await userEvent.type(screen.getByLabelText("Passwort wiederholen"), "kurz-aber-jetzt-lang");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Alles gespeichert"));
    const saved = await adapter.getSettings();
    expect(saved.auth.local.passwordSet).toBe(true);
    expect(JSON.stringify(saved).includes("kurz-aber-jetzt-lang")).toBe(false);
    expect((pw as HTMLInputElement).value).toBe("");
  });

  it("keeps a username edit made during the first password save valid", async () => {
    const adapter = createDemoAdapter();
    const initial = await adapter.getSettings();
    await adapter.saveSettings("auth", { ...initial.auth, mode: "local", local: { ...initial.auth.local, passwordSet: false } });
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderRoutes(routes, ["/settings/auth"], adapter);
      await user.type(await screen.findByLabelText("Neues Passwort"), "ein-langes-passwort");
      await user.type(screen.getByLabelText("Passwort wiederholen"), "ein-langes-passwort");
      await user.click(screen.getByRole("button", { name: "Speichern" }));
      await user.type(screen.getByLabelText("Benutzername"), "-neu");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect((screen.getByLabelText("Benutzername") as HTMLInputElement).value.endsWith("-neu")).toBe(true);
      expect((screen.getByLabelText("Neues Passwort") as HTMLInputElement).value).toBe("");
      expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(false);
      expect(screen.queryByText("Pflichtfeld") === null && screen.queryByText("Mindestens 12 Zeichen") === null).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it("adds a notification channel chosen at add time and tests it without sending", async () => {
    const adapter = createDemoAdapter();
    renderRoutes(routes, ["/settings/notifications"], adapter);
    await screen.findByText("Admin-Mail");
    expect(screen.queryByText("Gotify-Kanal") === null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Kanal hinzufügen" }));
    await userEvent.click(screen.getByRole("button", { name: "Gotify" }));
    await userEvent.type(screen.getByLabelText("Server-Adresse"), "https://gotify.example.com");
    await userEvent.click(screen.getByRole("button", { name: "Übernehmen" }));
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(async () => expect((await adapter.getSettings()).notifications.map((c) => c.kind)).toEqual(["email", "ntfy", "gotify"]));
    const gotify = (await adapter.getSettings()).notifications.find((c) => c.kind === "gotify");
    expect(gotify?.target).toBe("https://gotify.example.com");
    const result = await adapter.testNotification(gotify!);
    expect(result.ok && result.message.includes("nichts wurde versendet")).toBe(true);
  });
});
