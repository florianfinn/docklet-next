import test from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {checkCompiledNotices} from "../scripts/core-compiled-policy.mjs";
const evidence=JSON.parse(readFileSync(new URL("../docs/provenance/core-compiled-notices.json",import.meta.url)));
test("compiled licence acceptance is restricted to the exact reviewed files",()=>{
 assert.doesNotThrow(()=>checkCompiledNotices(evidence));
 for(const mutate of [m=>m.files.pop(),m=>m.files[0].sha256="0".repeat(64),m=>m.files[0].version="v1.77.2",m=>m.files[0].path="other.go",m=>m.cgo="1",m=>m.files[0].targets=["hub"],m=>m.files[0].reason="",m=>m.files.push(m.files[0])]){
  const changed=structuredClone(evidence);mutate(changed);assert.throws(()=>checkCompiledNotices(changed));
 }
});
