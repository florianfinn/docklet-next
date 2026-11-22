// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original v20 decoder boundary and bilingual vocabulary proofs.

import {readFileSync} from "node:fs";
import {describe,expect,it} from "vitest";
import {decodeProxyOperation,decodeCertificateObservation,proxyOutcome} from "./proxy-decoder";
import {decodeProxyData} from "./proxy-read-decoder";
import {PROXY_REASON_KEYS,PROXY_LAYOUT_REASON_KEYS,PROXY_STATE_KEYS,PROXY_PHASE_KEYS} from "../wire-catalogue-proxy";
import {renderMessage} from "../message-code";

const legacy=JSON.parse(readFileSync("../types/proxy/testdata/outcomes.json","utf8"))[0].body as {success:true;data:Record<string,unknown>};
const certificate={observedAt:"2026-11-20T00:00:00Z",instanceId:"example-instance",epoch:{containerId:"example-container",startedAt:"2026-11-19T00:00:00Z"},route:{name:"app",host:"app.example.test",sni:"tls.example.test",entryPoint:"websecure"},peer:{subject:"CN=app.example.test",issuer:"CN=example issuer",sans:["app.example.test"],notBefore:"2026-11-19T00:00:00Z",notAfter:"2026-12-19T00:00:00Z",sha256:"a".repeat(64)},chainTrusted:false,hostnameMatch:false,validity:"expired"};
const source={kind:"project",canonical:"/example/projects/proxy",mountTarget:"/etc/traefik",readOnly:false};
const discovery=(value:unknown)=>({success:true,data:[{identity:{containerId:"example-container",containerName:"proxy",startedAt:"2026-11-19T00:00:00Z"},kind:"traefik",image:"example-image",sources:[value]}]});
const config={name:"example",email:"admin@example.test",ca:{kind:"custom",url:"https://ca.example.test/directory"},storage:"/certs/acme.json",challenge:{kind:"dns",provider:"httpreq",delayBeforeCheck:0}};
const resolver={...legacy.data,kind:"resolver",state:"pending",phase:"writing",target:"",resolver:{intent:{...config,variables:[{name:"HTTPREQ_ENDPOINT",mode:"new"}]},phase:"writing",files:[{path:".env",kind:"env",old:{present:false,hash:""},new:{present:true,hash:"a".repeat(64)}}],recreateCommitted:false}};

describe("M28 wire boundaries",()=>{
 it("accepts v19 data without eligibility, certificate or kind",()=>{
  const old={...legacy.data};delete old.kind;
  expect(decodeProxyOperation({success:true,data:old}).state).toBe(old.state);
  expect(decodeProxyData("discovery",discovery(source))[0]?.sources?.[0]?.eligibility).toBeUndefined();
 });
 it("validates the actual source read entry point",()=>{
  const current={...source,mountMode:"ro",projectPath:"proxy/config",eligibility:{selectable:true,routeDirectory:{value:"routes",fixed:true},rootFile:"Caddyfile"}};
  expect(decodeProxyData("discovery",discovery(current))[0]?.sources?.[0]?.eligibility?.selectable).toBe(true);
  for(const patch of [{mountMode:"invalid"},{projectPath:false},{eligibility:{selectable:"yes"}},{eligibility:{selectable:true,reason:"future"}},{eligibility:{selectable:true,routeDirectory:{value:"routes",fixed:"true"}}}]) expect(()=>decodeProxyData("discovery",discovery({...current,...patch}))).toThrow("proxy.invalid-response");
 });
 it("requires complete epoch, peer and verification data without promoting certificate claims",()=>{
  expect(decodeCertificateObservation({success:true,data:certificate}).chainTrusted).toBe(false);
  for(const patch of [{epoch:{...certificate.epoch,startedAt:7}},{epoch:{startedAt:certificate.epoch.startedAt}},{chainTrusted:undefined},{hostnameMatch:"false"},{validity:"unknown"},{peer:{...certificate.peer,sha256:"bad"}},{peer:{...certificate.peer,sans:Array(33).fill("app.example.test")}}]) expect(()=>decodeCertificateObservation({success:true,data:{...certificate,...patch}})).toThrow("proxy.invalid-response");
  const operation=decodeProxyOperation({success:true,data:{...legacy.data,state:"applied-unverified",certificate,probe:{client:"helper",imageId:"sha256:example"}}});
  expect(proxyOutcome(operation).tone).toBe("warning");
  expect(operation.certificate?.hostnameMatch).toBe(false);
  expect(()=>decodeProxyOperation({success:true,data:{...legacy.data,probe:{client:"helper"}}})).toThrow();
 });
 it("decodes resolver receipts and rejects secret-bearing payload shapes",()=>{
  expect(decodeProxyOperation({success:true,data:resolver}).resolver?.files?.[0]?.kind).toBe("env");
  for(const patch of [
   {intent:{...resolver.resolver.intent,variables:[{name:"HTTPREQ_ENDPOINT",mode:"new",value:"example-live-sentinel"}]}},
   {files:[{...resolver.resolver.files[0],content:"example-live-sentinel"}]},
   {diagnostic:{code:"compose-up-failed",detail:"example-live-sentinel"}},
   {intent:{...resolver.resolver.intent,challenge:{...config.challenge,provider:"unlisted"}}},
   {replacement:{containerId:false}},
  ]) expect(()=>decodeProxyOperation({success:true,data:{...resolver,resolver:{...resolver.resolver,...patch}}})).toThrow("proxy.invalid-response");
 });
 it("keeps route and resolver states disjoint",()=>{
  for(const state of ["applied","rolled-back","refused"]){
   expect(()=>decodeProxyOperation({success:true,data:{...legacy.data,kind:"route",state}})).toThrow("proxy.invalid-response");
   expect(decodeProxyOperation({success:true,data:{...resolver,state}}).state).toBe(state);
   const absent:Record<string,unknown>={...legacy.data,state};delete absent.kind;expect(()=>decodeProxyOperation({success:true,data:absent})).toThrow("proxy.invalid-response");
  }
  for(const state of ["verified","applied-unverified","rejected","restored"]) expect(()=>decodeProxyOperation({success:true,data:{...resolver,state}})).toThrow("proxy.invalid-response");
  expect(decodeProxyOperation({success:true,data:{...legacy.data,kind:"future-kind",state:"recovery-required",reason:"unknown-operation-kind",target:""}}).state).toBe("recovery-required");
  expect(()=>decodeProxyOperation({success:true,data:{...legacy.data,kind:"future-kind",state:"verified",reason:"unknown-operation-kind"}})).toThrow();
 });
 it("covers every v20 code and phase with actual EN/DE catalogue text",()=>{
  const fixture=JSON.parse(readFileSync("../types/contract/testdata/agent-hub-v21.json","utf8"));
  expect(Object.keys(PROXY_REASON_KEYS).sort()).toEqual([...fixture.proxy.reasons].sort());
  expect(Object.keys(PROXY_STATE_KEYS).sort()).toEqual([...fixture.proxy.states].sort());
  expect(Object.keys(PROXY_PHASE_KEYS).sort()).toEqual([...fixture.proxy.phases].sort());
  for(const map of [PROXY_REASON_KEYS,PROXY_LAYOUT_REASON_KEYS,PROXY_STATE_KEYS,PROXY_PHASE_KEYS]) for(const [code,key] of Object.entries(map)) for(const locale of ["en-US","de"] as const){
   const text=renderMessage({code:key,params:{}},locale);expect(text).not.toBe(code);expect(text).not.toBe(key);expect(text.length).toBeGreaterThan(0);
  }
 });
});
