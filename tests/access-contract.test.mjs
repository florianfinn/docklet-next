// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import test from "node:test";
const read = path => readFileSync(new URL(`../${path}`,import.meta.url),"utf8");

test("the six access codes are published in OpenAPI and the generated DTOs",()=>{
 const codes=[...read("types/user/access.go").matchAll(/AccessCode = "(access\.[a-z-]+)"/g)].map(match=>match[1]);
 assert.equal(codes.length,6);
 const spec=JSON.parse(read("api/openapi.json"));
 const schema=spec.components.schemas.UserAccessProblem;
 assert.ok(schema,"UserAccessProblem is exported");
 assert.deepEqual([...schema.properties.code.enum].sort(),[...codes].sort());
 assert.deepEqual([...schema.required].sort(),["code","status"]);
 const generated=read("web/src/api/generated/schema.ts");
 for(const code of codes) assert.ok(generated.includes(`"${code}"`),code);
});
