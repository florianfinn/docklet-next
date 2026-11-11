// Source: docklet-next/issues/391
// Revision: acecb815f9b0b9c1ff6d7e2433dd489547baa335
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review round 3 (R3G-1..R3G-3) at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { QueryClient, QueryObserver, useQueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useImages, useSchedule } from "./queries";
import { useBackupOverviews } from "./backup-queries";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";
import { createAppQueryClient } from "./app-query-client";
import { refresh, MAX_SUPERSEDED_REPEATS } from "./refresh-owner";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };

it.each(["images", "backup-overviews"] as const)("R3G-1 a first load of %s after a partial batch joins a change: one attempt, its batches stay current", async (kind) => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  const suffix = kind === "images" ? "images" : "volumes/backups/overview";
  f.state.hold.add(`/api/environments/second/${suffix}`);
  const call = vi.spyOn(f.adapter, kind === "images" ? "listImages" : "listBackupOverviews");
  let client!: QueryClient;
  function Client() { client = useQueryClient(); return null; }
  function Images() { useImages(); return null; }
  function Backups() { useBackupOverviews(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Client />{kind === "images" ? <Images /> : <Backups />}</IntlProvider></AdapterProvider>);
  await tick(100);
  expect(client.getQueryData<unknown[]>([kind])!.length).toBeGreaterThan(0);
  expect(client.getQueryState([kind])?.fetchStatus).toBe("fetching");
  const oldPartials: number[] = [];
  const off = client.getQueryCache().subscribe((e) => {
    if (e.type === "updated" && e.query.queryKey[0] === kind && e.action.type === "success" && e.action.manual && call.mock.calls.length === 1)
      oldPartials.push((client.getQueryData<unknown[]>([kind]) ?? []).length);
  });
  f.source.onopen?.({} as Event); await tick();
  f.release(); await tick(100); off();
  
  expect(call.mock.calls.length).toBe(1);
});

it("R3G control: warm overview repeats after an explicit registration change", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client = useQueryClient(); useBackupOverviews(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  f.state.edge = "offline"; f.send(); await tick(100);
  expect(client.getQueryState(["backup-overviews"])?.fetchStatus).toBe("idle");
  f.requests.length = 0;
  f.state.hold.add("/api/environments/second/volumes/backups/overview");
  f.state.edge = "online"; f.send(); await tick(100);
  expect(f.pending.filter(p => p.path.endsWith("second/volumes/backups/overview"))).toHaveLength(1);
  // The producer's connectedAt/session/report updates also change environmentKey while data is already loaded.
  f.state.revision++; f.send(); await tick(100);
  f.release(); await tick(100);
  
  expect(f.requests.filter(p => p.path.endsWith("second/volumes/backups/overview"))).toHaveLength(2);
});

it("R3G bound and tail leave no subscription after normal settlement", async () => {
  const client = createAppQueryClient(), cache = client.getQueryCache(); let calls = 0;
  client.setQueryData(["bound"], 0);
  const subscribe = cache.subscribe.bind(cache); let active = 0, max = 0;
  vi.spyOn(cache, "subscribe").mockImplementation(listener => { active++; max = Math.max(active,max); const off=subscribe(listener); return () => {active--; off();}; });
  const value = await client.fetchQuery({queryKey:["bound"], staleTime:0, queryFn: async () => { calls++; refresh(client,[["bound"]]); await Promise.resolve(); return calls; }});
  expect([value,calls,max,active]).toEqual([MAX_SUPERSEDED_REPEATS+1,MAX_SUPERSEDED_REPEATS+1,1,0]);
});

it.each(["backup-overviews", "schedule"] as const)("R3G-2 a first load of %s recovers from a version-fencing environment frame", async (kind) => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  for (const host of ["first", "second"]) f.state.hold.add(`/api/environments/${host}/volumes/backups/overview`);
  let client!: QueryClient;
  function Client() { client = useQueryClient(); return null; }
  function Backups() { useBackupOverviews(); return null; }
  function Schedule() { useSchedule(["first", "second"]); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Client />{kind === "backup-overviews" ? <Backups /> : <Schedule />}</IntlProvider></AdapterProvider>);
  await tick(100); expect(f.pending.filter(p => p.path.endsWith("/overview"))).toHaveLength(2);
  const key = kind === "backup-overviews" ? [kind] : [kind,["first","second"]];
  expect(client.getQueryData(key)).toBeUndefined();
  f.state.edge = "offline"; f.send(); await tick(); f.release(); await tick(100);
  
  expect(client.getQueryState(key)?.status).toBe("success");
});

it("R3G-3 warm inventory recovery across offline, pending and online frames reads the Edge host once", async () => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client = useQueryClient(); useImages(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event); f.send(); await tick(100);
  f.state.edge = "offline"; f.send(); await tick(100);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  f.requests.length = 0; f.state.hold.add("/api/capabilities");
  f.state.edge = "pending"; f.send(); await tick(50);
  expect(f.pending.some(p => p.path === "/api/capabilities")).toBe(true);
  f.state.edge = "online"; f.send(); await tick(50);
  f.release(); await tick(100);
  
  expect(f.requests.filter(p => p.path.endsWith("second/images"))).toHaveLength(1);
});

it("R3G control: an initial value read before a notification joins as in plain TanStack", async () => {
  for (const client of [createAppQueryClient(), new QueryClient({defaultOptions:{queries:{retry:false,staleTime:30000}}})]) {
    let calls=0, finish!: (value:string)=>void;
    const observer = new QueryObserver(client, {queryKey:["initial-control"],queryFn:()=>{calls++;return new Promise<string>(resolve=>{finish=resolve;});}});
    const off=observer.subscribe(()=>{});
    refresh(client,[["initial-control"]]); finish("read-before-change");
    await vi.waitFor(()=>expect(client.getQueryState(["initial-control"])?.fetchStatus).toBe("idle"));
    expect([calls,client.getQueryData(["initial-control"]),client.getQueryState(["initial-control"])?.isInvalidated]).toEqual([1,"read-before-change",false]);
    off();client.clear();
  }
});
