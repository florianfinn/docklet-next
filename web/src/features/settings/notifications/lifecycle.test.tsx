// Source: docklet-next/issues/297
// Revision: d9c8c81f2ef74cb4a47f002e1acb69d85f01c1de
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original provider draft, write ownership and test feedback proofs.

import { CancelledError } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { useNotificationWrites } from "@/data/notification-writes";
import type { NotificationTestResult } from "@/domain/notifications";
import { renderRoutes, renderWithApp } from "@/test/render";

import SettingsPage from "../SettingsPage";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() }, Toaster: () => null }));

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  vi.clearAllMocks();
  simulationStore.reset();
  localStorage.clear();
});

const routes = [{ path: "/settings/:section", element: <SettingsPage /> }];

function barrier() {
  let release!: () => void;
  const promise = new Promise<void>((r) => { release = r; });
  return { promise, release };
}

/** Lets the microtasks queued behind an observed effect run before asserting. */
const settle = () => new Promise((r) => setTimeout(r, 0));
const disabled = (name: string) => (screen.getByRole("button", { name }) as HTMLButtonElement).disabled;

describe("provider choice in the add dialog", () => {
  it("starts every provider choice with an empty draft", async () => {
    renderRoutes(routes, ["/settings/notifications"]);
    await waitFor(() => expect(disabled("Ziel hinzufügen")).toBe(false));
    await userEvent.click(screen.getByRole("button", { name: "Ziel hinzufügen" }));
    let dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Gotify" }));
    await userEvent.type(within(dialog).getByLabelText("Server"), "gotify.example.com");
    await userEvent.click(within(dialog).getByRole("button", { name: "Anderen Kanal wählen" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Gotify" }));
    expect((within(dialog).getByLabelText("Server") as HTMLInputElement).value).toBe("");

    await userEvent.click(within(dialog).getByRole("button", { name: "Anderen Kanal wählen" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Google Chat" }));
    await userEvent.type(within(dialog).getByLabelText("Webhook-Adresse"), "https://chat.example.com/credential");
    await userEvent.click(within(dialog).getByRole("button", { name: "Anderen Kanal wählen" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Generischer Webhook" }));
    dialog = screen.getByRole("dialog");
    expect((within(dialog).getByLabelText("Webhook-Adresse") as HTMLInputElement).value).toBe("");
  });

  it("keeps the provider choice while its save is pending", async () => {
    const adapter = createDemoAdapter();
    const save = adapter.saveNotificationTarget.bind(adapter);
    const held = barrier();
    adapter.saveNotificationTarget = async (input) => { await held.promise; return save(input); };
    renderRoutes(routes, ["/settings/notifications"], adapter);
    await waitFor(() => expect(disabled("Ziel hinzufügen")).toBe(false));
    await userEvent.click(screen.getByRole("button", { name: "Ziel hinzufügen" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Google Chat" }));
    await userEvent.type(within(dialog).getByLabelText("Webhook-Adresse"), "https://chat.example.com/credential");
    await userEvent.click(within(dialog).getByRole("button", { name: "Speichern" }));
    expect((within(dialog).getByRole("button", { name: "Anderen Kanal wählen" }) as HTMLButtonElement).disabled).toBe(true);
    held.release();
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    expect((await adapter.listNotificationTargets()).map((t) => t.provider)).toContain("googlechat");
  });
});

describe("one full-target write per provider", () => {
  it("blocks edit and remove of a provider while its toggle is pending", async () => {
    const adapter = createDemoAdapter();
    const save = adapter.saveNotificationTarget.bind(adapter);
    const held = barrier();
    adapter.saveNotificationTarget = async (input) => { await held.promise; return save(input); };
    renderRoutes(routes, ["/settings/notifications"], adapter);
    await userEvent.click(await screen.findByRole("switch", { name: "Aktiv: ntfy" }));
    expect([disabled("Bearbeiten: ntfy"), disabled("Entfernen: ntfy"), disabled("Bearbeiten: E-Mail")]).toEqual([true, true, false]);
    held.release();
    await waitFor(() => expect(disabled("Bearbeiten: ntfy")).toBe(false));
    const ntfy = (await adapter.listNotificationTargets()).find((t) => t.provider === "ntfy");
    expect(ntfy?.kind === "target" && ntfy.enabled).toBe(false);
  });

  it("keeps a provider busy until the list refresh after its write settles", async () => {
    const adapter = createDemoAdapter();
    const list = adapter.listNotificationTargets.bind(adapter);
    const refresh = barrier();
    let hold = false;
    adapter.listNotificationTargets = async () => { if (hold) await refresh.promise; return list(); };
    const saved = vi.fn();
    const save = adapter.saveNotificationTarget.bind(adapter);
    adapter.saveNotificationTarget = async (input) => { hold = true; const result = await save(input); saved(); return result; };
    renderRoutes(routes, ["/settings/notifications"], adapter);
    await userEvent.click(await screen.findByRole("switch", { name: "Aktiv: ntfy" }));
    await waitFor(() => expect(saved.mock.calls.length).toBe(1));
    await settle();
    expect(disabled("Bearbeiten: ntfy")).toBe(true);
    refresh.release();
    await waitFor(() => expect(disabled("Bearbeiten: ntfy")).toBe(false));
  });

  it("keeps a provider busy after its edit dialog closed with a pending save", async () => {
    const adapter = createDemoAdapter();
    const save = adapter.saveNotificationTarget.bind(adapter);
    const held = barrier();
    adapter.saveNotificationTarget = async (input) => { await held.promise; return save(input); };
    renderRoutes(routes, ["/settings/notifications"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Bearbeiten: E-Mail" }));
    await userEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Speichern" }));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    expect([disabled("Bearbeiten: E-Mail"), disabled("Entfernen: E-Mail"), (screen.getByRole("switch", { name: "Aktiv: E-Mail" }) as HTMLButtonElement).disabled]).toEqual([true, true, true]);
    held.release();
    await waitFor(() => expect(disabled("Bearbeiten: E-Mail")).toBe(false));
  });
});

describe("write ownership across section navigation", () => {
  async function leaveAndReturn(view: ReturnType<typeof renderRoutes>) {
    await view.router.navigate("/settings/autoHeal");
    await waitFor(() => expect(screen.queryByRole("switch", { name: "Aktiv: E-Mail", hidden: true }) === null).toBe(true));
    await view.router.navigate("/settings/notifications");
    await screen.findByRole("switch", { name: "Aktiv: E-Mail" });
  }

  it("keeps a held toggle owning its provider after leaving and returning", async () => {
    const adapter = createDemoAdapter();
    const save = adapter.saveNotificationTarget.bind(adapter);
    const held = barrier();
    adapter.saveNotificationTarget = async (input) => { await held.promise; return save(input); };
    const view = renderRoutes(routes, ["/settings/notifications"], adapter);
    await userEvent.click(await screen.findByRole("switch", { name: "Aktiv: ntfy" }));
    await leaveAndReturn(view);
    expect([disabled("Bearbeiten: ntfy"), disabled("Entfernen: ntfy"), (screen.getByRole("switch", { name: "Aktiv: ntfy" }) as HTMLButtonElement).disabled, disabled("Bearbeiten: E-Mail")]).toEqual([true, true, true, false]);
    held.release();
    await waitFor(() => expect(disabled("Bearbeiten: ntfy")).toBe(false));
    const ntfy = (await adapter.listNotificationTargets()).find((t) => t.provider === "ntfy");
    expect(ntfy?.kind === "target" && ntfy.enabled).toBe(false);
  });

  it("keeps a held create out of the add choices after leaving and returning", async () => {
    const adapter = createDemoAdapter();
    const save = adapter.saveNotificationTarget.bind(adapter);
    const held = barrier();
    adapter.saveNotificationTarget = async (input) => { await held.promise; return save(input); };
    const view = renderRoutes(routes, ["/settings/notifications"], adapter);
    await waitFor(() => expect(disabled("Ziel hinzufügen")).toBe(false));
    await userEvent.click(screen.getByRole("button", { name: "Ziel hinzufügen" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Google Chat" }));
    await userEvent.type(within(dialog).getByLabelText("Webhook-Adresse"), "https://chat.example.com/credential");
    await userEvent.click(within(dialog).getByRole("button", { name: "Speichern" }));
    await leaveAndReturn(view);
    await waitFor(() => expect(disabled("Ziel hinzufügen")).toBe(false));
    await userEvent.click(screen.getByRole("button", { name: "Ziel hinzufügen" }));
    const again = await screen.findByRole("dialog");
    expect([within(again).queryByRole("button", { name: "Google Chat" }) === null, within(again).queryByRole("button", { name: "Gotify" }) !== null]).toEqual([true, true]);
    await userEvent.keyboard("{Escape}");
    held.release();
    await waitFor(() => expect(screen.queryByRole("switch", { name: "Aktiv: Google Chat" }) !== null).toBe(true));
  });

  it("refuses a second write for a held provider at the write boundary", async () => {
    const adapter = createDemoAdapter();
    const held = barrier();
    let owner: ReturnType<typeof useNotificationWrites> | undefined;
    function Probe() {
      owner = useNotificationWrites();
      return null;
    }
    renderWithApp(<Probe />, adapter);
    await waitFor(() => expect(owner !== undefined).toBe(true));
    const first = owner!.write("ntfy", () => held.promise);
    const results = await Promise.allSettled([owner!.write("ntfy", async () => "late"), owner!.write("email", async () => "other")]);
    expect([results[0].status === "rejected" && results[0].reason instanceof CancelledError, results[1].status === "fulfilled" && results[1].value]).toEqual([true, "other"]);
    held.release();
    await first;
    await expect(owner!.write("ntfy", async () => "after")).resolves.toBe("after");
  });
});

describe("test feedback ownership", () => {
  function heldTest(result: () => Promise<NotificationTestResult>) {
    const adapter = createDemoAdapter();
    const held = barrier();
    adapter.testNotificationTarget = async () => { await held.promise; return result(); };
    return { adapter, held };
  }
  const anyToast = () => [toast.success, toast.warning, toast.error].flatMap((f) => vi.mocked(f).mock.calls);

  it("reports a result for the unchanged target", async () => {
    const { adapter, held } = heldTest(async () => ({ code: "notification.test-sent" }));
    renderRoutes(routes, ["/settings/notifications"], adapter);
    const row = (await screen.findByRole("switch", { name: "Aktiv: ntfy" })).closest("li")!;
    await userEvent.click(within(row as HTMLElement).getByRole("button", { name: "Test senden" }));
    held.release();
    await waitFor(() => expect(vi.mocked(toast.success).mock.calls.length).toBe(1));
  });

  it("reports no cancellation for the unchanged target", async () => {
    const { adapter, held } = heldTest(async () => { throw new CancelledError(); });
    renderRoutes(routes, ["/settings/notifications"], adapter);
    const row = (await screen.findByRole("switch", { name: "Aktiv: ntfy" })).closest("li")!;
    const button = within(row as HTMLElement).getByRole("button", { name: "Test senden" }) as HTMLButtonElement;
    await userEvent.click(button);
    held.release();
    await waitFor(() => expect(button.textContent).toBe("Test senden"));
    await settle();
    expect(anyToast()).toEqual([]);
  });

  it("drops a result after its target was removed", async () => {
    const { adapter, held } = heldTest(async () => ({ code: "notification.test-sent", warning: "notification.target-disabled" }));
    renderRoutes(routes, ["/settings/notifications"], adapter);
    const row = (await screen.findByRole("switch", { name: "Aktiv: ntfy" })).closest("li")!;
    await userEvent.click(within(row as HTMLElement).getByRole("button", { name: "Test senden" }));
    await userEvent.click(screen.getByRole("button", { name: "Entfernen: ntfy" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Entfernen" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Bearbeiten: ntfy", hidden: true }) === null).toBe(true));
    held.release();
    await waitFor(() => expect(screen.queryByRole("button", { name: /Test senden/ }) !== null).toBe(true));
    await settle();
    expect(anyToast()).toEqual([]);
  });

  it("drops a result and a cancellation after the section was left", async () => {
    for (const result of [async () => ({ code: "notification.test-failed" as const }), async (): Promise<NotificationTestResult> => { throw new CancelledError(); }]) {
      const { adapter, held } = heldTest(result);
      const view = renderRoutes(routes, ["/settings/notifications"], adapter);
      const row = (await screen.findByRole("switch", { name: "Aktiv: ntfy" })).closest("li")!;
      await userEvent.click(within(row as HTMLElement).getByRole("button", { name: "Test senden" }));
      await view.router.navigate("/settings/autoHeal");
      await waitFor(() => expect(screen.queryByRole("switch", { name: "Aktiv: ntfy", hidden: true }) === null).toBe(true));
      held.release();
      await settle();
      expect(anyToast()).toEqual([]);
      view.unmount();
    }
  });
});
