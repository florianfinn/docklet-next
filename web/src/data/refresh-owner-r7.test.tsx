// Source: docklet-next/issues/391
// Revision: af621519900d0457676ec1cb0c050e459df6e90a
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review round 7 (R7G-1, R7G-2) and their controls at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useSecurity, useActiveSecurity, useSystemComponents } from "./queries";
import { securityFixture } from "./http/vulnerabilities.test-support";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };
function mountSecurity(kind:"security"|"active",f:ReturnType<typeof securityFixture>) {
 let client!:QueryClient;
 function Full(){useSecurity();return null;} function Active(){useActiveSecurity();return null;}
 function Reader(){client=useQueryClient();return kind==="security"?<Full/>:<Active/>;}
 const view=render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader/></IntlProvider></AdapterProvider>);
 const key=kind==="security"?["security"]:["security","active"];
 return {view,key,get client(){return client;}};
}
const scanFrame=(f:ReturnType<typeof securityFixture>)=>f.source.onmessage?.({data:JSON.stringify({channel:"activities",timestamp:"2026-11-11T12:00:00Z",activity:{type:"changed",activity:{type:"vulnerability_scan"}}}),lastEventId:""} as MessageEvent);
const environmentFrame=(f:ReturnType<typeof securityFixture>)=>f.source.onmessage?.({data:JSON.stringify({channel:"environments",timestamp:"2026-11-11T12:00:00Z",environment:{environments:["first","second"].map(id=>({id,name:id,apiUrl:"http://agent.example.com",enabled:true,status:f.state.hostStatuses[id]??"online",isEdge:true,registrationRevision:1}))}}),lastEventId:""} as MessageEvent);

it.each(["reopen","host-command","host-check"] as const)("policy versions: %s invalidation fences both directions and response orders",async trigger=>{
 for(const status of ["online","offline"])for(const newestFirst of [false,true]){
  vi.useFakeTimers();const f=recoveryFixture();const base=f.adapter;
  await base.getCurrentUser();const off=base.subscribeChanges(()=>{});
  await base.listImages(undefined,undefined,"background");await tick(10_001);
  f.state.edge=status==="online"?"offline":"online";f.state.hold.add("/api/environments");
  const old=base.listHosts();await tick(50);expect(f.pending.filter(p=>p.path==="/api/environments")).toHaveLength(1);
  if(trigger==="reopen")f.source.onopen?.({} as Event);
  else if(trigger==="host-command")await base.updateHost("first",{name:"renamed",color:null});
  else await base.reconnectHost!("second");
  f.state.edge=status;
  const current=base.listHosts();await tick(50);const held=f.pending.filter(p=>p.path==="/api/environments");expect(held).toHaveLength(2);
  held[newestFirst?1:0]!.release();await tick(20);held[newestFirst?0:1]!.release();
  expect((await current).find(h=>h.id==="second")?.status).toBe(status);
  expect((await old).find(h=>h.id==="second")?.status).toBe(status==="online"?"offline":"online");
  f.state.hold.clear();f.requests.length=0;await base.listImages(undefined,undefined,"background");off();
  expect(f.requests.filter(p=>p.path.endsWith("second/images"))).toHaveLength(status==="offline"?0:1);
 }
});

it.each(["security","active"] as const)("Security %s preserves post-boundary supersession and fresh publication",async kind=>{
 vi.useFakeTimers();const f=securityFixture();await f.adapter.getCurrentUser();const q=mountSecurity(kind,f);await tick(100);
 f.source.onopen?.({} as Event);environmentFrame(f);await tick(100);
 const original=f.fetcher.getMockImplementation()!;let release!:()=>void;let entered=false,hold=true;
 const gate=new Promise<void>(resolve=>{release=resolve;});const suffix=kind==="security"?"/second/vulnerabilities/all":"/second/images/vulnerabilities/active-summaries";
 f.fetcher.mockImplementation(async(input,init)=>{const response=await original(input,init);if(hold&&new URL(String(input),"https://hub.example.com").pathname.endsWith(suffix)){entered=true;await gate;}return response;});
 f.fetcher.mockClear();scanFrame(f);await tick(100);expect(entered).toBe(true);
 f.state.findingCount=1;scanFrame(f);await tick(50);hold=false;release();await tick(200);
 expect(q.client.getQueryState(q.key)?.fetchStatus).toBe("idle");expect(q.client.getQueryState(q.key)?.status).toBe("success");
 expect(f.fetcher.mock.calls.filter(([input])=>new URL(String(input),"https://hub.example.com").pathname.endsWith(suffix))).toHaveLength(2);
 const value=q.client.getQueryData<{images:Array<{hostId:string,counts:{critical:number}}>,findings?:unknown[]}>(q.key)!;
 expect(value.images.find(i=>i.hostId==="second")?.counts.critical).toBe(0);
});

it.each(["security","active"] as const)("Security %s disabled response does not absorb an enable notification",async kind=>{
 vi.useFakeTimers();const f=securityFixture();f.state.settings.featureVulnerabilityManagementEnabled="false";await f.adapter.getCurrentUser();const q=mountSecurity(kind,f);await tick(100);
 const original=f.fetcher.getMockImplementation()!;let release!:()=>void;let entered=false,hold=true;const gate=new Promise<void>(resolve=>{release=resolve;});
 f.fetcher.mockImplementation(async(input,init)=>{const response=await original(input,init);if(hold&&String(input).endsWith("/settings/public")){entered=true;await gate;}return response;});
 scanFrame(f);await tick(50);expect(entered).toBe(true);await f.adapter.setScanEnabled(true);hold=false;release();await tick(200);
 expect(q.client.getQueryState(q.key)?.fetchStatus).toBe("idle");expect(q.client.getQueryData<{enabled:boolean}>(q.key)?.enabled).toBe(true);
});

