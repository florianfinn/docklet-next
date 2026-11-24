// Source: docklet-next/issues/595
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original visible container and stack check route/result proofs.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { toast } from "sonner";
import { ContainerUpdates } from "@/features/containers/ContainerUpdates";
import { StackHeader } from "@/features/stacks/detail/StackHeader";
import { renderStackDetail } from "@/data/message-code";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { observation } from "@/data/http/update-observation.test-support";
import { envelope } from "@/data/http/runtime-commands.test-support";
import { deferred } from "@/data/http/activity-reader.test-support";
import { renderWithApp } from "@/test/render";

it.each([false,true])("shows a standalone container check before any update is known, then feedback (update=%s)",async(hasUpdate)=>{
 const f=updateFixture();await f.adapter.getCurrentUser();
 const demo=createDemoAdapter();const c=(await demo.listContainers()).find(row=>!row.stackId && row.origin==="standalone") ?? (await demo.listContainers()).find(row=>!row.stackId)!;
 c.id="direct~example-container";c.hostId="direct";c.origin="standalone";c.originSource="manual";c.redeployDisabled=false;c.update={...c.update,state:"current"} as typeof c.update;
 const pending=deferred<Response>();
 f.state.hold=(path)=>path.endsWith("/containers/example-container/check")?pending.promise:undefined;
 const success=vi.spyOn(toast,"success").mockImplementation(()=>"example");
 renderWithApp(<ContainerUpdates c={c} select={()=>{}} />,f.adapter);
 const button=await screen.findByRole("button",{name:"Nach Updates suchen"});await waitFor(()=>expect(button.hasAttribute("disabled")).toBe(false));await userEvent.click(button);
 expect(screen.getByRole("button",{name:"Updates werden geprüft…"}).getAttribute("aria-busy")).toBe("true");
 await act(async()=>pending.resolve(envelope(observation(hasUpdate?"version":"current"))));
 await waitFor(()=>expect(success).toHaveBeenCalled());expect(String(success.mock.calls[0]?.[0])).toContain(hasUpdate?"Update verfügbar:":"Kein Update");
 expect(f.writes().some(call=>call.path==="/api/environments/direct/updater/containers/example-container/check")).toBe(true);success.mockRestore();
});
it("checks a stack from a visible header action and uses the returned update result",async()=>{
 const f=updateFixture();await f.adapter.getCurrentUser();const demo=createDemoAdapter();const first=(await demo.listStacks())[0]!;const stack=renderStackDetail((await demo.getStack(first.id))!, "de");stack.id="direct~project";stack.hostId="direct";
 const success=vi.spyOn(toast,"success").mockImplementation(()=>"example");
 renderWithApp(<StackHeader stack={stack} host={undefined} />,f.adapter);
 const button=await screen.findByRole("button",{name:"Nach Updates suchen"});await waitFor(()=>expect(button.hasAttribute("disabled")).toBe(false));await userEvent.click(button);
 await waitFor(()=>expect(success).toHaveBeenCalled());expect(String(success.mock.calls[0]?.[0])).toContain("Update verfügbar:");expect(f.writes().filter(call=>call.path.endsWith("/projects/project/check"))).toHaveLength(1);success.mockRestore();
});
