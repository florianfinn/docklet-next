// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that the Dashboard's events window and the full history are separate queries (L3-d).

import { act, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import type { Topic } from "./adapter";
import { useDashboardEvents, useEvents } from "./queries";

function Both() {
  const full = useEvents();
  const recent = useDashboardEvents();
  return <p>{`${full.data?.length ?? "-"} ${recent.data?.totalItems ?? "-"}`}</p>;
}

it("L3-d an events invalidation (stream open, Activity Center) refetches the full history only; a window change refetches the window only", async () => {
  let notify: (topics: Topic[]) => void = () => {};
  const demo = createDemoAdapter();
  const adapter = {
    ...demo,
    subscribeChanges: (listener: (topics: Topic[]) => void) => { notify = listener; return () => {}; },
    listEvents: vi.fn(async () => []),
    listDashboardEvents: vi.fn(async () => ({ events: [], totalItems: 0 })),
  };
  renderWithApp(<Both />, adapter);
  await waitFor(() => expect([adapter.listEvents.mock.calls.length, adapter.listDashboardEvents.mock.calls.length]).toEqual([1, 1]));
  act(() => notify(["events"]));
  await waitFor(() => expect(adapter.listEvents.mock.calls.length).toBe(2));
  expect(adapter.listDashboardEvents.mock.calls.length).toBe(1);
  act(() => notify(["dashboardEvents"]));
  await waitFor(() => expect(adapter.listDashboardEvents.mock.calls.length).toBe(2));
  expect(adapter.listEvents.mock.calls.length).toBe(2);
});
