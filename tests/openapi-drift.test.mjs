import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
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

test("every lifecycle and policy producer reason is declared and translated", () => {
  const spec = JSON.parse(readFileSync(new URL("../api/openapi.json", import.meta.url)));
  const source = readFileSync(new URL("../types/image/reason.go", import.meta.url), "utf8");
  const declared = new Set(source.match(/const ReasonEnum = "([^"]+)"/)[1].split(","));
  for (const [name, fields] of Object.entries({
    SchedulerOutcome: ["code"], SchedulerTargetOutcome: ["code"],
    JobscheduleJobStatus: ["code"], UpdaterAutoUpdatePolicy: ["code", "applyCode", "bookkeepingCode"],
    UpdaterSystemImageHint: ["code"], ProjectUpdateInfo: ["policyCode"],
  })) {
    for (const field of fields) assert.deepEqual(new Set(spec.components.schemas[name].properties[field].enum), declared, `${name}.${field}`);
  }
  for (const code of spec.components.schemas.ImageProblem.properties.code.enum) assert.ok(declared.has(code), code);
  for (const locale of ["en", "de"]) {
    const catalogue = readFileSync(new URL(`../web/src/features/images/messages/errors-${locale}.ts`, import.meta.url), "utf8");
    for (const code of declared) assert.ok(catalogue.includes(`"${code.replaceAll(".", "_")}"`), `${locale}: missing reason ${code}`);
  }
  const directories = ["../backend/internal/project/", "../backend/internal/project/children/lifecycle/",
    "../backend/internal/updater/", "../backend/internal/updater/children/policies/", "../backend/internal/job/"];
  for (const directory of directories) {
    for (const name of readdirSync(new URL(directory, import.meta.url))) {
      if (!name.endsWith(".go") || name.endsWith("_test.go") ||
          (directory.endsWith("project/") && !name.startsWith("lifecycle_"))) continue;
      const source = readFileSync(new URL(directory + name, import.meta.url), "utf8");
      const patterns = [ /"((?:image|registry|update|restore|runtime|run|job)\.[a-z_-]+)"/g,
        /\bCode\s*:\s*"([^"]+)"/g, /\b(?:code|Code)\s*:?=\s*"([^"]+)"/g,
        /(?:Failure|skip)\(\s*"([^"]+)"/g, /policyCode\([^,]+,\s*"([^"]+)"/g ];
      for (const pattern of patterns) for (const [, code] of source.matchAll(pattern))
        assert.ok(declared.has(code), `${directory}${name}: undeclared reason ${code}`);
    }
  }
});

test("v7 policy manifest binds hub schedules, frozen revisions and notification producers", () => {
  const spec = readFileSync(new URL("../api/openapi.json", import.meta.url));
  const fixture = readFileSync(new URL("../types/contract/testdata/agent-hub-v7.json", import.meta.url));
  const manifest = JSON.parse(fixture);
  assert.equal(manifest.imagePolicies.owner, "hub");
  assert.equal(manifest.imagePolicies.defaultMode, "off");
  assert.deepEqual(manifest.imagePolicies.targetKinds, ["project"]);
  assert.equal(manifest.imagePolicies.routes.length, 5);
  const document = JSON.parse(spec);
  for (const schema of ["JobscheduleConfig", "JobscheduleUpdate"]) {
    assert.equal(document.components.schemas[schema].properties.pollingInterval, undefined);
    assert.equal(document.components.schemas[schema].properties.autoUpdateInterval, undefined);
  }
  for (const suffix of ["AutoUpdatePolicy", "UpdateScheduleConfig", "UpdateCandidate", "SystemImageHints"]) {
    const changed = JSON.parse(spec);
    const name = Object.keys(manifest.imagePolicies.schemas).find((key) => key.endsWith(suffix));
    delete changed.components.schemas[name].properties[Object.keys(changed.components.schemas[name].properties)[0]];
    assert.notDeepEqual(agentFixture(Buffer.from(JSON.stringify(changed)), fixture), fixture);
  }
  assert.equal(manifest.imageSafety.inactiveCapabilities.includes("policies"), false);
  for (const [path, item] of Object.entries(document.paths)) {
    if (!path.includes("/updater/")) continue;
    const expected = [...path.matchAll(/\{([^}]+)\}/g)].map((match) => match[1]).sort();
    for (const operation of Object.values(item)) {
      if (!operation?.operationId) continue;
      const actual = (operation.parameters ?? []).filter((parameter) => parameter.in === "path").map((parameter) => parameter.name).sort();
      assert.deepEqual(actual, expected, operation.operationId);
    }
  }
});
