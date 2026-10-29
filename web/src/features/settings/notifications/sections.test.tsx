// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original dialog generation and per-host Auto Heal state proofs.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { NotificationTarget } from "@/domain/notifications";
import { renderRoutes } from "@/test/render";

import SettingsPage from "../SettingsPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const routes = [{ path: "/settings/:section", element: <SettingsPage /> }];

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => { resolve = r; });
  return { promise, resolve };
}

describe("notification target dialog generations", () => {
  it("a late save of a closed dialog leaves the next dialog open and unchanged", async () => {
    const adapter = createDemoAdapter();
    const save = adapter.saveNotificationTarget.bind(adapter);
    const late = deferred<NotificationTarget>();
    let calls = 0;
    adapter.saveNotificationTarget = async (input) => {
      calls += 1;
      if (calls === 1) {
        const saved = await save(input);
        return late.promise.then(() => saved);
      }
      return save(input);
    };
    renderRoutes(routes, ["/settings/notifications"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Bearbeiten: E-Mail" }));
    await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(calls).toBe(1));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));

    await userEvent.click(screen.getByRole("button", { name: "Bearbeiten: ntfy" }));
    const second = await screen.findByRole("dialog");
    const topic = within(second).getByLabelText("Topic") as HTMLInputElement;
    await userEvent.clear(topic);
    await userEvent.type(topic, "draft-topic");

    late.resolve(undefined as never);
    await waitFor(async () => expect((await adapter.listNotificationTargets()).length).toBe(2));
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByRole("dialog") === second).toBe(true);
    expect(within(second).getByRole("heading", { name: "ntfy bearbeiten" }) !== null).toBe(true);
    expect(topic.value).toBe("draft-topic");
  });
});

describe("Auto Heal per host", () => {
  it("shows offline, environment-managed and locked hosts without an edit action", async () => {
    const adapter = createDemoAdapter();
    const read = adapter.getAutoHealSettings.bind(adapter);
    adapter.getAutoHealSettings = async (hostId) => ({ ...(await read(hostId)), ...(hostId === "boreal" ? { configurationLocked: true } : {}) });
    renderRoutes(routes, ["/settings/autoHeal"], adapter);
    await screen.findByText(/Durch Umgebungsvariablen gesetzt – nur lesbar/);
    expect(screen.getByText(/UI-Konfiguration ist am Hub abgeschaltet – nur lesbar/) !== null).toBe(true);
    expect(screen.getByText("Host offline") !== null).toBe(true);
    expect(screen.getByRole("button", { name: "Bearbeiten: Atlas" }) !== null).toBe(true);
    for (const host of ["Boreal", "Delta", "Cedar"]) expect(screen.queryByRole("button", { name: `Bearbeiten: ${host}` }) === null).toBe(true);
  });

  it("keeps a custom window and the stored exceptions when another field changes", async () => {
    const adapter = createDemoAdapter();
    const stored = await adapter.getAutoHealSettings("atlas");
    await adapter.saveAutoHealSettings("atlas", { enabled: true, maxRestarts: stored.maxRestarts, windowMinutes: 45, excludedStacks: stored.excludedStacks, excludedContainers: ["proxy"] });
    renderRoutes(routes, ["/settings/autoHeal"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Bearbeiten: Atlas" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("combobox", { name: "Zeitfenster" }).textContent).toBe("45 Minuten");
    await userEvent.click(within(dialog).getByRole("switch", { name: "Unhealthy-Container neu starten" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    const saved = await adapter.getAutoHealSettings("atlas");
    expect([saved.enabled, saved.windowMinutes, saved.excludedStacks, saved.excludedContainers]).toEqual([false, 45, ["craft"], ["proxy"]]);
  });
});
