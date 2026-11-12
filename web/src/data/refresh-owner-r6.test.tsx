// Source: docklet-next/issues/391
// Revision: 971f68c9d6b9604ecbb3d8b3892dd93e585ef44a
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review round 6 (R6G-1, R6G-2) and their controls at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useDashboardEvents, useEvents, useStacks, useContainers, useImages, useVolumes, useNetworkSummaries, useTopology, useSecurity, useActiveSecurity } from "./queries";
import { securityFixture } from "./http/vulnerabilities.test-support";
import { hardeningFixture } from "./http/web-hardening.test-support";
import { snapshot } from "./http/runtime-commands.test-support";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };
const frame = (f: ReturnType<typeof recoveryFixture>, id = "") => f.source.onmessage?.({data:JSON.stringify({channel:"events",timestamp:"2026-11-11T12:00:00Z",eventLog:{type:"changed"}}),lastEventId:id} as MessageEvent);

it.each([{stream:false,current:"offline"},{stream:true,current:"offline"},{stream:false,current:"online"},{stream:true,current:"online"}])("R6 policy: old catalogue cannot reverse a newer reconnect catalogue $current, stream=$stream", async ({stream,current:status}) => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeChanges(() => {});
  await f.adapter.listImages(undefined, undefined, "background");
  await tick(10_001); f.state.edge=status==="online"?"offline":"online";f.state.hold.add("/api/environments");
  const old = f.adapter.listHosts(); await tick(50);
  expect(f.pending.filter(p => p.path === "/api/environments")).toHaveLength(1);
  f.state.edge = status;
  f.source.onopen?.({} as Event); // Reopen invalidates the catalogue before the first stream row.
  if (stream) f.send();
  const current = f.adapter.listHosts(); await tick(50);
  const held = f.pending.filter(p => p.path === "/api/environments");
  expect(held).toHaveLength(2);
  held[1]!.release();
  expect((await current).find(h => h.id === "second")?.status).toBe(status);
  held[0]!.release(); await old;
  f.state.hold.clear(); f.requests.length = 0;
  await f.adapter.listImages(undefined, undefined, "background");
  stop();
  expect(f.requests.filter(p => p.path.endsWith("second/images"))).toHaveLength(status==="offline"?0:1);
});

it.each(["dashboard", "history"] as const)("R6 events %s: spaced idle frames produce one physical read each and publish the newest data", async kind => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client=useQueryClient(); return kind === "dashboard" ? <Dashboard /> : <History />; }
  function Dashboard() { useDashboardEvents(); return null; }
  function History() { useEvents(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  f.source.onopen?.({} as Event); frame(f); await tick(100);
  const key = kind === "dashboard" ? ["dashboard-events"] : ["events"];
  f.requests.length = 0;
  for (let n=1; n<=3; n++) { f.state.eventTotal=n; frame(f); await tick(100); }
  expect(client.getQueryState(key)?.fetchStatus).toBe("idle");
  expect(f.requests.filter(p => p.path === "/api/events")).toHaveLength(3);
  const value=client.getQueryData<{totalItems:number}|unknown[]>(key);
  expect(Array.isArray(value)?value.length:value?.totalItems).toBe(3);
});

it.each([false,true])("R6 Dashboard initial frame within bound is absorbed, late=%s", async late => {
  vi.useFakeTimers(); const f = recoveryFixture(); await f.adapter.getCurrentUser(); let client!: QueryClient;
  function Reader() { client=useQueryClient(); useDashboardEvents(); return null; }
  const tree = (mounted:boolean) => <AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}>{mounted?<Reader/>:null}</IntlProvider></AdapterProvider>;
  const view=render(tree(false)); await tick(100);
  expect(f.source.onopen).toBeTypeOf("function"); f.source.onopen?.({} as Event);
  view.rerender(tree(true)); await tick(late?1100:200);
  frame(f); await tick(100);
  expect(client.getQueryState(["dashboard-events"])?.fetchStatus).toBe("idle");
  expect(f.requests.filter(p => p.path === "/api/events")).toHaveLength(late?2:1);
});

