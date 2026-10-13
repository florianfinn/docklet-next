import { act, fireEvent, screen, within } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { DeploymentsTab } from "@/features/stacks/detail/DeploymentsTab";
import { setUiStyle, uiStyleStore, type ToastDesign } from "@/shared/preferences/ui-style-store";
import { renderWithApp } from "@/test/render";

import { actionDialogStore, closeActionDialog } from "./action-dialog-store";
import { ActionDialog } from "./ActionDialog";
import { ActionToasts } from "./ActionToasts";
import { activityStore, openActivity } from "./activity-store";
import { ActivityCenter } from "./ActivityCenter";
import { AppToaster } from "./AppToaster";
import { deployRuns } from "./deploy-history";
import { outputViewStore } from "./output-view-store";

beforeEach(() => {
  vi.useFakeTimers();
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 1 }));
});

afterEach(() => {
  act(() => {
    toast.dismiss();
    closeActionDialog();
    activityStore.reset();
    outputViewStore.reset();
  });
  vi.useRealTimers();
  simulationStore.reset();
  uiStyleStore.reset();
  localStorage.clear();
});

const tick = (ms: number) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)));
const dialog = () => document.querySelector<HTMLElement>('[data-layer="action-dialog"]');
const toastFor = (title: string) => screen.getAllByText(title).map((el) => el.closest<HTMLElement>('[data-slot="action-toast"]')).find(Boolean) ?? null;

function renderNotifications(adapter = createDemoAdapter()) {
  renderWithApp(
    <>
      <ActionToasts />
      <ActionDialog />
      <ActivityCenter />
      <AppToaster />
    </>,
    adapter,
  );
  return adapter;
}

