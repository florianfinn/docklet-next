import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), "utf8");
// next-intl forbids dots in keys, so a wire code nests its leaf under the group in the "codes" block.
const hasCode = (catalogue, code) => {
  const codes = catalogue.slice(catalogue.indexOf("  codes: {"));
  const [group, leaf] = code.includes(".") ? code.split(/\.(.*)/s) : [undefined, code];
  const scope = group === undefined ? codes : codes.slice(codes.indexOf(`    ${group}: {`));
  return catalogue.includes("  codes: {") && (group === undefined || codes.includes(`    ${group}: {`)) && scope.slice(0, scope.indexOf("\n  },")).includes(`"${leaf}":`);
};
test("notification and Auto Heal codes have English and German catalogue entries", () => {
  const codes = ["notification.name-taken", "notification.name-invalid", "notification.target-not-found", "settings.activity-limit-invalid", "notification.config-invalid", "notification.hub-only", "notification.dispatch-invalid", "notification.dispatch-unsupported", "notification.dispatch-stale", "notification.test-busy", "notification.test-sent", "notification.test-failed", "notification.target-disabled", "delivery-unreachable", "delivery-rejected", "delivery-auth", "delivery-invalid", "autoheal.settings-invalid", "autoheal.settings-env-managed", "settings.key-managed"];
  for (const locale of ["en", "de"]) {
    const catalogue = read(`web/src/features/settings/messages/${locale}.ts`);
    for (const code of codes) assert.ok(hasCode(catalogue, code), `${locale}: ${code}`);
    const runs = read(`web/src/data/messages/run-${locale}.ts`);
    for (const code of ["autoheal.restarted", "autoheal.restart-failed", "autoheal.limit-reached", "autoheal.interrupted", "notification.delivery-failed"]) assert.ok(hasCode(runs, code), `${locale}: ${code}`);
  }
});

test("v10 manifest contains the actual Auto Heal and notification dispatch producers", () => {
  const spec = JSON.parse(read("api/openapi.json"));
  const fixture = JSON.parse(read("types/contract/testdata/agent-hub-v12.json"));
  assert.equal(fixture.minimumAgentContractVersion, 12);
  assert.deepEqual(fixture.notificationDispatch.autoHeal, spec.components.schemas.NotificationDispatchAutoHeal);
  assert.deepEqual(fixture.notificationDispatch.backupFailed, spec.components.schemas.NotificationDispatchBackupFailed);
  for (const provider of ["Discord", "Email", "Telegram", "Signal", "Slack", "Ntfy", "Pushover", "Gotify", "Matrix", "GoogleChat", "Generic"]) assert.ok(spec.components.schemas[`NotificationConfig${provider}`], provider);
  assert.ok(spec.components.schemas.NotificationEventMap);
  assert.deepEqual(spec.components.schemas.NotificationTestResponse.properties.code.enum, ["notification.test-sent", "notification.test-failed"]);
});

test("M19 freezes target-ID routes, import codes and required v15 hints", () => {
  const spec = JSON.parse(read("api/openapi.json"));
  const fixture = JSON.parse(read("types/contract/testdata/agent-hub-v17.json"));
  assert.equal(fixture.version, 17);
  assert.equal(fixture.minimumAgentContractVersion, 17);
  assert.deepEqual(fixture.autoHeal.settings, spec.components.schemas.AutohealSettings);
  assert.deepEqual(fixture.autoHeal.update, spec.components.schemas.AutohealUpdate);
  for (const member of ["cooldownMinutes", "invalidFields"]) assert.ok(fixture.autoHeal.settings.required.includes(member));
  assert.equal(fixture.autoHeal.update.required.includes("cooldownMinutes"), false);
  for (const member of ["maxRestarts", "windowMinutes", "cooldownMinutes"]) {
    assert.equal(fixture.autoHeal.settings.properties[member].minimum, undefined);
    assert.equal(fixture.autoHeal.settings.properties[member].maximum, undefined);
  }
  for (const member of ["excludedStacks", "excludedContainers"]) assert.equal(fixture.autoHeal.settings.properties[member].maxItems, undefined);
  assert.equal(fixture.autoHeal.settings.properties.invalidFields.readOnly, true);
  assert.equal(Object.hasOwn(fixture.autoHeal.update.properties, "invalidFields"), false);
  for (const [method,path,operation] of [
    ["put", "/environments/{id}/notifications/settings/{targetId}", "update-notification-settings"],
    ["delete", "/environments/{id}/notifications/settings/{targetId}", "delete-notification-settings"],
    ["post", "/environments/{id}/notifications/test/{targetId}", "test-notification"],
    ["post", "/stack-import", "import-stack-source"],
  ]) assert.equal(spec.paths[path][method].operationId, operation);
  for (const path of ["/environments/{id}/notifications/settings/{provider}", "/environments/{id}/notifications/test/{provider}", "/settings/search", "/settings/categories", "/customize/search", "/customize/categories"]) assert.equal(spec.paths[path], undefined);
  assert.deepEqual(spec.components.schemas.NotificationProblem.properties.code.enum, ["notification.config-invalid", "notification.hub-only", "notification.dispatch-invalid", "notification.dispatch-unsupported", "notification.dispatch-stale", "notification.test-busy", "notification.name-taken", "notification.name-invalid", "notification.target-not-found"]);
  assert.ok(spec.components.schemas.NotificationResponse.required.includes("name"));
  assert.deepEqual(spec.components.schemas.StackimportProblem.properties.code.enum, ["import.invalid-reference", "import.registry-not-configured", "import.not-found", "import.too-large", "import.timeout", "import.redirect-refused", "import.source-unavailable", "import.invalid-compose", "import.unauthorized", "import.busy"]);
  assert.deepEqual(spec.components.schemas.StackimportResult.properties.found.items.enum, ["import.found.compose", "import.found.env-example", "import.found.image-config", "import.found.ports", "import.found.volumes", "import.found.environment"]);
  assert.deepEqual(spec.components.schemas.StackimportResult.properties.missing.items.enum, ["import.missing.env-file", "import.missing.label-file", "import.missing.include", "import.missing.extends", "import.missing.build-context", "import.missing.variables", "import.missing.compose"]);
});