it("R6 Dashboard sequence gap queues one fresh pending read and publishes it", async () => {
  vi.useFakeTimers(); const f=recoveryFixture(); await f.adapter.getCurrentUser(); let client!:QueryClient;
  function Reader(){client=useQueryClient();useDashboardEvents();return null;}
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader /></IntlProvider></AdapterProvider>);
  f.source.onopen?.({} as Event);frame(f,"1");await tick(100);f.requests.length=0;
  f.state.hold.add("/api/events");frame(f,"2");await tick(50);
  f.state.eventTotal=2;frame(f,"4");await tick(50);f.release();await tick(100);
  expect(client.getQueryState(["dashboard-events"])?.fetchStatus).toBe("idle");
  expect(client.getQueryData<{totalItems:number}>(["dashboard-events"])?.totalItems).toBe(2);
  expect(f.requests.filter(p=>p.path==="/api/events")).toHaveLength(2);
});

it.each(["stacks","containers","images","volumes","networks","topology"] as const)("R6 inventory %s absorbs changes before its catalogue boundary", async kind=>{
  vi.useFakeTimers();const f=recoveryFixture();await f.adapter.getCurrentUser();let client!:QueryClient;
  function Reader(){client=useQueryClient();return kind==="stacks"?<Stacks/>:kind==="containers"?<Containers/>:kind==="images"?<Images/>:kind==="volumes"?<Volumes/>:kind==="networks"?<Networks/>:<Topology/>;}
  function Stacks(){useStacks();return null;} function Containers(){useContainers();return null;} function Images(){useImages();return null;} function Volumes(){useVolumes();return null;} function Networks(){useNetworkSummaries();return null;} function Topology(){useTopology();return null;}
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader/></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event);f.send();await tick(100);
  const key=[kind==="networks"?"network-summaries":kind]; expect(client.getQueryState(key)?.status).toBe("success");
  f.requests.length=0;f.state.hold.add("/api/capabilities");f.state.edge="offline";f.send();await tick(50);
  expect(f.pending.some(p=>p.path==="/api/capabilities")).toBe(true);
  f.state.edge="online";f.send();await tick(50);f.release();await tick(100);
  expect(client.getQueryState(key)?.fetchStatus).toBe("idle");
  const suffix=kind==="stacks"?"projects":kind==="topology"?"networks":kind;
  expect(f.requests.filter(p=>p.path.endsWith(`second/${suffix}`))).toHaveLength(1);
});

it("R6 old catalogue versions are collected and identity reset clears the internal cache", async () => {
  vi.useFakeTimers(); const f=recoveryFixture(); await f.adapter.getCurrentUser();
  const fetch=QueryClient.prototype.fetchQuery; let cache:QueryClient|undefined;
  const remember=(client:QueryClient)=>{cache=client;};
  const spy=vi.spyOn(QueryClient.prototype,"fetchQuery").mockImplementation(function(this:QueryClient, options) {
    if(options.queryKey[0]==="environments")remember(this);
    return fetch.call(this,options);
  });
  const off=f.adapter.subscribeChanges(()=>{});
  try {
    for(let n=0;n<4;n++){f.send();await f.adapter.listHosts();await tick(1000);}
    expect(cache?.getQueryCache().getAll()).toHaveLength(4);
    await tick(31_000);
    expect(cache?.getQueryCache().getAll()).toHaveLength(0);
    await f.adapter.listHosts();expect(cache?.getQueryCache().getAll()).toHaveLength(1);
    await f.adapter.login("operator","example-password");
    expect(cache?.getQueryCache().getAll()).toHaveLength(0);
  }finally{off();spy.mockRestore();}
});

it("R6 topology attempt cancellation does not abort a physical topology read", async () => {
  vi.useFakeTimers(); const f=recoveryFixture(); await f.adapter.getCurrentUser(); let client!:QueryClient;
  function Reader(){client=useQueryClient();useTopology();return null;}
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader/></IntlProvider></AdapterProvider>);
  await tick(100); f.source.onopen?.({} as Event);f.send();await tick(100);
  f.state.hold.add("/api/environments/second/networks");f.state.revision++;f.send();await tick(50);
  const pending=f.pending.filter(p=>p.path.endsWith("second/networks"));expect(pending).toHaveLength(1);
  await client.cancelQueries({queryKey:["topology"]});expect(pending.some(p=>p.aborted)).toBe(false);
  f.release();await tick(100);
  expect(client.getQueryState(["topology"])?.fetchStatus).toBe("idle");
});

