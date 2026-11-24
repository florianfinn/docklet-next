// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original typed eligibility consumer and old-agent capability gate proofs.

import { screen, waitFor } from "@testing-library/react";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { envelope } from "@/data/http/runtime-commands.test-support";
import { renderWithApp } from "@/test/render";
import { StackBindingNotice } from "./stack-binding-notice";

it("renders the typed service binding before any dialog or write",async()=>{
 const f=updateFixture();await f.adapter.getCurrentUser();
 const host=(await createDemoAdapter().listHosts())[0]!;
 host.id="direct";host.componentVersion={...host.componentVersion,agentContractVersion:23} as NonNullable<typeof host.componentVersion>;
 f.adapter.listHosts=async()=>[host];
 f.state.hold=path=>path.endsWith("/update-eligibility")?Promise.resolve(envelope({services:[{service:"app",eligible:false,binding:{scope:"service",service:"app",reason:"variable-reference",variable:"SERVICE_IMAGE"}}]})):undefined;
 renderWithApp(<StackBindingNotice stackId="direct~project" />,f.adapter);
 await screen.findByText(/SERVICE_IMAGE/);expect(document.body.textContent).toContain("app");expect(f.writes()).toHaveLength(0);
 expect(f.calls.filter(call=>call.path.endsWith("/update-eligibility"))).toHaveLength(1);
});
it("predicts no refusal for a v22 agent and sends no unsupported eligibility command",async()=>{
 const f=updateFixture();await f.adapter.getCurrentUser();const host=(await createDemoAdapter().listHosts())[0]!;
 host.id="direct";host.componentVersion={...host.componentVersion,agentContractVersion:22} as NonNullable<typeof host.componentVersion>;let hostsRead=false;f.adapter.listHosts=async()=>{hostsRead=true;return [host];};
 renderWithApp(<StackBindingNotice stackId="direct~project" />,f.adapter);
 await waitFor(()=>expect(hostsRead).toBe(true));
 await new Promise(resolve=>setTimeout(resolve,100));
 expect(document.body.textContent?.includes(catalogs.de.containers.updateCheck.generic)).toBe(false);
 expect(document.body.textContent?.includes(catalogs.de.containers.updateCheck.notOffered.split(":")[0]!)).toBe(false);
 expect(f.calls.filter(call=>call.path.endsWith("/update-eligibility"))).toHaveLength(0);expect(f.writes()).toHaveLength(0);
});
