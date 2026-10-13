import { renderStackDetail } from "@/data/message-code";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { IntlProvider } from "use-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { messages } from "@/app/i18n/messages";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { StackDetail, UpdateInfo } from "@/domain/model";
import { applySnapshot, captureSnapshot, decodeDesignCode, encodeDesignCode } from "@/shared/preferences/favorite-code";
import { NotificationsSection } from "@/features/settings/sections/NotificationsSection";
import { ActivitySection } from "@/features/settings/sections/SimpleSections";
import { DeploymentsTab } from "@/features/stacks/detail/DeploymentsTab";
import { StackHeader } from "@/features/stacks/detail/StackHeader";
import { StackUpdatePanel, UpdatePanel } from "@/shared/components/updates/update-panel";
import { createStore, useStore } from "@/shared/lib/store";
import { DEFAULT_UI_STYLE, mergeUiStyle, setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { renderWithApp } from "@/test/render";

import { closeActionDialog } from "./action-dialog-store";
import { ActionDialog } from "./ActionDialog";
import { ActionToasts } from "./ActionToasts";
import { activityStore } from "./activity-store";
import { ActivityCenter } from "./ActivityCenter";
import { AppToaster } from "./AppToaster";
import { outputViewStore } from "./output-view-store";
import { presentationStore } from "./run-presentation";

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
  presentationStore.reset();
  localStorage.clear();
});

const tick = (ms: number) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)));
const dialog = () => document.querySelector<HTMLElement>('[data-layer="action-dialog"]');
/** Visible action toasts; dismissed ones stay in the DOM with data-removed until their exit animation. */
const toasts = () => [...document.querySelectorAll<HTMLElement>('[data-slot="action-toast"]')].filter((t) => t.closest("[data-sonner-toast]")?.getAttribute("data-removed") !== "true");

function Notifications() {
  return (
    <>
      <ActionToasts />
      <ActionDialog />
      <ActivityCenter />
      <AppToaster />
    </>
  );
}

const tabShown = createStore(true);
function StackSurface({ stack, inline }: { stack: StackDetail; inline: boolean }) {
  const shown = useStore(tabShown, (v) => v);
  return (
    <>
      <StackHeader stack={stack} host={undefined} inline={inline && shown} />
      {shown && inline && <DeploymentsTab stack={stack} />}
      <Notifications />
    </>
  );
}

describe("start sites decide the presentation", () => {
  afterEach(() => tabShown.reset());

  it("shows a restore started in the Deployments tab only in the tab, and as a toast once the tab is left", async () => {
    const adapter = createDemoAdapter();
    const stack = renderStackDetail(await adapter.getStack("craft"), "de");
    renderWithApp(<StackSurface stack={stack} inline />, adapter);
    fireEvent.click(screen.getAllByRole("button", { name: /Manuell wiederherstellen/ })[0]!);
    await tick(50);
    fireEvent.click(screen.getByRole("button", { name: "Wiederherstellen" }));
    await tick(300);
    const focus = document.querySelector<HTMLElement>('[data-slot="deployment-current"] [data-slot="run-focus"]');
    expect([dialog(), toasts().length, focus?.dataset.status, presentationStore.get().surfaces.craft]).toEqual([null, 0, "running", 1]);

    act(() => tabShown.set(false));
    await tick(50);
    expect(toasts().map((t) => [t.dataset.status, t.textContent?.includes("Wiederherstellung · Craft")])).toEqual([["running", true]]);

    act(() => tabShown.set(true));
    await tick(400);
    expect(toasts().length).toBe(0);
  });

  it("starts a redeploy from the stack header inline on the Deployments tab and in the dialog elsewhere", async () => {
    const adapter = createDemoAdapter();
    const stack = renderStackDetail(await adapter.getStack("home"), "de");
    const view = renderWithApp(<StackSurface stack={stack} inline />, adapter);
    fireEvent.click(screen.getByRole("button", { name: /^Pull & Redeploy$/ }));
    await tick(50);
    expect([dialog(), toasts().length, document.querySelector('[data-slot="deployment-current"] [data-slot="run-focus"]')?.getAttribute("data-status")]).toEqual([null, 0, "running"]);
    await tick(20000);
    view.unmount();

    const other = renderStackDetail(await adapter.getStack("cloud"), "de");
    renderWithApp(<StackSurface stack={other} inline={false} />, adapter);
    fireEvent.click(screen.getByRole("button", { name: /^Pull & Redeploy$/ }));
    await tick(50);
    expect([dialog()?.dataset.design, toasts().length]).toEqual(["focus", 0]);
  });

  it("keeps start, stop and checks as toasts even on the Deployments tab", async () => {
    const adapter = createDemoAdapter();
    const stack = renderStackDetail(await adapter.getStack("home"), "de");
    renderWithApp(<StackSurface stack={stack} inline />, adapter);
    fireEvent.click(screen.getByRole("button", { name: /^Neustart$/ }));
    await tick(50);
    expect([dialog(), toasts().length]).toEqual([null, 1]);
  });
});