it.each([false,true])("R6 activity snapshot terminal chain produces only the relevant transition wave, historical=%s", async historical=>{
  vi.useFakeTimers();const f=hardeningFixture();f.inventory.state.hostIds=["0","second"];
  f.state.run=snapshot("second",{type:"project_down",resourceType:"project",resourceId:"project",resourceName:"app",status:historical?"success":"running",...(historical?{endedAt:"2026-11-11T12:00:01Z"}:{})});
  await f.adapter.getCurrentUser();let client!:QueryClient;
  function Reader(){client=useQueryClient();useImages();useContainers();useStacks();useTopology();return null;}
  const terminals=vi.fn();const offTerminal=f.adapter.subscribeStackTerminals!(terminals);
  const offActions=f.adapter.subscribeActions(()=>{});
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader/></IntlProvider></AdapterProvider>);
  await tick(100);f.source.open();await tick(100);
  const send=()=>f.source.onmessage?.({data:JSON.stringify({channel:"activities",timestamp:"2026-11-11T12:00:02Z",activity:{type:"snapshot",environmentId:"second"}}),lastEventId:""} as MessageEvent);
  const images=()=>f.calls.filter(p=>p.includes("/second/images?")).length;
  const before=images();f.state.run={...f.state.run,status:"success",endedAt:"2026-11-11T12:00:01Z"};send();await tick(100);
  expect(terminals).toHaveBeenCalledTimes(historical?0:1);
  expect(images()-before).toBe(historical?0:1);
  const settled=images();send();await tick(31_000);
  expect(images()).toBe(settled);
  expect(client.getQueryState(["images"])?.fetchStatus).toBe("idle");
  offActions();offTerminal();
});

it.each(["security","active"] as const)("R6 Security %s reads a recovered host once when the offline refresh is still before its catalogue", async kind=>{
  vi.useFakeTimers();const f=securityFixture(); f.inventory.state.isEdge=true;await f.adapter.getCurrentUser();let client!:QueryClient;
  function Reader(){client=useQueryClient();return kind==="security"?<Full/>:<Active/>;}
  function Full(){useSecurity();return null;} function Active(){useActiveSecurity();return null;}
  const send=()=>f.source.onmessage?.({data:JSON.stringify({channel:"environments",timestamp:"2026-11-11T12:00:00Z",environment:{environments:["first","second"].map(id=>({id,name:id,apiUrl:"http://agent.example.com",enabled:true,status:f.state.hostStatuses[id]??"online",isEdge:true,registrationRevision:1}))}}),lastEventId:""} as MessageEvent);
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader/></IntlProvider></AdapterProvider>);
  await tick(100);f.source.onopen?.({} as Event);send();await tick(100);
  const key=kind==="security"?["security"]:["security","active"];
  expect(client.getQueryState(key)?.status).toBe("success");expect(client.getQueryState(key)?.fetchStatus).toBe("idle");
  const original=f.fetcher.getMockImplementation()!;let release!:()=>void,blocked=false;
  const gate=new Promise<void>(resolve=>{release=resolve;});
  f.fetcher.mockImplementation(async(input,init)=>{const response=await original(input,init);if(new URL(String(input),"https://hub.example.com").pathname==="/api/capabilities"){blocked=true;await gate;}return response;});
  const suffix=kind==="security"?"/second/images?":"/second/images/vulnerabilities/active-summaries";
  f.fetcher.mockClear();f.state.hostStatuses.second="offline";send();await tick(50);expect(blocked).toBe(true);
  f.state.hostStatuses.second="online";send();await tick(50);
  expect(f.fetcher.mock.calls.filter(([input])=>String(input).includes(suffix))).toHaveLength(0);
  release();await tick(100);
  expect(client.getQueryState(key)?.fetchStatus).toBe("idle");expect(client.getQueryState(key)?.status).toBe("success");
  expect(f.fetcher.mock.calls.filter(([input])=>String(input).includes(suffix))).toHaveLength(1);
});
