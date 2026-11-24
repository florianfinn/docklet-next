// Source: docklet-next/issues/592
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original HTTP adoption retry, fallback, cancellation and regain proofs.

import {act, fireEvent, render, screen, waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach, expect, it, vi} from "vitest";
import ProxyPage from "./ProxyPage";
import {fixture, instance, Providers} from "./proxy.test-support";
import type {Adoption,Instance} from "./types";

const replacement:Instance={...instance,identity:{...instance.identity,containerId:"container-two",startedAt:"2026-11-24T00:00:00Z"}};
const json=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{headers:{"Content-Type":"application/json","X-CSRF-Token":"example"}});
function setup(answer:Adoption) {
 const f=fixture(),raw=f.fetcher.getMockImplementation()!;
 let failed=true;
 const adoptions:string[]=[],reads:Instance[]=[],inventories:Instance[]=[];
 f.fetcher.mockImplementation(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/adopt")){adoptions.push(path);if(answer.outcome!=="refused")failed=false;return json(answer)}
  if(path.endsWith("/read")||path.endsWith("/inventory")) {
   const i=JSON.parse(String(init?.body)) as Instance;
   (path.endsWith("/read")?reads:inventories).push(i);
   if(failed && i.id===instance.id)return new Response(JSON.stringify({code:"container-changed"}),{status:409});
  }
  return raw(input,init);
 });
 vi.stubGlobal("fetch",f.fetcher);
 return {...f,adoptions,reads,inventories};
}
afterEach(()=>{vi.unstubAllGlobals();vi.useRealTimers()});
it.each(["read","inventory"])("replaces the instance and retries both reads once after %s refuses",async(entry)=>{
 const f=setup({outcome:"adopted",instance:replacement,operationId:"adoption-one"});
 const raw=f.fetcher.getMockImplementation()!;
 f.fetcher.mockImplementation((input,init)=>String(input).endsWith(entry==="read"?"/inventory":"/read") && JSON.parse(String(init?.body)).identity.containerId===instance.identity.containerId ? Promise.resolve(json(entry==="read"?f.state.inventory:f.state.read)) : raw(input,init));
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByRole("tab",{name:"Routes"});
 expect(f.adoptions).toHaveLength(1);
 expect(f.reads.at(-1)?.identity.containerId).toBe(replacement.identity.containerId);
 expect(f.inventories.at(-1)?.identity.containerId).toBe(replacement.identity.containerId);
});
it("uses current response to retry the same instance only once",async()=>{
 const f=setup({outcome:"current",instance});
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByRole("tab",{name:"Routes"});
 expect(f.adoptions).toHaveLength(1);expect(f.reads).toHaveLength(2);expect(f.inventories).toHaveLength(2);
});
it.each([true,false])("skips a stale first instance, marks it and keeps removal available (superseded=%s)",async(superseded)=>{
 const usable={...replacement,id:"instance-two",name:"Usable proxy"};
 const f=setup({outcome:"refused",reason:superseded?"held-by-instance":"source-changed",instance,...(superseded?{supersededBy:usable.id}:{})});
 f.state.instances=[instance,usable];
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByRole("tab",{name:"Routes"});
 expect(f.reads.at(-1)?.id).toBe(usable.id);
 const picker=screen.getByRole("combobox",{name:"Configured instances"});picker.focus();await userEvent.keyboard("{ArrowDown}");
 const stale=await screen.findByRole("option",{name:/container-on.*Stale/});await userEvent.click(stale);
 await screen.findByRole("button",{name:"Remove instance"});
 expect(screen.getByText("Stale")!==null).toBe(true);
 expect(f.adoptions).toHaveLength(1);
});
it("tries every refused instance at most once without looping",async()=>{
 const f=setup({outcome:"refused",reason:"source-changed",instance});
 const second={...instance,id:"instance-two"};f.state.instances=[instance,second];
 const raw=f.fetcher.getMockImplementation()!;
 f.fetcher.mockImplementation(async(input,init)=>{
  if(String(input).endsWith("/adopt")&&String(input).includes(second.id)) {f.adoptions.push(String(input));return json({outcome:"refused",reason:"operation-open",instance:second})}
  if((String(input).endsWith("/read")||String(input).endsWith("/inventory"))&&JSON.parse(String(init?.body)).id===second.id)return new Response(JSON.stringify({code:"container-changed"}),{status:409});
  return raw(input,init);
 });
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByText("An operation is still open for this instance.");
 await screen.findByRole("button",{name:"Remove instance"});
 expect(f.adoptions).toHaveLength(2);
});
it("aborts a pending adoption on unmount and ignores its late answer",async()=>{
 const f=setup({outcome:"adopted",instance:replacement,operationId:"adoption-one"});
 const raw=f.fetcher.getMockImplementation()!;
 let resolve!: (r:Response)=>void,signal:AbortSignal|undefined;
 f.fetcher.mockImplementation((input,init)=>{
  if(String(input).endsWith("/adopt")){signal=init?.signal as AbortSignal;return new Promise(r=>{resolve=r})}
  return raw(input,init);
 });
 const view=render(<Providers><ProxyPage/></Providers>);
 await waitFor(()=>expect(signal!==undefined).toBe(true));view.unmount();expect(signal?.aborted).toBe(true);
 await act(async()=>resolve(json({outcome:"adopted",instance:replacement,operationId:"adoption-one"})));
 expect(f.reads.some(i=>i.identity.containerId===replacement.identity.containerId)).toBe(false);
});
it("refreshes on focus and visibility regain once after the two-second debounce, with no polling",async()=>{
 const f=fixture();vi.stubGlobal("fetch",f.fetcher);
 render(<Providers><ProxyPage/></Providers>);await screen.findByRole("tab",{name:"Routes"});
 const count=()=>f.fetcher.mock.calls.filter(([u])=>String(u).endsWith("/read")).length;
 const initial=count();vi.useFakeTimers();
 fireEvent.focus(window);fireEvent(document,new Event("visibilitychange"));
 await act(async()=>{await vi.advanceTimersByTimeAsync(1999)});expect(count()).toBe(initial);
 await act(async()=>{await vi.advanceTimersByTimeAsync(1)});expect(count()).toBe(initial+1);
 await act(async()=>{await vi.advanceTimersByTimeAsync(6000)});expect(count()).toBe(initial+1);
});
it("cancels adoption when the selected instance changes and ignores its late answer",async()=>{
 const f=setup({outcome:"adopted",instance:replacement,operationId:"adoption-one"});
 const usable={...replacement,id:"instance-two",name:"Usable proxy"};f.state.instances=[instance,usable];
 const raw=f.fetcher.getMockImplementation()!;
 let resolve!: (r:Response)=>void,signal:AbortSignal|undefined;
 f.fetcher.mockImplementation((input,init)=>{
  if(String(input).endsWith("/adopt")){signal=init?.signal as AbortSignal;return new Promise(r=>{resolve=r})}
  return raw(input,init);
 });
 render(<Providers><ProxyPage/></Providers>);
 await waitFor(()=>expect(signal!==undefined).toBe(true));
 screen.getByRole("combobox",{name:"Configured instances"}).focus();await userEvent.keyboard("{ArrowDown}");
 await userEvent.click(await screen.findByRole("option",{name:/Usable proxy/}));
 await screen.findByRole("tab",{name:"Routes"});expect(signal?.aborted).toBe(true);
 const reads=f.reads.length;
 await act(async()=>resolve(json({outcome:"adopted",instance:replacement,operationId:"adoption-one"})));
 expect(f.reads.length).toBe(reads);expect(f.reads.at(-1)?.id).toBe(usable.id);
});
it("adopts a recreation on the existing Refresh gesture",async()=>{
 const f=fixture(),raw=f.fetcher.getMockImplementation()!;
 let changed=false,adoptions=0;
 f.fetcher.mockImplementation(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/adopt")){adoptions++;return json({outcome:"adopted",instance:replacement,operationId:"adoption-one"})}
  if(changed&&(path.endsWith("/read")||path.endsWith("/inventory"))&&JSON.parse(String(init?.body)).identity.containerId===instance.identity.containerId)return new Response(JSON.stringify({code:"container-changed"}),{status:409});
  return raw(input,init);
 });
 vi.stubGlobal("fetch",f.fetcher);render(<Providers><ProxyPage/></Providers>);
 await screen.findByRole("tab",{name:"Routes"});changed=true;
 await userEvent.click(screen.getByRole("button",{name:"Refresh"}));
 await waitFor(()=>expect(adoptions).toBe(1));
 await screen.findByRole("tab",{name:"Routes"});
 expect(f.fetcher.mock.calls.some(([url,init])=>String(url).endsWith("/read")&&JSON.parse(String(init?.body)).identity.containerId===replacement.identity.containerId)).toBe(true);
});
// S2-R1-2: each read may adopt once, so a refusal clears on the next read and a later recreation is followed.
function sequenced(answers:Adoption[],initiallyChanged=true) {
 const f=fixture(),raw=f.fetcher.getMockImplementation()!;
 let current=instance.identity.containerId,changed=initiallyChanged;const adoptions:Adoption[]=[];
 f.fetcher.mockImplementation(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/adopt")){const answer=answers[adoptions.length]??answers.at(-1)!;adoptions.push(answer);if(answer.outcome!=="refused"){current=answer.instance.identity.containerId;changed=false;}return json(answer)}
  if((path.endsWith("/read")||path.endsWith("/inventory"))&&changed&&JSON.parse(String(init?.body)).identity.containerId===current)return new Response(JSON.stringify({code:"container-changed"}),{status:409});
  return raw(input,init);
 });
 vi.stubGlobal("fetch",f.fetcher);
 return {...f,adoptions,recreate:()=>{changed=true}};
}
it("adopts on Refresh after an earlier operation-open refusal and clears the stale mark",async()=>{
 const f=sequenced([{outcome:"refused",reason:"operation-open",instance},{outcome:"adopted",instance:replacement,operationId:"adoption-one"}]);
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByText("An operation is still open for this instance.");
 expect(f.adoptions).toHaveLength(1);expect(screen.queryByText("Stale")!==null).toBe(true);
 await userEvent.click(screen.getByRole("button",{name:"Refresh"}));
 await waitFor(()=>expect(f.adoptions).toHaveLength(2));
 await screen.findByRole("tab",{name:"Routes"});
 expect(screen.queryByText("Stale")===null).toBe(true);
});
it("follows a second recreation in the same page session on the next read",async()=>{
 const third:Instance={...instance,identity:{...instance.identity,containerId:"container-three",startedAt:"2026-11-24T01:00:00Z"}};
 const f=sequenced([{outcome:"adopted",instance:replacement,operationId:"adoption-one"},{outcome:"adopted",instance:third,operationId:"adoption-two"}]);
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByRole("tab",{name:"Routes"});expect(f.adoptions).toHaveLength(1);
 f.recreate();
 await userEvent.click(screen.getByRole("button",{name:"Refresh"}));
 await waitFor(()=>expect(f.adoptions).toHaveLength(2));
 await screen.findByRole("tab",{name:"Routes"});
 expect(f.fetcher.mock.calls.some(([url,init])=>String(url).endsWith("/read")&&JSON.parse(String(init?.body)).identity.containerId===third.identity.containerId)).toBe(true);
});
// S2-R1-3: only opening switches the view; a refusal on a later read keeps the instance and marks it stale.
it("keeps the open instance when a later read's adoption is refused",async()=>{
 const f=sequenced([{outcome:"refused",reason:"source-changed",instance}],false);
 const second={...replacement,id:"instance-two",name:"Second proxy"};f.state.instances=[instance,second];
 render(<Providers><ProxyPage/></Providers>);
 await screen.findByRole("tab",{name:"Routes"});
 f.recreate();
 await userEvent.click(screen.getByRole("button",{name:"Refresh"}));
 await waitFor(()=>expect(f.adoptions).toHaveLength(1));
 await waitFor(()=>expect(screen.queryByRole("button",{name:"Remove instance"})!==null).toBe(true));
 expect(screen.queryByText("Stale")!==null).toBe(true);
 const lastRead=f.fetcher.mock.calls.filter(([url])=>String(url).endsWith("/read")).at(-1);
 expect(JSON.parse(String(lastRead?.[1]?.body)).id).toBe(instance.id);
});
