import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { assertNoDrift } from "../scripts/openapi-contract.mjs";

test("the contract guard rejects a changed Huma operation", () => {
  const original = readFileSync(new URL("../api/openapi.json", import.meta.url));
  const types = readFileSync(new URL("../web/src/api/generated/schema.ts", import.meta.url));
  const changed = JSON.parse(original);
  changed.paths["/health"].get.operationId = "health-check-changed";
  const changedExport = Buffer.from(JSON.stringify(changed));

  assert.doesNotThrow(() => assertNoDrift(original, types, original, types));
  assert.throws(
    () => assertNoDrift(original, types, changedExport, types),
    /API contract drift: api\/openapi\.json/,
  );
  assert.throws(
    () => assertNoDrift(original, types, original, Buffer.from("changed generated types")),
    /API contract drift: web\/src\/api\/generated\/schema\.ts/,
  );
});
