// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original generated proxy v20 route, variable and schema parity checks.

import {readFileSync} from "node:fs";
import {createHash} from "node:crypto";
import test from "node:test";
import assert from "node:assert/strict";
import {agentFixture} from "../../scripts/agent-contract.mjs";
const spec=readFileSync(new URL("../../api/openapi.json",import.meta.url));
const raw=readFileSync(new URL("./testdata/agent-hub-v20.json",import.meta.url));
const document=JSON.parse(spec);
const manifest=JSON.parse(raw);
test("v20 is regenerated from the real Huma export and command registry",()=>{
 assert.deepEqual(agentFixture(spec,raw),raw);
 assert.equal(manifest.proxy.minimumContract,20);
 for(const path of ["/modules/proxy/instances/{instanceId}/certificate-check","/modules/proxy/instances/{instanceId}/resolvers","/environments/{id}/modules/proxy/certificate-check","/environments/{id}/modules/proxy/resolvers"]){
  const operation=document.paths[path]?.post;
  assert.ok(operation,path);
  assert.ok(operation.responses[501],path);
 }
 assert.equal(document.paths["/environments/{id}/modules/proxy/instances/{instanceId}/resolvers"],undefined);
 assert.deepEqual(manifest.proxy.dnsProviders,document["x-proxy-dns-providers"]);
 assert.equal(manifest.proxy.dnsProviders.length,8);
 for(const command of ["proxy.certificate.check","proxy.resolver.create","proxy.operation","proxy.fence","proxy.release"])assert.ok(manifest.proxy.edgeRoutes.some(row=>row.command===command),command);
 const fingerprint=createHash("sha256").update(raw).digest("hex");
 assert.ok(readFileSync(new URL("./agent.go",import.meta.url),"utf8").includes(`AgentHubFingerprint = "${fingerprint}"`));
});
test("resolver persistence schemas contain modes and hashes, never values or file bytes",()=>{
 const schemas=document.components.schemas;
 assert.deepEqual(Object.keys(schemas.ProxyResolverVariableIntent.properties).sort(),["mode","name"]);
 assert.ok(schemas.ProxyResolverVariable.properties.value);
 assert.equal(schemas.ProxyResolverFile.properties.content,undefined);
 assert.equal(schemas.ProxyResolverDiagnostic.properties.detail,undefined);
 assert.equal(schemas.ProxySource.required.includes("eligibility"),false);
 assert.equal(schemas.ProxyOperation.required.includes("kind"),false);
 for(const name of ["ProxyResolverEnvelope","ProxyCertificateObservation","ProxyProbeEvidence","ProxyDNSProviderDefinition"])assert.ok(manifest.proxy.schemas[name],name);
});
