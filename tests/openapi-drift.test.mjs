import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { assertNoDrift } from "../scripts/openapi-contract.mjs";
import { agentFixture } from "../scripts/agent-contract.mjs";

test("the v7 manifest records the merged activity discovery producer", () => {
  const spec = readFileSync(new URL("../api/openapi.json", import.meta.url));
  const fixture = readFileSync(new URL("../types/contract/testdata/agent-hub-v7.json", import.meta.url));
  assert.deepEqual(agentFixture(spec, fixture), fixture);
  const changed = JSON.parse(spec);
  const recoveryName = JSON.parse(fixture).activityDiscovery.recovery.$ref.split("/").at(-1);
  const recovery = changed.components.schemas[recoveryName];
  delete recovery.properties.unknownReason;
  assert.notDeepEqual(agentFixture(Buffer.from(JSON.stringify(changed)), fixture), fixture);
  const manifest = JSON.parse(fixture);
  assert.ok(manifest.imageSafety.edgeCommands.includes("image.counts"));
  assert.ok(manifest.activityDiscovery.optionalFields.includes("stepTrace"));
  assert.ok(manifest.activityDiscovery.optionalFields.includes("recovery"));
});

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