describe("clickable action toast", () => {
  it("opens the details from anywhere on the card, while closing does only that", async () => {
    const adapter = createDemoAdapter();
    renderWithApp(<Notifications />, adapter);
    let id = "";
    await act(async () => {
      id = await adapter.checkUpdates({ stackId: "cloud" });
      await vi.advanceTimersByTimeAsync(50);
    });
    const card = toasts()[0]!;
    expect([card.querySelector('[data-slot="toast-footer"]'), within(card).queryByRole("button", { name: /Details|Ablauf öffnen$/ })]).toEqual([null, null]);
    fireEvent.click(card.querySelector('[data-slot="toast-detail"]')!);
    expect([activityStore.get().open, activityStore.get().focusRunId]).toEqual([true, id]);

    act(() => activityStore.reset());
    fireEvent.click(within(toasts()[0]!).getByRole("button", { name: "Hinweis schließen" }));
    await tick(400);
    expect([activityStore.get().open, toasts().length]).toEqual([false, 0]);
  });

  it("offers the title as keyboard entry with the run's name", async () => {
    const adapter = createDemoAdapter();
    renderWithApp(<Notifications />, adapter);
    await act(async () => {
      await adapter.checkUpdates({ stackId: "cloud" });
      await vi.advanceTimersByTimeAsync(50);
    });
    const title = within(toasts()[0]!).getByRole("button", { name: "Im Aktionszentrum öffnen: Update-Prüfung · Cloud" });
    title.focus();
    fireEvent.click(title);
    expect(activityStore.get().open).toBe(true);
  });

  it("colours running segments in the brand tone and settled ones semantically", async () => {
    setUiStyle({ actionPresentationOverride: "toast" });
    const adapter = createDemoAdapter();
    renderWithApp(<Notifications />, adapter);
    await act(async () => {
      await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(3500);
    });
    const tones = () => [...toasts()[0]!.querySelectorAll<HTMLElement>('[data-slot="step-track"] > li')].map((li) => `${li.dataset.state}:${li.dataset.tone}`);
    expect([toasts()[0]!.dataset.tone, tones().filter((x) => x.startsWith("done") || x.startsWith("running")).every((x) => x.endsWith(":run"))]).toEqual(["run", true]);
    await tick(20000);
    expect([toasts()[0]!.dataset.tone, tones().every((x) => x === "done:ok")]).toEqual(["ok", true]);
  });
});

