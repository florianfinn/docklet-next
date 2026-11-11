// Source: docklet-next/issues/391
// Revision: 1b1760c732e3d1cc1e2843527d3506ca47403b3f
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review round 5 (R5G-1..3) and their controls at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useDashboardEvents, useEvents, useImages, useSecurity, useTopology } from "./queries";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";
import { eventDemandFixture } from "./http/event-demand.test-support";
import { securityFixture } from "./http/vulnerabilities.test-support";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };

it("a topology refresh waiting before the catalogue reads a recovered Edge host once", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  let client!: QueryClient;
  function Reader() { client = useQueryClient(); useTopology(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  expect(client.getQueryState(["topology"])?.fetchStatus).toBe("idle");
  f.requests.length = 0; f.state.hold.add("/api/capabilities");
  f.state.edge = "offline"; f.send(); await tick(50);
  expect(f.pending.some(p => p.path === "/api/capabilities")).toBe(true);
  f.state.edge = "online"; f.send(); await tick(50);
  expect(f.requests.filter(p => p.path.endsWith("second/networks"))).toHaveLength(0);
  f.release(); await tick(100);
  expect(client.getQueryState(["topology"])?.fetchStatus).toBe("idle");
  expect(client.getQueryState(["topology"])?.status).toBe("success");
  expect(f.requests.filter(p => p.path.endsWith("second/networks"))).toHaveLength(1);
});

it("a Dashboard event frame during a held read publishes the coalesced pending read", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  let client!: QueryClient;
  function Reader() { client = useQueryClient(); useDashboardEvents(); return null; }
  const frame = () => f.source.onmessage?.({data:JSON.stringify({channel:"events",timestamp:"2026-11-11T12:00:00Z",eventLog:{type:"changed"}}),lastEventId:""} as MessageEvent);
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  f.source.onopen?.({} as Event); frame(); await tick(100);
  expect(client.getQueryData<{totalItems:number}>(["dashboard-events"])?.totalItems).toBe(0);
  f.requests.length = 0; f.state.hold.add("/api/events");
  frame(); await tick(50);
  expect(f.pending.some(p => p.path === "/api/events")).toBe(true);
  f.state.eventTotal = 1; frame(); await tick(50);
  f.release(); await tick(100);
  expect(client.getQueryState(["dashboard-events"])?.fetchStatus).toBe("idle");
  expect(f.requests.filter(p => p.path === "/api/events")).toHaveLength(2);
  expect(client.getQueryData<{totalItems:number}>(["dashboard-events"])?.totalItems).toBe(1);
});

it("a full-history event frame during a held read publishes the coalesced pending read", async () => {
  vi.useFakeTimers(); const f = eventDemandFixture(); f.state.rows = f.state.rows.slice(0, 1);
  await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client = useQueryClient(); useEvents(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100);
  expect(client.getQueryData<Array<{id:string}>>(["events"])?.[0]?.id).toBe("event-0");
  let release!: () => void;
  f.state.hold = async response => { await new Promise<void>(resolve => { release = resolve; }); return response; };
  f.notify(); await tick(50); expect(release).toBeTypeOf("function");
  f.state.rows[0] = { ...f.state.rows[0]!, id: "new-event" }; f.notify(); await tick(50);
  f.state.hold = undefined; release(); await tick(100);
  expect(f.calls).toHaveLength(3);
  expect(client.getQueryState(["events"])?.fetchStatus).toBe("idle");
  expect(client.getQueryData<Array<{id:string}>>(["events"])?.[0]?.id).toBe("new-event");
});

it("a pre-change catalogue in flight cannot be absorbed as the new inventory scope", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); f.state.hostIds = ["first"];
  await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client = useQueryClient(); useImages(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  await tick(10_001); f.state.hold.add("/api/environments");
  const oldCatalogue = f.adapter.listHosts(); await tick(50);
  expect(f.pending.some(p => p.path === "/api/environments")).toBe(true);
  f.state.hold.add("/api/capabilities"); f.send(); f.source.onopen?.({} as Event); await tick(50);
  expect(f.pending.some(p => p.path === "/api/capabilities")).toBe(true);
  f.state.hostIds.push("second"); f.send(); await tick(50);
  f.releasePath("/api/capabilities"); await tick(50);
  f.releasePath("/api/environments"); await oldCatalogue; await tick(100);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  expect(client.getQueryState(["images"])?.status).toBe("success");
  expect(client.getQueryData<Array<{hostId:string}>>(["images"])?.some(i => i.hostId === "second")).toBe(true);
});

it("a settled offline Edge Security batch clears its unavailable alert after fresh recovery", async () => {
  vi.useFakeTimers(); const f = securityFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client = useQueryClient(); useSecurity(); return null; }
  const frame = () => f.source.onmessage?.({data:JSON.stringify({channel:"environments",timestamp:"2026-11-11T12:00:00Z",environment:{environments:["first","second"].map(id=>({id,name:id,apiUrl:"http://agent.example.com",enabled:true,status:f.state.hostStatuses[id]??"online",isEdge:id==="second",registrationRevision:1}))}}),lastEventId:""} as MessageEvent);
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); frame(); await tick(100);
  const unavailable = () => client.getQueryData<{unavailable:Array<{hostId:string}>}>(["security"])?.unavailable;
  expect(unavailable()).toEqual([]);
  f.state.hostStatuses.second = "offline"; frame(); await tick(100);
  expect(unavailable()?.some(r => r.hostId === "second")).toBe(true);
  f.state.hostStatuses.second = "online"; frame(); await tick(100);
  expect(client.getQueryState(["security"])?.fetchStatus).toBe("idle");
  expect(unavailable()).toEqual([]);
  const settled = f.fetcher.mock.calls.length;
  await tick(31_000);
  expect(f.fetcher.mock.calls).toHaveLength(settled);
});

it("a change after the image catalogue boundary still repeats the held resource read", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client = useQueryClient(); useImages(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  f.requests.length = 0; f.state.hold.add("/api/environments/second/images");
  f.state.revision++; f.send(); await tick(50);
  expect(f.pending.some(p => p.path.endsWith("second/images"))).toBe(true);
  f.state.revision++; f.send(); await tick(50);
  f.release(); await tick(100);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  expect(f.requests.filter(p => p.path.endsWith("second/images"))).toHaveLength(2);
});

it("a host catalogue read from before a stream frame does not replace the environment state that frame set", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  const changes = vi.fn(); const off = f.adapter.subscribeChanges(changes);
  f.source.onopen?.({} as Event); f.send(); await tick(100);
  await tick(10_001); f.state.hold.add("/api/environments");
  const old = f.adapter.listHosts(); await tick(50);
  expect(f.pending.some(p => p.path === "/api/environments")).toBe(true);
  f.state.revision++; f.send(); await tick(50);
  expect(changes.mock.lastCall?.[0]).toContain("images");
  // The held catalogue answer was produced before the change (registration revision 1).
  f.releasePath("/api/environments"); await old;
  changes.mockClear(); f.send(); await tick(50);
  expect(changes.mock.lastCall?.[0]).toEqual(["hosts"]);
  off();
});
