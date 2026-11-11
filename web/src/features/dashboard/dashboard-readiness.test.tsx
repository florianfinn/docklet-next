// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs for Dashboard readiness markers and hidden-widget reads (L3-i, L3-m).

import { act, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";
import { CORE_WIDGETS, layoutStore } from "./dashboard-layout";
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
