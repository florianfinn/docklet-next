// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original M8/M10 module import and delayed-read ownership proofs.

import { lazy, Suspense } from "react";
import { act, screen, waitFor, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { IntlProvider } from "use-intl";
import { Link } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { ModuleRoute } from "@/app/ModuleRoute";
import { catalogs } from "@/app/i18n/catalog";
import { visibleNavItems, NAV } from "@/app/shell/nav-items";
import { createDemoAdapter } from "./demo/demo-adapter";
import { renderRoutes } from "@/test/render";
import { AdapterContext } from "./use-adapter";
import { registerIdentityLifetime } from "./identity-lifetime";
import { useModuleRead } from "./use-module";

afterEach(() => localStorage.clear());
it("M8 never imports a disabled module on direct URL or navigation", async () => {
 const adapter=createDemoAdapter(),load=vi.fn(async()=>({default:()=> <p>Module surface</p>}));
 const Page=lazy(load);
 const states=vi.spyOn(adapter,"getModuleStates");
 const {router}=renderRoutes([{path:"/proxy",element:<Suspense><ModuleRoute id="proxy" page={Page}/></Suspense>},{path:"/",element:<Link to="/proxy">Open module</Link>}],["/proxy"],adapter);
 await waitFor(()=>expect(states).toHaveBeenCalledTimes(1));
 await act(async()=>{await states.mock.results[0]?.value;});
 expect(load).toHaveBeenCalledTimes(0);
 await screen.findByText("Seite nicht gefunden");
 await act(()=>router.navigate("/"));await userEvent.click(screen.getByRole("link",{name:"Open module"}));
 await screen.findByText("Seite nicht gefunden");expect(load).toHaveBeenCalledTimes(0);
 const nav=NAV.flatMap((g)=>g.items);
 expect(visibleNavItems(nav,true,{proxy:false}).some((m)=>m.key==="proxy")).toBe(false);
 expect(visibleNavItems(nav,true,{proxy:true}).some((m)=>m.key==="proxy")).toBe(true);
});

it.each(["off","identity"] as const)("M10 discards a held module read after %s changes",async(change)=>{
 const adapter=createDemoAdapter();let version=0;const listeners=new Set<()=>void>();
 registerIdentityLifetime(adapter.subscribeChanges,{version:()=>version,active:()=>true,listen:(fn)=>{listeners.add(fn);return()=>{listeners.delete(fn);};}});
 adapter.getModuleStates=async()=>({proxy:true});
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
 let release!:(value:string)=>void;let nextRelease!:(value:string)=>void;
 const read=vi.fn(()=>new Promise<string>((resolve)=>{if(read.mock.calls.length===1)release=resolve;else nextRelease=resolve;}));
 function Consumer(){const value=useModuleRead("proxy",["read"],read);const cache=useQueryClient();return <><p>{value.data??"No module result"}</p><button onClick={()=>cache.setQueryData(["module-states",version],{proxy:false})}>Off</button></>;}
 render(<QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><IntlProvider locale="de" messages={catalogs.de}><Consumer/></IntlProvider></AdapterContext.Provider></QueryClientProvider>);
 await waitFor(()=>expect(read).toHaveBeenCalledTimes(1));
 if(change==="off")await userEvent.click(screen.getByRole("button",{name:"Off"}));
 else await act(()=>{version++;listeners.forEach((fn)=>fn());});
 await act(async()=>{release("Old module result");await Promise.resolve();});
 expect(screen.queryByText("Old module result")!==null).toBe(false);
 expect(client.getQueryData(["module","proxy",version,"read"])).toBeUndefined();
 if(change==="identity"){await waitFor(()=>expect(read).toHaveBeenCalledTimes(2));await act(async()=>{nextRelease("Current module result");});await screen.findByText("Current module result");}
 client.clear();
});
