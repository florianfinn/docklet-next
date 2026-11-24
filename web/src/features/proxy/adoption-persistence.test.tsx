// Source: docklet-next/issues/592
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Real persisted receipt decoded and rendered, with strict empty-digest guard proofs.

import {render,screen} from "@testing-library/react";
import {expect,it,vi} from "vitest";
import {decodeProxyOperation} from "@/data/http/proxy-decoder";
import {decodeProxyAdoption} from "@/data/http/proxy-read-decoder";
import {persistedAdoption} from "./adoption-fixture.test-support";
import {Operations} from "./Operations";
import {Providers,instance,operation} from "./proxy.test-support";

it.each(["en-US","de"] as const)("round-trips a real persisted adoption into Recent operations (%s)",locale=>{
 const o=decodeProxyOperation({success:true,data:persistedAdoption});
 expect(o.kind).toBe("adoption");expect(o.state).toBe("applied");expect(o.digest).toBe("");expect(o.finishedAt).toBeTruthy();
 render(<Providers locale={locale}><Operations rows={[o]} busy={false} release={vi.fn()}/></Providers>);
 expect(screen.getByText(locale==="de"?"Neu erstelltem Container gefolgt":"Followed recreated container")!==null).toBe(true);
 expect(screen.getByText(`${o.adoption!.previousContainerId} → ${o.adoption!.containerId}`)!==null).toBe(true);
});
it.each([
 {kind:"route"}, {kind:"resolver"}, {state:"pending"}, {phase:"admitted"}, {finishedAt:undefined}, {adoption:undefined},
 {adoption:{...persistedAdoption.adoption,containerId:""}},
 {adoption:{...persistedAdoption.adoption,previousContainerId:persistedAdoption.adoption.containerId}},
 {adoption:{...persistedAdoption.adoption,startedAt:"invalid"}},
])("rejects a malformed empty-digest adoption: %j",patch=>{
 expect(()=>decodeProxyOperation({success:true,data:{...persistedAdoption,...patch}})).toThrow("proxy.invalid-response");
});
it("keeps route and resolver digest validation",()=>{
 for(const kind of ["route","resolver"]){expect(()=>decodeProxyOperation({success:true,data:operation({kind,digest:""})})).toThrow()}
});
it.each([
 {outcome:"refused"}, {outcome:"refused",reason:"unknown"}, {outcome:"refused",reason:"held-by-instance"},
 {outcome:"adopted"}, {outcome:"current",operationId:"unexpected"}, {outcome:"current",reason:"source-changed"},
 {outcome:"refused",reason:"source-changed",supersededBy:"unexpected"},
])("rejects malformed adoption response %j",patch=>{
 expect(()=>decodeProxyAdoption({success:true,data:{instance,...patch}})).toThrow("proxy.invalid-response");
});
