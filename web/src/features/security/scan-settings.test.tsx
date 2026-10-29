import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import SettingsPage from "@/features/settings/SettingsPage";
import { renderRoutes } from "@/test/render";

import SecurityPage from "./SecurityPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const routes = [
  { path: "/settings/:section", element: <SettingsPage /> },
  { path: "/security", element: <SecurityPage /> },
];

function deferred() {
  let resolve!: () => void;
  let reject!: (e: Error) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("scan schedule settings", () => {
  it("saves a weekly scan with weekday and time and shows it on the security page", async () => {
    const adapter = createDemoAdapter();
    const { router } = renderRoutes(routes, ["/settings/security"], adapter);
    const recurrence = await screen.findByRole("radiogroup", { name: "Wiederholung" });
    expect(screen.getByText("Uhrzeit und Wochentag gelten in der Zeitzone des Hubs (Europe/Berlin), nicht in der deines Browsers.") !== null).toBe(true);
    expect(within(recurrence).getByRole("radio", { name: "Täglich" }).getAttribute("aria-checked")).toBe("true");

    await userEvent.click(within(recurrence).getByRole("radio", { name: "Wöchentlich" }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Wochentag" })).getByRole("radio", { name: "Mittwoch" }));
    await userEvent.click(screen.getByRole("button", { name: "Uhrzeit: 03:00" }));
    await userEvent.click(within(await screen.findByRole("radiogroup", { name: "Stunde" })).getByRole("radio", { name: "04" }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Minute" })).getByRole("radio", { name: "15" }));
    await waitFor(() => expect(screen.queryByRole("radiogroup", { name: "Minute" })).toBe(null));
    expect(screen.getByText(/^Nächste Ausführung: Mi\., .* 04:15 · in .* · Zeitzone Europe\/Berlin \((MEZ|MESZ)\)$/) !== null).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Alles gespeichert"));
    // Flush the saved render's navigation-blocker effect before programmatic navigation.
    await act(async () => {
      expect((await adapter.getSettings()).security.schedule).toEqual({ mode: "weekly", day: "wed", time: "04:15" });
      expect((await adapter.getSecurity()).schedule).toEqual({ mode: "weekly", day: "wed", time: "04:15" });
    });

    await act(() => router.navigate("/security"));
    expect(router.state.location.pathname).toBe("/security");
    const link = await screen.findByRole("link", { name: /^Scan-Zeitplan: Mittwochs 04:15\. Nächster Scan Mi\., / });
    expect(link.getAttribute("href")).toBe("/settings/security#scan-schedule");
    expect(link.textContent).toBe("Mittwochs 04:15");
  });

  it("opens the schedule from the security page link with focus on it", async () => {
    const { router } = renderRoutes(routes, ["/security"]);
    const link = await screen.findByRole("link", { name: /^Scan-Zeitplan: Täglich 03:00\./ });
    await userEvent.click(link);
    await waitFor(() => expect(router.state.location.pathname + router.state.location.hash).toBe("/settings/security#scan-schedule"));
    await waitFor(() => expect(document.activeElement?.id).toBe("scan-schedule"));
    expect(within(document.activeElement as HTMLElement).getByRole("radiogroup", { name: "Wiederholung" }) !== null).toBe(true);
  });

  it("disables the schedule while scans are off, blocks invalid times and discards edits", async () => {
    const adapter = createDemoAdapter();
    await adapter.saveSettings("security", { scanEnabled: false, schedule: { mode: "daily", time: "" } });
    renderRoutes(routes, ["/settings/security"], adapter);
    const recurrence = await screen.findByRole("radiogroup", { name: "Wiederholung" });
    // happy-dom does not apply fieldset disabling to :disabled; the browser check covers the effect.
    expect((recurrence.closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
    expect(screen.getByText("Der Zeitplan greift erst, wenn Scans aktiv sind.") !== null).toBe(true);

    await userEvent.click(screen.getByRole("switch", { name: "Scans aktivieren" }));
    expect(screen.getByText("Uhrzeit wählen.") !== null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    expect(screen.getByRole("status").textContent).toBe("Bitte markierte Felder prüfen");
    expect((await adapter.getSettings()).security.scanEnabled).toBe(false);

    await userEvent.click(screen.getByRole("button", { name: "Verwerfen" }));
    expect(screen.getByRole("switch", { name: "Scans aktivieren" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("status").textContent).toBe("Alles gespeichert");
  });
});

describe("scan switch on the security page", () => {
  it("switches at once, stays busy until the adapter answers and keeps the result", async () => {
    const adapter = createDemoAdapter();
    const save = deferred();
    const real = adapter.setScanEnabled;
    adapter.setScanEnabled = async (on) => {
      await save.promise;
      return real(on);
    };
    renderRoutes(routes, ["/security"], adapter);
    const toggle = await screen.findByRole("switch", { name: "Scans aktiv" });
    await userEvent.click(toggle);
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect((toggle as HTMLButtonElement).disabled && screen.getByRole("status", { name: "Wird gespeichert …" }) !== null).toBe(true);
    expect(screen.getByText("Schwachstellenscans sind deaktiviert") !== null).toBe(true);

    save.resolve();
    await waitFor(() => expect((screen.getByRole("switch", { name: "Scans aktiv" }) as HTMLButtonElement).disabled).toBe(false));
    expect(screen.getByRole("switch", { name: "Scans aktiv" }).getAttribute("aria-checked")).toBe("false");
    expect((await adapter.getSecurity()).enabled).toBe(false);
    expect((await adapter.getSettings()).security.scanEnabled).toBe(false);
  });

  it("restores the previous state when saving fails", async () => {
    const adapter = createDemoAdapter();
    const save = deferred();
    adapter.setScanEnabled = () => save.promise;
    renderRoutes(routes, ["/security"], adapter);
    const toggle = await screen.findByRole("switch", { name: "Scans aktiv" });
    await userEvent.click(toggle);
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    save.reject(new Error("Hub nicht erreichbar"));
    await waitFor(() => expect(screen.getByRole("switch", { name: "Scans aktiv" }).getAttribute("aria-checked")).toBe("true"));
    expect((screen.getByRole("switch", { name: "Scans aktiv" }) as HTMLButtonElement).disabled).toBe(false);
    expect((await adapter.getSecurity()).enabled).toBe(true);
  });

  it("waits for the simulated latency, fails with the lab's forced failure and reports both topics on success", async () => {
    const adapter = createDemoAdapter();
    const topics: string[][] = [];
    adapter.subscribeChanges((t) => topics.push([...t]));
    simulationStore.set((s) => ({ ...s, latency: "short", nextActionFails: "start" }));
    const failed = await adapter.setScanEnabled(false).catch((e: unknown) => e);
    expect([failed instanceof Error && failed.message.includes("Speichern"), (await adapter.getSecurity()).enabled, simulationStore.get().nextActionFails]).toEqual([true, true, "none"]);
    const started = performance.now();
    await adapter.setScanEnabled(false);
    expect([performance.now() - started >= 450, topics.at(-1)]).toEqual([true, ["security", "settings"]]);
  });
});
