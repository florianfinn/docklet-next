// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import test from "node:test";
const read = path => readFileSync(new URL(`../${path}`,import.meta.url),"utf8");

test("v11 contains every M14 code, shape and bilingual catalogue entry",()=>{
 const spec=JSON.parse(read("api/openapi.json"));
 const fixture=JSON.parse(read("types/contract/testdata/agent-hub-v11.json")).containerHostOperations;
 for(const [name,schema] of Object.entries(fixture.schemas)) assert.deepEqual(schema,spec.components.schemas[name],name);
 for(const name of ["ContainerProblem","EnvironmentHostProblem"]){
  for(const code of fixture.schemas[name].properties.code.enum){
   const [group,leaf]=code.split(".");
   for(const locale of ["en","de"]){
    const catalogue=read(`web/src/data/messages/run-${locale}.ts`);
    assert.ok(catalogue.includes(`    ${group}: {`),code);
    assert.ok(catalogue.includes(`"${leaf}":`),`${locale}: ${code}`);
   }
  }
 }
 assert.equal(fixture.shell.frameProtocolVersion,1);
 assert.deepEqual(fixture.shell.closeCodes,[1000,1008]);
 assert.equal(fixture.restart.command,"system.agent.restart");
 assert.ok(read("backend/pkg/libarcane/edge/commands.go").includes('PathPattern: "/api/system/agent/restart", CommandName: "system.agent.restart"'));
 assert.equal(spec.paths["/system/agent/restart"],undefined,"S2 owns the behaviour");
});

test("stack stop, definition states and bilingual descriptions are in the v11 contract",()=>{
 const spec=JSON.parse(read("api/openapi.json"));
 const fixture=JSON.parse(read("types/contract/testdata/agent-hub-v11.json")).containerHostOperations;
 assert.ok(fixture.runtimeOperations.includes("stop-project"));
 assert.deepEqual(fixture.stackServiceState,spec.components.schemas.ProjectRuntimeService.properties.status);
 assert.ok(fixture.stackServiceState.enum.includes("not-created"));
 assert.ok(fixture.routes.some(route=>route.operationId==="stop-project"));
 assert.deepEqual(fixture.settings.enum,["down","stop"]);
 assert.deepEqual(fixture.stackStopMode,{values:["down","stop"],default:"down",environment:"0"});
 for(const locale of ["en","de"]){
  const catalogue=read(`web/src/features/stacks/messages/${locale}.ts`);
  for(const code of ["not-created","definition-invalid"])assert.ok(catalogue.includes(`"${code}":`),`${locale}: ${code}`);
 }
});

test("typed host refusals are declared on every affected hub operation",()=>{
 const spec=JSON.parse(read("api/openapi.json"));
 const expected=["get-auto-heal-settings","update-auto-heal-settings","update-settings","createEnvironment","updateEnvironment","testConnection"];
 for(const item of Object.values(spec.paths)) for(const op of Object.values(item)) {
  if(expected.includes(op?.operationId) || op?.responses?.["202"]?.description?.startsWith("New admission only")) {
   assert.ok(JSON.stringify(op.responses).includes("#/components/schemas/EnvironmentHostProblem"),op.operationId);
  }
 }
 for(const id of expected) assert.ok(Object.values(spec.paths).some(item=>Object.values(item).some(op=>op?.operationId===id)),id);
 assert.ok(spec.components.schemas.EnvironmentHostProblem.properties.code.enum.includes("agent.restart-interrupted"));
 assert.ok(spec.components.schemas.UpdaterAutoUpdatePolicy.properties.code.enum.includes("host.restarting"));
});