describe("action dialog for major actions", () => {
  it("opens for a redeploy, keeps its run when a second one starts and switches only on request", async () => {
    const adapter = renderNotifications();
    await act(async () => {
      await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    expect(within(dialog()!).getByRole("heading").textContent).toBe("Pull & Redeploy · Cloud");

    await act(async () => {
      await adapter.runStackCommand("media", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    const offer = dialog()!.querySelector('[data-slot="dialog-waiting"]');
    expect([within(dialog()!).getByRole("heading").textContent, offer?.textContent?.includes("Pull & Redeploy · Media")]).toEqual(["Pull & Redeploy · Cloud", true]);

    fireEvent.click(within(dialog()!).getByRole("button", { name: "Anzeigen" }));
    expect([within(dialog()!).getByRole("heading").textContent, actionDialogStore.get().waiting.length]).toEqual(["Pull & Redeploy · Media", 0]);
  });

  it("stays open after success, closing never stops the action, and the toast reopens it", async () => {
    const adapter = renderNotifications();
    let id = "";
    await act(async () => {
      id = await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    expect([dialog()?.dataset.design, document.querySelectorAll('[data-slot="action-toast"]').length]).toEqual(["focus", 0]);
    fireEvent.click(within(dialog()!).getByRole("button", { name: "Ablauffenster schließen" }));
    expect(dialog()).toBe(null);
    await tick(1500);
    const run = (await adapter.listActions()).find((r) => r.id === id)!;
    expect([run.status, toastFor("Pull & Redeploy · Cloud")?.dataset.status]).toEqual(["running", "running"]);

    fireEvent.click(within(toastFor("Pull & Redeploy · Cloud")!).getByRole("button", { name: "Ablauf öffnen: Pull & Redeploy · Cloud" }));
    expect(dialog()?.dataset.status).toBe("running");
    await tick(400);
    expect(document.querySelectorAll('[data-slot="action-toast"]').length).toBe(0);
    await tick(20000);
    expect([dialog()?.dataset.status, within(dialog()!).getByRole("status").textContent]).toEqual(["succeeded", "Erfolgreich abgeschlossen"]);
  });

  it("does not open for small actions or when automatic opening is off", async () => {
    const adapter = renderNotifications();
    await act(async () => {
      await adapter.checkUpdates({ stackId: "cloud" });
      setUiStyle({ actionPresentationOverride: "toast" });
      await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    expect([dialog(), document.querySelectorAll('[data-slot="action-toast"]').length]).toEqual([null, 2]);
  });
});

describe("action toast size", () => {
  const rows = (el: HTMLElement) => [...el.querySelectorAll<HTMLElement>("[data-slot]")].map((n) => n.dataset.slot!).filter((s) => /^(toast-|step-track)/.test(s) && !s.startsWith("toast-life"));

  it.each<ToastDesign>(["bar", "compact", "edge"])("keeps the same rows from start to failure in the %s design", async (design) => {
    setUiStyle({ toastDesign: design, toastScale: 1.5, actionPresentationOverride: "toast" });
    const adapter = renderNotifications();
    await act(async () => {
      await adapter.runStackCommand("home", "redeploy", "pull");
      await vi.advanceTimersByTimeAsync(50);
    });
    const running = rows(toastFor("Pull & Redeploy · Home")!);
    await tick(20000);
    const failed = toastFor("Pull & Redeploy · Home")!;
    expect([failed.dataset.status, failed.dataset.design, rows(failed)]).toEqual(["failed", design, running]);
    expect(failed.querySelectorAll('[data-slot="step-track"] > li').length).toBe(5);
  });
});

describe("notices and side sheets", () => {
  it("leave toasts usable while the action centre is open and keep the centre open", async () => {
    renderNotifications();
    act(() => openActivity());
    await tick(50);
    act(() => void toast.success("Einstellungen gespeichert"));
    await tick(50);
    expect([document.body.style.pointerEvents, screen.getByRole("dialog", { name: "Aktionszentrum" }) !== null]).toEqual(["", true]);

    const notice = screen.getByText("Einstellungen gespeichert").closest<HTMLElement>("[data-sonner-toast]")!;
    const close = within(notice).getByRole("button", { name: "Meldung schließen" });
    fireEvent.pointerDown(close);
    fireEvent.focus(close);
    fireEvent.click(close);
    await tick(600);
    expect([screen.queryByText("Einstellungen gespeichert"), activityStore.get().open]).toEqual([null, true]);
  });

  it("stack several notices, newest first", async () => {
    renderNotifications();
    act(() => {
      toast.success("Einstellungen gespeichert");
      toast.error("Speichern fehlgeschlagen");
    });
    await tick(50);
    const notices = [...document.querySelectorAll<HTMLElement>("[data-sonner-toast]")];
    expect(notices.map((n) => [n.dataset.type, n.dataset.expanded])).toEqual([
      ["error", "true"],
      ["success", "true"],
    ]);
  });
});

describe("deploy history", () => {
  it("shows one current run with its output, older runs collapsed, and a new run replaces the main view", async () => {
    const adapter = createDemoAdapter();
    const stack = await adapter.getStack("craft");
    const view = renderWithApp(<DeploymentsTab stack={stack} />, adapter);
    const current = () => [...document.querySelectorAll<HTMLElement>('[data-slot="deployment-current"]')].map((n) => n.dataset.run);
    await act(async () => {
      await adapter.runStackCommand("craft", "redeploy");
      await vi.advanceTimersByTimeAsync(20000);
    });
    const all = deployRuns(await adapter.listActions(), stack.deployments, "craft");
    const older = all.length - 1;
    expect([older > 0, current(), screen.getByRole("button", { name: new RegExp(`^${older} frühere?`) }).getAttribute("aria-expanded")]).toEqual([true, [all[0]!.id], "false"]);

    // Pick an older run: it takes over the main view.
    fireEvent.click(screen.getByRole("button", { name: /frühere?/ }));
    const oldest = all.at(-1)!;
    fireEvent.click(screen.getByRole("button", { name: `${oldest.title} anzeigen` }));
    expect([current(), screen.getByText("Ausgewählter Ablauf aus dem Verlauf") !== null]).toEqual([[oldest.id], true]);

    let id = "";
    await act(async () => {
      id = await adapter.runStackCommand("craft", "redeploy");
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect([current(), document.querySelectorAll('[data-slot="run-output"]').length]).toEqual([[id], 1]);
    expect(screen.getByRole("button", { name: new RegExp(`^${older + 1} frühere`) }) !== null).toBe(true);
    view.unmount();
  });

  it("folds layer download progress into one updating line per layer", async () => {
    const adapter = createDemoAdapter();
    const stack = await adapter.getStack("craft");
    renderWithApp(<DeploymentsTab stack={stack} />, adapter);
    await act(async () => {
      await adapter.runStackCommand("craft", "redeploy");
      await vi.advanceTimersByTimeAsync(20000);
    });
    const list = screen.getByRole("list", { name: "Rohausgabe" });
    const layerRows = [...list.querySelectorAll("li")].filter((li) => /^\s*[0-9a-f]{12} /.test(li.lastElementChild?.textContent ?? ""));
    expect(layerRows.length).toBe(3);
    expect(layerRows.every((li) => li.textContent?.includes("Pull complete") && Number(li.getAttribute("data-frames")) > 4)).toBe(true);
  });
});
