// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original modal, threshold, countdown and dismissal interaction regression coverage. M37 #580: loader per phase.

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import { createHubUpdateSession, type HubUpdateState } from "@/data/http/hub-update-session";
import { activityStore } from "@/features/activity/activity-store";
import { GateScreen } from "../gate-screen";
import { HubUpdateOverlay } from "./HubUpdateOverlay";
import { updateProgress } from "./progress";

const cleanups: (() => void)[] = [];
afterEach(() => { cleanup(); cleanups.splice(0).forEach(f => f()); activityStore.reset(); vi.useRealTimers(); });
function mount(state: HubUpdateState, locale: keyof typeof catalogs = "en-US") {
  vi.useFakeTimers();
  let healthy = false;
  const operation = { opId: "operation", runId: "update-run", targetVersion: "1.0.1", helperRunning: true };
  const json = (body: unknown) => new Response(JSON.stringify(body));
  const session = createHubUpdateSession({ baseUrl: "/api", client: { fetch: async () => json({ version: "1.0.1", eligibility: { state: "checking", manual: null }, operation: { ...operation, phase: healthy ? "settled" : "helper-started", ...(healthy ? { outcome: "healthy" } : {}) } }), token: async () => "example" }, publicFetch: async () => json({ currentVersion: "v1.0.1" }), readRun: vi.fn() });
  session.set({ state, dismissed: false }); const reload = vi.fn(); cleanups.push(session.clear);
  const adapter = { ...createDisconnectedAdapter(), hubUpdateSession: session, getAuthMode: async () => "local" as const };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => client.clear());
  const result = render(<AdapterContext.Provider value={adapter}><QueryClientProvider client={client}><IntlProvider locale={locale} messages={catalogs[locale]}><GateScreen message="Gate error" action={<button>Background retry</button>} /><div role="status" style={{ zIndex: 100 }}>Toast</div><HubUpdateOverlay reload={reload} /></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  return { ...result, session, reload, healthy: () => { healthy = true; }, tick: async (ms: number) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); } };
}
const running = (elapsed = 0): HubUpdateState => ({ phase: "running", target: "1.0.1", startedAt: Date.now() - elapsed, opId: "operation", runId: "update-run", online: false, targetSeen: false, authenticated: true });
it("B6 modal above a gate and toast traps interaction and cannot escape-dismiss", async () => {
  const f = mount(running()); await f.tick(0);
  const dialog = screen.getByRole("dialog");
  expect(dialog.getAttribute("aria-modal")).toBe("true"); expect(dialog.className).toContain("z-[120]");
  expect(document.querySelector('[data-slot="gate-screen"]')?.closest('[aria-hidden="true"]') !== null).toBe(true);
  expect(screen.queryByRole("button", { name: "Background retry" }) === null).toBe(true);
  expect(dialog.querySelector('[data-slot="brand-logo"] svg') !== null).toBe(true);
  expect(dialog.querySelector('[data-slot="brand-loader"]') !== null).toBe(true);
  fireEvent.keyDown(dialog, { key: "Escape" }); expect(f.session.get().dismissed).toBe(false); expect(screen.queryByRole("dialog") !== null).toBe(true);
});
it("B9 estimate threshold, close threshold and cap stay truthful", async () => {
  const f = mount(running());
  expect(updateProgress(60_000)).toBe(90); expect(updateProgress(120_000)).toBeGreaterThan(90); expect(updateProgress(999999999)).toBeLessThan(100);
  expect(screen.queryByText("Taking longer than usual") === null).toBe(true); expect(screen.queryByRole("button", { name: "Close" }) === null).toBe(true);
  await f.tick(59_000); expect(screen.queryByText("Taking longer than usual") === null).toBe(true);
  await f.tick(1000); expect(screen.queryByText("Taking longer than usual") !== null).toBe(true);
  await f.tick(1139_000); expect(screen.queryByRole("button", { name: "Close" }) === null).toBe(true);
  await f.tick(1000); expect(screen.queryByRole("button", { name: "Close" }) !== null).toBe(true);
  expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).not.toBe("100");
});
it("B2 done reloads exactly once after ten seconds or a button; clearing cancels it", async () => {
  const f = mount({ phase: "done", target: "1.0.1" });
  await f.tick(9999); expect(f.reload).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog").querySelector('[data-slot="brand-logo"]') !== null && screen.getByRole("dialog").querySelector('[data-slot="brand-loader"]') === null).toBe(true);
  await f.tick(1); expect(f.reload).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Open Docklet" })); await f.tick(10_000); expect(f.reload).toHaveBeenCalledTimes(1);
  f.unmount();
  const next = mount({ phase: "done", target: "1.0.2" }); fireEvent.click(screen.getByRole("button", { name: "Open Docklet" })); await next.tick(10_000); expect(next.reload).toHaveBeenCalledTimes(1);
  next.unmount(); const cleared = mount({ phase: "done", target: "1.0.3" }); act(() => cleared.session.clear()); await cleared.tick(10_000); expect(cleared.reload).not.toHaveBeenCalled();
});
it("B9 Q7 dismissed progress never reloads; success reopens a fresh countdown", async () => {
  const f = mount({ phase: "idle" });
  await act(async () => { await f.session.start("1.0.1", async () => ({ accepted: true, opId: "operation", runId: "update-run", phase: "reserved" })); });
  await f.tick(20 * 60_000); fireEvent.click(screen.getByRole("button", { name: "Close" }));
  await f.tick(15_000); expect(screen.queryByRole("dialog") === null).toBe(true); expect(f.reload).not.toHaveBeenCalled();
  f.healthy(); await f.tick(3000);
  expect(screen.queryByRole("button", { name: "Open Docklet" }) !== null).toBe(true); await f.tick(9999); expect(f.reload).not.toHaveBeenCalled(); await f.tick(1); expect(f.reload).toHaveBeenCalledTimes(1);
});
it.each(["en-US", "de"] as const)("B3 %s failed and unknown remain distinct and never auto-reload", async locale => {
  const f = mount({ phase: "failed", target: "1.0.1", runId: "update-run", code: "update.self-failed", authenticated: true }, locale);
  expect(screen.getByRole("dialog").textContent).toContain(locale === "de" ? "nicht abgeschlossen" : "did not finish"); await f.tick(60_000); expect(f.reload).not.toHaveBeenCalled();
  act(() => f.session.set({ state: { phase: "unknown", target: "1.0.1", runId: "update-run", versionSeen: "v1.0.1", authenticated: true }, dismissed: false }));
  expect(screen.getByRole("dialog").textContent).toContain(locale === "de" ? "Ergebnis ist unbekannt" : "result is unknown"); expect(screen.getByRole("dialog").textContent).toContain("v1.0.1");
  fireEvent.click(screen.getByRole("button", { name: locale === "de" ? "Lauf ansehen" : "View run" })); expect(activityStore.get().focusRunId).toBe("update-run"); expect(screen.queryByRole("dialog") === null).toBe(true);
});
it("B4 signin has no success claim, timer or protected run link", async () => {
  const f = mount({ phase: "signin", target: "1.0.1", versionSeen: "v1.0.1" });
  expect(screen.getByRole("dialog").textContent).toContain("Sign in to see the result"); expect(screen.queryByRole("button", { name: "View run" }) === null).toBe(true);
  await f.tick(60_000); expect(f.reload).not.toHaveBeenCalled(); fireEvent.click(screen.getByRole("button", { name: "Sign in" })); expect(f.reload).toHaveBeenCalledTimes(1);
});

it("B9 progress remains below one hundred for arbitrarily long updates", () => {
  expect(updateProgress(10_000_000_000)).toBeLessThan(100);
});

it("B6 closing progress restores the connected previous focus target", async () => {
  const f = mount({ phase: "idle" });
  const trigger = screen.getByRole("button", { name: "Background retry" }); trigger.focus();
  act(() => f.session.set({ state: running(20 * 60_000), dismissed: false })); await f.tick(0);
  fireEvent.click(screen.getByRole("button", { name: "Close" })); await f.tick(0);
  expect(document.activeElement === trigger).toBe(true);
});
