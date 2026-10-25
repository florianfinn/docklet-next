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

test("v7 lifecycle manifest changes with executable DTOs and admission operations", () => {
  const spec = readFileSync(new URL("../api/openapi.json", import.meta.url));
  const fixture = readFileSync(new URL("../types/contract/testdata/agent-hub-v7.json", import.meta.url));
  const manifest = JSON.parse(fixture);
  assert.equal(manifest.imageLifecycle.routes.length, 7);
  assert.equal(manifest.imageLifecycle.edgeRoutes.length, 6);
  assert.equal(manifest.imageLifecycle.retiredRoutes.length, 3);
  for (const route of manifest.imageLifecycle.retiredRoutes) {
    const operation = JSON.parse(spec).paths[route.path][route.method.toLowerCase()];
    assert.equal(operation.deprecated, true);
    assert.ok(JSON.stringify(operation.responses["409"]).includes("ImageOperationError"));
  }
  assert.ok(manifest.lifecycleOperations.includes("update-project"));
  assert.ok(manifest.lifecycleOperations.includes("restore-project"));
  assert.ok(manifest.imageSafety.capabilities.includes("restore"));
  for (const path of ["/environments/{id}/updater/projects/{projectId}/update", "/environments/{id}/updater/{kind}/{targetId}/restore-points/{pointId}/restore"]) {
    assert.equal(JSON.parse(spec).paths[path].post.parameters.find((parameter) => parameter.name.toLowerCase() === "idempotency-key").required, true);
  }
  for (const suffix of ["LifecycleIntent", "LifecycleResult", "RestorePoint", "RuntimeOptions"]) {
    const changed = JSON.parse(spec);
    const producer = Object.keys(manifest.imageLifecycle.schemas).find((key) => key.endsWith(suffix));
    const schema = changed.components.schemas[producer];
    delete schema.properties[Object.keys(schema.properties)[0]];
    assert.notDeepEqual(agentFixture(Buffer.from(JSON.stringify(changed)), fixture), fixture);
  }
});
