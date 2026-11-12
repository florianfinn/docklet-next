// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs for Dashboard readiness markers and hidden-widget reads (L3-i, L3-m).

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";
import { CORE_WIDGETS, WIDGET_IDS, layoutStore } from "./dashboard-layout";
import { firstResult } from "./use-readiness";

beforeEach(() => { simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { vi.restoreAllMocks(); layoutStore.reset(); simulationStore.reset(); localStorage.clear(); });

const marks = () => {
  const el = document.querySelector<HTMLElement>("div.contents");
  return [el?.hasAttribute("data-dashboard-core-ready") ?? false, el?.hasAttribute("data-dashboard-settled") ?? false];
};
function held<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((r) => { resolve = r; }); return { promise, resolve }; }

it("L3-i core-ready waits for the inventory, settled for every shown widget", async () => {
  const demo = createDemoAdapter();
  const stacks = held<Awaited<ReturnType<typeof demo.listStacks>>>();
  const events = held<Awaited<ReturnType<typeof demo.listDashboardEvents>>>();
  const adapter = { ...demo, listStacks: () => stacks.promise, listDashboardEvents: () => events.promise };
  layoutStore.set((l) => ({ ...l, hidden: CORE_WIDGETS.filter((id) => id !== "timeline") }));
  renderWithApp(<DashboardPage />, adapter);
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(marks()).toEqual([false, false]);
  await act(async () => { stacks.resolve(await demo.listStacks()); });
  await waitFor(() => expect(marks()[0]).toBe(true));
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(marks()).toEqual([true, false]);
  await act(async () => { events.resolve({ events: [], totalItems: 0 }); });
  await waitFor(() => expect(marks()).toEqual([true, true]));
});

it("L3-i a failed widget read settles the page with its error shown", async () => {
  const demo = createDemoAdapter();
  const adapter = { ...demo, listDashboardEvents: async () => { throw new Error("example failure"); } };
  layoutStore.set((l) => ({ ...l, hidden: CORE_WIDGETS.filter((id) => id !== "events") }));
  renderWithApp(<DashboardPage />, adapter);
  await waitFor(() => expect(marks()).toEqual([true, true]));
  expect(document.querySelector("[data-widget='events'] [role='alert']") !== null).toBe(true);
});

it("L3-m a hidden schedule widget does not read the schedule; shown, it does", async () => {
  const demo = createDemoAdapter();
  const listSchedule = vi.fn(demo.listSchedule);
  const adapter = { ...demo, listSchedule };
  layoutStore.set((l) => ({ ...l, hidden: [...CORE_WIDGETS] }));
  const view = renderWithApp(<DashboardPage />, adapter);
  await waitFor(() => expect(marks()).toEqual([true, true]));
  expect(listSchedule).not.toHaveBeenCalled();
  view.unmount();
  layoutStore.set((l) => ({ ...l, hidden: CORE_WIDGETS.filter((id) => id !== "schedule") }));
  renderWithApp(<DashboardPage />, adapter);
  await waitFor(() => expect(listSchedule).toHaveBeenCalled());
  await waitFor(() => expect(marks()).toEqual([true, true]));
});

it("L3-i a partial inventory seed (data while still fetching) is not a first complete result", () => {
  expect([
    firstResult({ isError: false, isSuccess: true, fetchStatus: "fetching" }),
    firstResult({ isError: false, isSuccess: true, fetchStatus: "idle" }),
    firstResult({ isError: true, isSuccess: false, fetchStatus: "idle" }),
  ]).toEqual([false, true, true]);
});

it("R1D-2 settled waits for the service summaries, then for the shown service detail", async () => {
  const demo = createDemoAdapter();
  const summary = held<Awaited<ReturnType<NonNullable<typeof demo.listServiceWidgets>>>>();
  const detail = held<Awaited<ReturnType<NonNullable<typeof demo.getServiceWidget>>>>();
  const getServiceWidget = vi.fn(() => detail.promise);
  layoutStore.set((l) => ({ ...l, hidden: WIDGET_IDS.filter((id) => id !== "plex") }));
  renderWithApp(<DashboardPage />, { ...demo, listServiceWidgets: () => summary.promise, getServiceWidget });
  await waitFor(() => expect(marks()[0]).toBe(true));
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  const beforeSummary = marks()[1];
  await act(async () => { summary.resolve((await demo.listServiceWidgets!()).map((w) => w.kind === "plex" ? { ...w, configured: true, enabled: true } : w)); });
  await waitFor(() => expect(getServiceWidget).toHaveBeenCalled());
  const beforeDetail = marks()[1];
  await act(async () => { detail.resolve(await demo.getServiceWidget!("plex")); });
  await waitFor(() => expect(marks()[1]).toBe(true));
  expect([beforeSummary, beforeDetail]).toEqual([false, false]);
});

it("R1D-3 a core inventory error settles with its error shown", async () => {
  const demo = createDemoAdapter();
  layoutStore.set((l) => ({ ...l, hidden: WIDGET_IDS.filter((id) => id !== "events") }));
  renderWithApp(<DashboardPage />, { ...demo, listStacks: async () => { throw new Error("example inventory failure"); } });
  // The error UI is the effect; the markers are read once it is shown.
  await screen.findByRole("alert");
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(marks()).toEqual([true, true]);
});

it("R1D-3 the load widget settles when online hosts' metrics are explicitly unavailable before any sample", async () => {
  const demo = createDemoAdapter();
  const hosts = (await demo.listHosts()).map((h) => ({ ...h, status: "online" as const }));
  layoutStore.set((l) => ({ ...l, hidden: WIDGET_IDS.filter((id) => id !== "load") }));
  let push!: (snapshot: { at: string; simulated: boolean; hosts: Record<string, never>; containers: Record<string, never>; unavailable?: string[] }) => void;
  renderWithApp(<DashboardPage />, { ...demo, listHosts: async () => hosts, subscribeMetrics: (listener) => { push = listener; listener({ at: "2026-11-11T12:00:00Z", simulated: false, hosts: {}, containers: {} }); return () => {}; } });
  await waitFor(() => expect(marks()[0]).toBe(true));
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  const beforeOutcome = marks()[1];
  act(() => push({ at: "2026-11-11T12:00:10Z", simulated: false, hosts: {}, containers: {}, unavailable: hosts.map((h) => h.id) }));
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect([beforeOutcome, marks()[1]]).toEqual([false, true]);
});