describe("action centre from the dialog", () => {
  it("stays open when the dialog hands over to it", async () => {
    const adapter = createDemoAdapter();
    renderWithApp(<Notifications />, adapter);
    await act(async () => {
      await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    fireEvent.click(within(dialog()!).getByRole("button", { name: "Im Aktionszentrum anzeigen" }));
    await tick(500);
    expect([dialog(), activityStore.get().open, screen.queryByRole("dialog", { name: "Aktionszentrum" }) !== null]).toEqual([null, true, true]);
  });
});

describe("action centre from a toast", () => {
  it("stays open after a real pointer click on a toast while focus was outside", async () => {
    // user-event drives pointer, focus and click in order; it needs real timers.
    vi.useRealTimers();
    const wait = (ms: number) => act(() => new Promise<void>((r) => setTimeout(r, ms)));
    const adapter = createDemoAdapter();
    renderWithApp(
      <>
        <button type="button">Draußen</button>
        <Notifications />
      </>,
      adapter,
    );
    const user = userEvent.setup();
    screen.getByRole("button", { name: "Draußen" }).focus();
    await act(async () => void (await adapter.checkUpdates({ stackId: "cloud" })));
    await wait(50);
    await user.click(toasts()[0]!.querySelector<HTMLElement>('[data-slot="toast-detail"]')!);
    await wait(300);
    expect([activityStore.get().open, screen.queryByRole("dialog", { name: "Aktionszentrum" }) !== null]).toEqual([true, true]);
  });
});

describe("presentation setting", () => {
  it("is a hub setting under Activity with Discard and Save, and new runs follow the saved value", async () => {
    const adapter = createDemoAdapter();
    const settings = await adapter.getSettings();
    renderWithApp(
      <>
        <ActivitySection settings={settings} />
        <Notifications />
      </>,
      adapter,
    );
    fireEvent.click(screen.getByRole("radio", { name: "Nur Meldung" }));
    expect([screen.queryByText("Ungespeicherte Änderungen") !== null, uiStyleStore.get().actionPresentationOverride]).toEqual([true, "hub"]);
    fireEvent.click(screen.getByRole("button", { name: "Verwerfen" }));
    expect(screen.getByRole("radio", { name: "Ablauffenster öffnen" }).getAttribute("aria-checked")).toBe("true");

    fireEvent.click(screen.getByRole("radio", { name: "Nur Meldung" }));
    fireEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await tick(600);
    expect([(await adapter.getSettings()).activity.actionPresentation, screen.queryByText("Ungespeicherte Änderungen")]).toEqual(["toast", null]);

    await act(async () => {
      await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    expect([dialog(), toasts().length]).toEqual([null, 1]);
  });

  it("keeps the design-lab override separate: it wins in this browser, is named in the form and travels in the design code", async () => {
    const adapter = createDemoAdapter();
    const settings = await adapter.getSettings();
    act(() => setUiStyle({ actionPresentationOverride: "toast", runMotion: "stripes" }));
    renderWithApp(
      <>
        <ActivitySection settings={settings} />
        <Notifications />
      </>,
      adapter,
    );
    expect([screen.getByRole("radio", { name: "Ablauffenster öffnen" }).getAttribute("aria-checked"), document.querySelector('[data-slot="presentation-override"]')?.textContent]).toEqual([
      "true",
      "In diesem Browser überschreibt das Design-Labor die Hub-Einstellung mit „Nur Meldung“.",
    ]);
    await act(async () => {
      await adapter.runStackCommand("cloud", "redeploy");
      await vi.advanceTimersByTimeAsync(50);
    });
    expect([dialog(), toasts().length]).toEqual([null, 1]);

    const code = encodeDesignCode(captureSnapshot());
    act(() => uiStyleStore.reset());
    const decoded = decodeDesignCode(code);
    act(() => void (decoded.ok && applySnapshot(decoded.snapshot)));
    const stored = JSON.parse(localStorage.getItem("docklet-react-opus.ui-style") ?? "{}") as { data?: unknown };
    expect([uiStyleStore.get().actionPresentationOverride, uiStyleStore.get().runMotion, mergeUiStyle(stored.data, DEFAULT_UI_STYLE).actionPresentationOverride]).toEqual(["toast", "stripes", "toast"]);
  });

  it("summarises the hub value on the notifications page without its own draft", async () => {
    const adapter = createDemoAdapter();
    const settings = await adapter.getSettings();
    renderWithApp(<NotificationsSection settings={settings} />, adapter);
    const pointer = document.querySelector<HTMLElement>('[data-slot="action-presentation-pointer"]')!;
    expect([pointer.textContent?.includes("Aktuell im Hub: Ablauffenster öffnen."), within(pointer).getByRole("link", { name: "Unter Aktivität ändern" }).getAttribute("href"), screen.queryByRole("radio", { name: "Nur Meldung" })]).toEqual([true, "/settings/activity", null]);
  });
});

describe("update panel times", () => {
  const img = { repository: "example/app", tag: "2026.10.1", version: "2026.10.1" };
  const wrap = (ui: React.ReactNode) =>
    render(
      <IntlProvider locale="de" messages={messages} timeZone="Europe/Berlin">
        {ui}
      </IntlProvider>,
    );

  it("shows the last check and the last recorded update, and names what is unknown", () => {
    const checked = new Date(Date.now() - 2 * 3_600_000).toISOString();
    const updated = new Date(Date.now() - 26 * 3_600_000).toISOString();
    const services = [
      { name: "app", update: { state: "current", current: img, checkedAt: checked, registry: "ghcr.io" } satisfies UpdateInfo },
      { name: "worker", update: { state: "unchecked", current: img } satisfies UpdateInfo },
    ];
    const { container, unmount } = wrap(<StackUpdatePanel services={services} times={{ checkedAt: checked, unchecked: 1, updatedAt: updated }} />);
    const times = container.querySelector<HTMLElement>('[data-slot="update-times"]')!;
    expect([times.querySelector('[data-time="checked"] time')?.getAttribute("datetime"), times.textContent?.includes("1 Image ohne Prüfung"), times.querySelector('[data-time="updated"] time')?.getAttribute("datetime")]).toEqual([checked, true, updated]);
    unmount();

    const none = wrap(<StackUpdatePanel services={services} times={{ checkedAt: checked, unchecked: 1 }} />);
    expect(none.container.querySelector('[data-time="updated"]')?.textContent).toBe("Kein Update-Lauf erfasst");
    none.unmount();

    const single = wrap(
      <>
        <UpdatePanel update={{ state: "unchecked", current: img }} />
        <UpdatePanel update={{ state: "local", current: img }} />
      </>,
    );
    const rows = [...single.container.querySelectorAll('[data-slot="update-times"]')].map((n) => [n.querySelector('[data-time="checked"]')?.textContent, n.querySelector('[data-time="updated"]')]);
    expect(rows).toEqual([
      ["Noch nie geprüft", null],
      ["Lokales Image – keine Prüfung möglich", null],
    ]);
  });
});
