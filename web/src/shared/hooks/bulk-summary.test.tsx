// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original identity-bound bulk summary ownership across navigation. M19 web consumer integration.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Outlet } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { RuntimeAdmission } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { BulkResultController } from "@/shared/components/bulk-result-dialog";
import { bulkResultStore } from "@/shared/components/bulk-result-store";
import { renderRoutes } from "@/test/render";
import { useBulkRun } from "./use-bulk-run";
const notify = vi.hoisted(() => vi.fn());
vi.mock("sonner", async (load) => { const actual = await load<typeof import("sonner")>(); return { ...actual, toast: Object.assign(notify, { success: vi.fn(), error: vi.fn(), warning: vi.fn() }) }; });
beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); bulkResultStore.reset(); notify.mockReset(); });
async function fixture() {
  const adapter = createDemoAdapter(), stack = (await adapter.listStacks())[0]!;
  let resolve!: (value: RuntimeAdmission) => void;
  const held = new Promise<RuntimeAdmission>((done) => { resolve = done; }); adapter.admitRuntimeCommand = vi.fn(() => held);
  function Start() { const bulk = useBulkRun(); return <button onClick={() => void bulk.run([{ kind: "stack", id: stack.id, name: "Captured stack", host: "Captured host" }], "start")}>Start bulk</button>; }
  return { adapter, resolve, Start };
}
it("bulk summary Details survives navigation and retains captured labels", async () => {
  const f = await fixture();
  const view = renderRoutes([{ element: <><BulkResultController /><Outlet /></>, children: [{ path: "/", element: <f.Start /> }, { path: "/other", element: <p>Other page</p> }] }], ["/"], f.adapter);
  await userEvent.click(screen.getByText("Start bulk")); await waitFor(() => expect(f.adapter.admitRuntimeCommand).toHaveBeenCalledTimes(1));
  await act(() => view.router.navigate("/other"));
  await act(() => f.resolve({ outcome: "started", runId: "accepted" }));
  await waitFor(() => expect(notify).toHaveBeenCalledTimes(1));
  await act(() => notify.mock.calls[0]?.[1].action.onClick());
  expect(screen.queryByRole("dialog") !== null).toBe(true);
  expect(screen.queryByText("Captured stack · Captured host") !== null).toBe(true);
});
it("identity end clears Details and suppresses late summary publication", async () => {
  const f = await fixture(); let version = 0; const listeners = new Set<() => void>();
  registerIdentityLifetime(f.adapter.subscribeChanges, { active: () => version === 0, version: () => version, listen: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; } });
  renderRoutes([{ path: "/", element: <><f.Start /><BulkResultController /></> }], ["/"], f.adapter);
  await userEvent.click(screen.getByText("Start bulk")); await waitFor(() => expect(f.adapter.admitRuntimeCommand).toHaveBeenCalledTimes(1));
  await act(() => { version++; listeners.forEach((fn) => fn()); f.resolve({ outcome: "started", runId: "accepted" }); });
  expect(notify).toHaveBeenCalledTimes(0); expect(bulkResultStore.get()).toEqual({ open: false, results: [] });
});
