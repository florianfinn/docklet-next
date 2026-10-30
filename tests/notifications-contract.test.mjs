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
  const codes = ["notification.config-invalid", "notification.hub-only", "notification.dispatch-invalid", "notification.dispatch-unsupported", "notification.dispatch-stale", "notification.test-busy", "notification.test-sent", "notification.test-failed", "notification.target-disabled", "delivery-unreachable", "delivery-rejected", "delivery-auth", "delivery-invalid", "autoheal.settings-invalid", "autoheal.settings-env-managed", "settings.key-managed"];
  for (const locale of ["en", "de"]) {
    const catalogue = read(`web/src/features/settings/messages/${locale}.ts`);
    for (const code of codes) assert.ok(hasCode(catalogue, code), `${locale}: ${code}`);
    const runs = read(`web/src/data/messages/run-${locale}.ts`);
    for (const code of ["autoheal.restarted", "autoheal.restart-failed", "autoheal.limit-reached", "autoheal.interrupted", "notification.delivery-failed"]) assert.ok(hasCode(runs, code), `${locale}: ${code}`);
  }
});

test("v10 manifest contains the actual Auto Heal and notification dispatch producers", () => {
  const spec = JSON.parse(read("api/openapi.json"));
  const fixture = JSON.parse(read("types/contract/testdata/agent-hub-v11.json"));
  assert.equal(fixture.minimumAgentContractVersion, 11);
  assert.deepEqual(fixture.notificationDispatch.autoHeal, spec.components.schemas.NotificationDispatchAutoHeal);
  assert.deepEqual(fixture.notificationDispatch.backupFailed, spec.components.schemas.NotificationDispatchBackupFailed);
  for (const provider of ["Discord", "Email", "Telegram", "Signal", "Slack", "Ntfy", "Pushover", "Gotify", "Matrix", "GoogleChat", "Generic"]) assert.ok(spec.components.schemas[`NotificationConfig${provider}`], provider);
  assert.ok(spec.components.schemas.NotificationEventMap);
  assert.deepEqual(spec.components.schemas.NotificationTestResponse.properties.code.enum, ["notification.test-sent", "notification.test-failed"]);
});
