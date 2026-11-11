// Source: docklet-next/issues/391
// Revision: a792e790672330dfe6f0661ab63ce359cde21ccb
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review round 4 (R4G-1) and its controls at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useDashboardEvents, useImages } from "./queries";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };

it.each([false, true])("R4G-1 a restart before the offline wave resolves reads the recovered host once (pending=%s)", async (pending) => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  let client!: QueryClient;
  const calls = vi.spyOn(f.adapter, "listImages");
  function Reader() { client = useQueryClient(); useImages(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  f.requests.length = 0; calls.mockClear(); f.state.hold.add("/api/capabilities");
  f.state.edge = "offline"; f.send(); await tick(50);
  expect(f.pending.some(p => p.path === "/api/capabilities")).toBe(true);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("fetching");
  if (pending) { f.state.edge = "pending"; f.send(); await tick(50); }
  f.state.edge = "online"; f.send(); await tick(50);
  expect(f.requests.filter(p => p.path.endsWith("second/images"))).toHaveLength(0);
  f.release(); await tick(100);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  expect(client.getQueryState(["images"])?.status).toBe("success");
  
  expect(f.requests.filter(p => p.path.endsWith("second/images"))).toHaveLength(1);
});

it("R4G transition table preserves visible refusals, enabled, registration and readable changes", async () => {
  const f = recoveryFixture(); await f.adapter.getCurrentUser();
  const changes = vi.fn(); const off = f.adapter.subscribeChanges(changes);
  const emit = (status: string, extra = {}) => {
    const rows = ["first", "second"].map(id => ({ id, name:id, apiUrl:"http://agent.example.com", enabled:true, status:id === "second" ? status : "online", isEdge:id === "second", registrationRevision:1, ...(id === "second" ? extra : {}) }));
    f.source.onmessage?.({data:JSON.stringify({channel:"environments",timestamp:"2026-11-11T12:00:00Z",environment:{environments:rows}}),lastEventId:""} as MessageEvent);
  };
  emit("offline");
  for (const status of ["pending", "error", "offline"]) {
    emit(status); expect(changes.mock.lastCall?.[0]).toEqual(["hosts"]);
  }
  for (const status of ["update-required", "token-rejected", "offline"]) {
    emit(status); expect(changes.mock.lastCall?.[0]).toContain("images");
  }
  emit("pending", {registrationRevision:2}); expect(changes.mock.lastCall?.[0]).toContain("images");
  emit("pending", {registrationRevision:2,enabled:false}); expect(changes.mock.lastCall?.[0]).toContain("images");
  emit("online"); expect(changes.mock.lastCall?.[0]).toContain("images");
  emit("online", {connectedAt:"2026-11-11T12:05:00Z"}); expect(changes.mock.lastCall?.[0]).toContain("images");
  off();
});

it("R4G skipped-to-skipped transitions preserve retained inventory and persistent reasons", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  const calls = vi.spyOn(f.adapter,"listImages");
  function Reader() { client=useQueryClient(); useImages(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  f.state.edge="offline"; f.send(); await tick(100);
  const rows = client.getQueryData(["images"]); expect(rows).toBeDefined();
  calls.mockClear();
  for(const status of ["pending","error","offline"]) { f.state.edge=status; f.send(); await tick(100); }
  expect(client.getQueryData(["images"])).toBe(rows);
  expect(calls).not.toHaveBeenCalled();
  f.state.edge="update-required"; f.send(); await tick(100);
  expect(calls).toHaveBeenCalledTimes(1);
  expect(client.getQueryState(["images"])?.status).toBe("success");
});

it("R4G event owner keeps one running read and one pending read for a warm notification burst", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client=useQueryClient(); useDashboardEvents(); return null; }
  const frame = () => f.source.onmessage?.({data:JSON.stringify({channel:"events",timestamp:"2026-11-11T12:00:00Z",eventLog:{type:"changed"}}),lastEventId:""} as MessageEvent);
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  f.source.onopen?.({} as Event); frame(); await tick(100);
  expect(client.getQueryState(["dashboard-events"])?.status).toBe("success");
  f.requests.length=0; f.state.hold.add("/api/events");
  frame(); await tick(50);
  expect(f.pending.some(p=>p.path === "/api/events")).toBe(true);
  frame(); frame(); await tick(50);
  f.release(); await tick(100);
  expect(client.getQueryState(["dashboard-events"])?.fetchStatus).toBe("idle");
  
  expect(f.requests.filter(p=>p.path === "/api/events")).toHaveLength(2);
});

