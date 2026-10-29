import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), "utf8");
test("notification and Auto Heal codes have English and German catalogue entries", () => {
  const codes = ["notification.config-invalid", "notification.hub-only", "notification.dispatch-invalid", "notification.dispatch-unsupported", "notification.dispatch-stale", "notification.test-busy", "notification.test-sent", "notification.test-failed", "notification.target-disabled", "delivery-unreachable", "delivery-rejected", "delivery-auth", "delivery-invalid", "autoheal.settings-invalid", "autoheal.settings-env-managed", "settings.key-managed"];
  for (const locale of ["en", "de"]) {
    const catalogue = read(`web/src/features/settings/messages/${locale}.ts`);
    for (const code of codes) assert.ok(catalogue.includes(`"${code}":`), `${locale}: ${code}`);
    const runs = read(`web/src/data/messages/run-${locale}.ts`);
    for (const code of ["autoheal.restarted", "autoheal.restart-failed", "autoheal.limit-reached", "autoheal.interrupted", "notification.delivery-failed"]) assert.ok(runs.includes(`"${code}":`), `${locale}: ${code}`);
  }
});

test("v10 manifest contains the actual Auto Heal and notification dispatch producers", () => {
  const spec = JSON.parse(read("api/openapi.json"));
  const fixture = JSON.parse(read("types/contract/testdata/agent-hub-v10.json"));
  assert.equal(fixture.minimumAgentContractVersion, 10);
  assert.deepEqual(fixture.notificationDispatch.autoHeal, spec.components.schemas.NotificationDispatchAutoHeal);
  assert.deepEqual(fixture.notificationDispatch.backupFailed, spec.components.schemas.NotificationDispatchBackupFailed);
  for (const provider of ["Discord", "Email", "Telegram", "Signal", "Slack", "Ntfy", "Pushover", "Gotify", "Matrix", "GoogleChat", "Generic"]) assert.ok(spec.components.schemas[`NotificationConfig${provider}`], provider);
  assert.ok(spec.components.schemas.NotificationEventMap);
  assert.deepEqual(spec.components.schemas.NotificationTestResponse.properties.code.enum, ["notification.test-sent", "notification.test-failed"]);
});
