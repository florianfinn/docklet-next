// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original module toggle and deletion confirmation gestures.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { lazy, Suspense } from "react";
import { ModuleRoute } from "@/app/ModuleRoute";
import { renderRoutes } from "@/test/render";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { ModuleProblemError } from "@/data/module-error";
import SettingsPage from "./SettingsPage";

afterEach(()=>localStorage.clear());
it("confirms disable, renders the named busy operation and preserves enabled state",async()=>{
 const adapter=createDemoAdapter();await adapter.setModuleEnabled!("proxy",true);
 adapter.setModuleEnabled=vi.fn(async()=>{throw new ModuleProblemError("module-busy",409,"apply-1");});
 renderRoutes([{path:"/settings/:section",element:<SettingsPage/>}],["/settings/modules"],adapter);
 const toggle=await screen.findByRole("switch",{name:"Proxy"});expect(toggle.getAttribute("aria-checked")).toBe("true");
 expect(screen.getByRole("button",{name:"Moduldaten löschen"}).hasAttribute("disabled")).toBe(true);
 await userEvent.click(toggle);expect(adapter.setModuleEnabled).toHaveBeenCalledTimes(0);
 const dialog=await screen.findByRole("alertdialog");await userEvent.click(within(dialog).getByRole("button",{name:"Modul ausschalten"}));
 expect(await within(dialog).findByText("Ein Vorgang ist noch nicht abgeschlossen: apply-1. Warte auf seinen Abschluss.")!==null).toBe(true);
 expect(toggle.getAttribute("aria-checked")).toBe("true");
});
it("confirms deletion while off and lists hosts pending cleanup",async()=>{
 const adapter=createDemoAdapter();adapter.deleteModuleData=vi.fn(async()=>({pendingCleanup:[{id:"remote",name:"Example host",reason:"agent.unavailable"}]}));
 renderRoutes([{path:"/settings/:section",element:<SettingsPage/>}],["/settings/modules"],adapter);
 await screen.findByRole("switch",{name:"Proxy"});await userEvent.click(screen.getByRole("button",{name:"Moduldaten löschen"}));
 expect(adapter.deleteModuleData).toHaveBeenCalledTimes(0);
 const dialog=await screen.findByRole("alertdialog");await userEvent.click(within(dialog).getByRole("button",{name:"Moduldaten löschen"}));
 expect(await screen.findByText("Example host")!==null).toBe(true);expect(adapter.deleteModuleData).toHaveBeenCalledWith("proxy");
});

it("removes a module surface after confirmed disable while the public refetch is held",async()=>{
 const adapter=createDemoAdapter();await adapter.setModuleEnabled!("proxy",true);
 const initial=adapter.getModuleStates!;let reads=0;let release!:(value:{proxy:boolean})=>void;
 adapter.getModuleStates=vi.fn(()=>++reads===1?initial():new Promise<{proxy:boolean}>((resolve)=>{release=resolve;}));
 const Page=lazy(async()=>({default:()=> <p>Enabled module surface</p>}));
 renderRoutes([{path:"/settings/:section",element:<><SettingsPage/><Suspense><ModuleRoute id="proxy" page={Page}/></Suspense></>}],["/settings/modules"],adapter);
 await screen.findByText("Enabled module surface");await userEvent.click(screen.getByRole("switch",{name:"Proxy"}));
 const dialog=await screen.findByRole("alertdialog");await userEvent.click(within(dialog).getByRole("button",{name:"Modul ausschalten"}));
 await waitFor(()=>expect(adapter.getModuleStates).toHaveBeenCalledTimes(2));
 expect(screen.queryByText("Enabled module surface")!==null).toBe(false);
 await act(async()=>{release({proxy:false});});
 await screen.findByText("Seite nicht gefunden");
});
