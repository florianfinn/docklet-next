// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regressions for create/upload view identity with separator characters in paths (M16 review CB-1).

import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useLayoutEffect } from "react";
import { useLocation } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { revisionProducer } from "@/data/http/volume-revision.test-support";
import type { Volume } from "@/domain/volumes";
import { mockWideWorkspace } from "@/test/layout";
import { renderRoutes } from "@/test/render";
import { FileWorkspace } from "@/features/volumes/files/FileWorkspace";
const volume: Volume = { id: "edge~data", hostId: "edge", name: "data", createdAt: "2026-10-22T12:00:00Z", kind: "volume", sizeBytes: 0, consumers: [], source: "", labels: {} };
let client: ReturnType<typeof useQueryClient>;
let committed = "";
function Page() {
 const queryClient=useQueryClient(), location=useLocation();
 useLayoutEffect(()=>{client=queryClient; committed=location.search;});
 const fetching=useIsFetching({queryKey:["files"]});
 return <><output data-testid="fetching">{fetching}</output><FileWorkspace volume={volume}/></>;
}
beforeEach(()=>{simulationStore.set(s=>({...s,latency:"instant",live:false}));mockWideWorkspace();});
afterEach(()=>{simulationStore.reset();localStorage.clear();vi.restoreAllMocks();});
it.each([["create","ordinary"],["create","colon"],["upload","ordinary"],["upload","colon"]] as const)("%s respects a changed %s view",async (operation,scenario)=>{
 const producer=revisionProducer(true);
 producer.contents.set("c","content C");producer.contents.set("b:/c","other C");
 ["a:","a:/b","a","b:"].forEach(p=>producer.folders.add(p));
 let release!:()=>void;
 const gate=new Promise<void>(r=>release=r);
 const createEntry=vi.fn(async (...args:Parameters<typeof producer.workspace.createEntry>)=>{await gate;return producer.workspace.createEntry(...args);});
 const uploadFile=vi.fn(async (...args:Parameters<typeof producer.workspace.uploadFile>)=>{await gate;return producer.workspace.uploadFile(...args);});
 const adapter={...createDemoAdapter(),...producer.workspace,createEntry,uploadFile,kind:"http" as const,getCapability:async(operationId:string,resourceRef={kind:"hub" as const,id:"0"})=>({operationId,resourceRef,available:true})};
 const initial=new URLSearchParams({tab:"files",path:"/a:/b",file:"/c"});
 const f=renderRoutes([{path:"/volumes/:id",element:<Page/>}],["/volumes/edge~data?"+initial],adapter);
 await screen.findByRole("textbox",{name:"Inhalt von c"});
 if(operation==="create") {
 await userEvent.click(screen.getByRole("button",{name:"Neue Datei"}));
 await userEvent.type(within(await screen.findByRole("dialog")).getByRole("textbox",{name:"Name"}),"created.txt");
 await userEvent.click(screen.getByRole("button",{name:"Anlegen"}));
 } else {
 fireEvent.change(f.container.querySelector('input[type="file"]')!,{target:{files:[new File(["one"],"one.txt"),new File(["two"],"two.txt")]}});
 }
 const method=operation==="create"?createEntry:uploadFile;
 await waitFor(()=>expect(method.mock.calls.length).toBe(1));
 const next=new URLSearchParams({tab:"files",path:"/a",file:scenario==="colon"?"/b:/c":"/b.txt",sort:"name"});
 let navigated=false;
 const before=committed;
 const off=client.getQueryCache().subscribe(event=>{
  if(!navigated&&event.type==="updated"&&event.action.type==="success"&&event.query.queryKey[0]==="files"&&event.query.queryKey[2]!=="content"&&(event.query.state.data as {revision?:string})?.revision==="r2"){
   navigated=true;void f.router.navigate("/volumes/edge~data?"+next);
  }
 });
 try {
  await act(async()=>{
   release();
   await vi.waitFor(()=>expect(navigated&&method.mock.settledResults[0]?.type==="fulfilled").toBe(true));
   await new Promise(r=>setTimeout(r,0));
   expect([navigated,committed===before]).toEqual([true,true]);
  });
  await waitFor(()=>expect(screen.queryByRole("dialog")===null).toBe(true));
  await waitFor(()=>expect(screen.getByTestId("fetching").textContent).toBe("0"));
  if(operation==="create") expect(producer.contents.has("a:/b/created.txt")).toBe(true);
  else {
    expect(producer.contents.get("a:/b/one.txt")).toBe("one");
    expect({calls:uploadFile.mock.calls.length,secondWritten:producer.contents.has("a:/b/two.txt")}).toEqual({calls:1,secondWritten:false});
  }
  const final=new URLSearchParams(f.router.state.location.search);
  expect({path:final.get("path"),file:final.get("file"),sort:final.get("sort")}).toEqual({path:"/a",file:next.get("file"),sort:"name"});
 } finally {off();}
});
