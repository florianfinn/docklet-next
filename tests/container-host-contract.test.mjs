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