it.each(["security","active"] as const)("Security %s enabled response does not absorb a disable notification",async kind=>{
 vi.useFakeTimers();const f=securityFixture();await f.adapter.getCurrentUser();const q=mountSecurity(kind,f);await tick(100);
 const original=f.fetcher.getMockImplementation()!;let release!:()=>void;let entered=false,hold=true;const gate=new Promise<void>(resolve=>{release=resolve;});
 f.fetcher.mockImplementation(async(input,init)=>{const response=await original(input,init);if(hold&&String(input).endsWith("/settings/public")){entered=true;await gate;}return response;});
 scanFrame(f);await tick(50);expect(entered).toBe(true);await f.adapter.saveScanSettings!({...await f.adapter.getScanSettings!(),enabled:false});expect(f.state.settings.featureVulnerabilityManagementEnabled).toBe("false");hold=false;release();await tick(200);
 expect(q.client.getQueryState(q.key)?.fetchStatus).toBe("idle");expect(q.client.getQueryData<{enabled:boolean}>(q.key)?.enabled).toBe(false);
});

it.each([["security","settings"],["active","settings"],["security","host"],["active","host"]] as const)("Security %s identity cancellation at %s retires a held warm fetch without a tail",async (kind,stage)=>{
 vi.useFakeTimers();const f=securityFixture();await f.adapter.getCurrentUser();const q=mountSecurity(kind,f);await tick(100);
 const original=f.fetcher.getMockImplementation()!;let release!:()=>void;let entered=false,hold=true,revoke=false;const gate=new Promise<void>(resolve=>{release=resolve;});
 f.fetcher.mockImplementation(async(input,init)=>{if(revoke&&String(input).endsWith("/auth/me"))return Response.json({code:"auth.unauthorized"},{status:401});const response=await original(input,init);const path=new URL(String(input),"https://hub.example.com").pathname;const suffix=stage==="settings"?"/settings/public":kind==="security"?"/second/vulnerabilities/all":"/second/images/vulnerabilities/active-summaries";if(hold&&path.endsWith(suffix)){entered=true;await gate;}return response;});
 scanFrame(f);await tick(50);expect(entered).toBe(true);scanFrame(f);revoke=true;await f.adapter.getCurrentUser().catch(()=>undefined);await tick(20);hold=false;release();await tick(200);
 expect(q.client.getQueryState(q.key)?.fetchStatus).toBe("idle");expect(q.client.getQueryData(q.key)).toBeUndefined();
});

it("system components recovery audit: counts the fresh hint fan-out after delayed prerequisites",async()=>{
 vi.useFakeTimers();const f=recoveryFixture();await f.adapter.getCurrentUser();let client!:QueryClient;
 function Reader(){client=useQueryClient();useSystemComponents();return null;}
 render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Reader/></IntlProvider></AdapterProvider>);
 await tick(100);f.source.onopen?.({} as Event);f.send();await tick(100);
 expect(client.getQueryState(["system-components"])?.status).toBe("success");
 f.requests.length=0;f.state.hold.add("/api/capabilities");f.state.edge="offline";f.send();await tick(50);
 expect(f.pending.some(p=>p.path==="/api/capabilities")).toBe(true);
 f.state.edge="online";f.send();await tick(50);
 expect(f.requests.filter(p=>p.path.endsWith("second/updater/system-image-hints"))).toHaveLength(0);
 f.release();await tick(200);
 expect(client.getQueryState(["system-components"])?.fetchStatus).toBe("idle");
 expect(f.requests.filter(p=>p.path.endsWith("second/updater/system-image-hints"))).toHaveLength(1);
});

it.each(["security","active"] as const)("Security %s offline rows recover and clear unavailable flags",async kind=>{
 vi.useFakeTimers();const f=securityFixture();f.inventory.state.isEdge=true;await f.adapter.getCurrentUser();const q=mountSecurity(kind,f);await tick(100);
 f.source.onopen?.({} as Event);environmentFrame(f);await tick(100);
 f.state.hostStatuses.second="offline";environmentFrame(f);await tick(100);
 const offline=q.client.getQueryData<{images:Array<{hostId:string,unavailable?:boolean}>,unavailable:Array<{hostId:string}>}>(q.key)!;
 expect(offline.unavailable.some(h=>h.hostId==="second")).toBe(true);
 expect(offline.images.some(i=>i.hostId==="second"&&i.unavailable)).toBe(true);
 f.state.hostStatuses.second="online";environmentFrame(f);await tick(100);
 const online=q.client.getQueryData<typeof offline>(q.key)!;
 expect(online.unavailable.some(h=>h.hostId==="second")).toBe(false);
 expect(online.images.some(i=>i.hostId==="second")).toBe(true);
 expect(online.images.some(i=>i.hostId==="second"&&i.unavailable)).toBe(false);
 const settled=f.fetcher.mock.calls.length;await tick(31_000);expect(f.fetcher.mock.calls.length).toBe(settled);
});
