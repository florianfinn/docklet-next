import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (name) => JSON.parse(readFileSync(new URL(`../api/${name}`, import.meta.url), "utf8"));
const methods = new Set(["get", "post", "put", "patch", "delete", "head", "options", "trace"]);
const laterOperations = new Map([
  ["POST /users/{userId}/disable", "disable-user"],
  ["POST /users/{userId}/enable", "enable-user"],
  ["POST /users/{userId}/reset-password", "reset-user-password"],
  ["PUT /users/{userId}/administrator", "set-user-admin"],
  ["PUT /users/{userId}/app-assignments", "set-user-app-assignments"],
  ["POST /environments/{id}/agent/restart", "restartEnvironmentAgent"],
  ["GET /environments/{id}/agent/restarts/{runId}", "getEnvironmentAgentRestart"],
  ["PUT /environments/{id}/containers/{containerId}/origin", "assign-container-origin"],
  ["DELETE /environments/{id}/containers/{containerId}/origin", "reset-container-origin"],
  ["POST /environments/{id}/containers/{containerId}/recreate", "recreate-container"],
  ["POST /environments/{id}/projects/{projectId}/stop", "stop-project"],
  ["GET /environments/{id}/auto-heal", "get-auto-heal-settings"],
  ["PUT /environments/{id}/auto-heal", "update-auto-heal-settings"],
  ["GET /backup-targets", "list-backup-targets"],
  ["POST /backup-targets", "create-backup-target"],
  ["DELETE /backup-targets/{id}", "retire-backup-target"],
  ["GET /backup-targets/{id}", "get-backup-target"],
  ["PUT /backup-targets/{id}", "update-backup-target"],
  ["GET /backup-targets/{id}/recovery-key", "get-backup-target-recovery-key"],
  ["POST /backup-targets/{id}/test", "test-backup-target"],
  ["GET /environments/{id}/volumes/backups/overview", "get-volume-backup-overview"],
  ["GET /environments/{id}/volumes/backups/repository", "get-volume-backup-repository"],
  ["POST /environments/{id}/volumes/backups/repository/check", "check-volume-backup-repository"],
  ["GET /environments/{id}/volumes/backups/targets/local/recovery-key", "get-local-backup-recovery-key"],
  ["PUT /environments/{id}/volumes/backups/targets/local/recovery-key", "attach-local-backup-recovery-key"],
  ["GET /environments/{id}/volumes/backups/targets/{targetId}/references", "get-volume-backup-target-references"],
  ["POST /environments/{id}/volumes/backups/targets/{targetId}/test", "test-host-backup-target"],

  ["POST /environments/{id}/vulnerabilities/scan", "scan-host-vulnerabilities"],
  ["GET /environments/{id}/vulnerabilities/runs/{activityId}", "get-vulnerability-scan-run"],
  ["GET /environments/{id}/vulnerabilities/runs", "list-vulnerability-scan-runs"],
  ["GET /environments/{id}/updater/policies/{kind}/{targetId}", "get-auto-update-policy"],
  ["PUT /environments/{id}/updater/policies/{kind}/{targetId}", "set-auto-update-policy"],
  ["GET /environments/{id}/updater/schedules", "get-update-schedules"],
  ["PUT /environments/{id}/updater/schedules", "set-update-schedules"],
  ["GET /environments/{id}/updater/system-image-hints", "get-system-image-hints"],
  ["POST /environments/{id}/updater/projects/{projectId}/update", "update-project-images"],
  ["POST /environments/{id}/updater/{kind}/{targetId}/restore-points/{pointId}/restore", "restore-update"],
  ["GET /environments/{id}/updater/projects/{projectId}/baseline", "get-update-baseline"],
  ["GET /environments/{id}/updater/{kind}/{targetId}/restore-points", "list-restore-points"],
  ["DELETE /environments/{id}/updater/{kind}/{targetId}/restore-points/{pointId}", "delete-restore-point"],
  ["GET /environments/{id}/updater/restore-points", "list-host-restore-points"],
  ["DELETE /environments/{id}/updater/restore-points/{pointId}", "delete-host-restore-point"],
  ["GET /environments/{id}/volumes/prune", "plan-volume-prune"],
  ["GET /hub/discovery", "discoverHub"],
	["GET /auth/mode", "get-sign-in-mode"],
  ["GET /capabilities", "get-capabilities"],
  ["GET /backups/maintenance", "get-system-backup-maintenance"],
  ["GET /backups/storage", "get-system-backup-storage"],
  ["POST /backups/upload", "import-system-backup-upload"],
  ["GET /backups/{id}/download", "download-system-backup"],
  ["GET /environments/0/runtime-runs/{runId}", "get-runtime-run-outcome"],
  ["GET /environments/{id}/runtime-runs/{runId}", "get-hub-runtime-run"],
  ["POST /environments/0/runtime-registration", "register-runtime-executor"],
  ["POST /environments/0/runtime-runs", "execute-runtime-run"],
  ["POST /environments/0/runtime-runs/{runId}/cancel", "cancel-runtime-run"],
  ["POST /environments/0/runtime-target", "resolve-runtime-target"],
  ["POST /environments/{id}/runtime-runs/{runId}/cancel", "cancel-hub-runtime-run"],
  ["POST /environments/{id}/runtime-runs/{runId}/release", "release-runtime-run"],
]);
const retiredSystemRecovery = new Map([
  ["GET /backups/volumes/config", "get-system-volume-backup-config"],
  ["PUT /backups/volumes/config", "update-system-volume-backup-config"],
  ["GET /backups/volumes/options", "list-system-volume-backup-options"],
  ["POST /backups/volumes/run", "run-system-volume-backups"],

  ["PUT /backups/recovery-key", "set-system-backup-recovery-key"],
  ["POST /backups/recovery-key/generate", "generate-system-backup-recovery-key"],
  ["POST /backups/{id}/files/browse", "browse-system-backup-files"],
  ["POST /backups/{id}/restore-files", "restore-system-backup-files"],
]);
const operations = (document) => new Map(
  Object.entries(document.paths).flatMap(([path, entries]) =>
    Object.entries(entries)
      .filter(([method]) => methods.has(method))
      .map(([method, operation]) => [`${method.toUpperCase()} ${path}`, operation.operationId]),
  ),
);

test("the functional cut removes only the reviewed OpenAPI operations", () => {
  const baseline = operations(read("openapi.before-cut.json"));
  const current = operations(read("openapi.json"));
  const manifest = read("removed-operations.json");
  const removed = new Map(manifest.removedOperations.map(({ method, path, operationId }) =>
    [`${method} ${path}`, operationId],
  ));

  assert.equal(baseline.size, manifest.baselineOperations);
  assert.equal(current.size, manifest.remainingOperations + laterOperations.size - retiredSystemRecovery.size);
  assert.equal(removed.size, manifest.removedOperations.length);
  assert.equal(removed.size, 159);
  for (const [key, id] of removed) {
    assert.equal(baseline.get(key), id, `removed operation must exist in the baseline: ${key}`);
    assert.equal(current.has(key), false, `removed route reappeared: ${key}`);
    assert.equal([...current.values()].includes(id), false, `removed operation ID reappeared: ${id}`);
  }
  for (const [key, id] of baseline) {
    if (removed.has(key)) continue;
    if (retiredSystemRecovery.has(key)) {
      assert.equal(id, retiredSystemRecovery.get(key));
      assert.equal(current.has(key), false, `obsolete system recovery route remains: ${key}`);
      assert.equal([...current.values()].includes(id), false, `obsolete system recovery ID remains: ${id}`);
      continue;
    }
    assert.equal(current.get(key), id, `retained operation changed: ${key}`);
  }
  for (const [key, id] of current) {
    if (laterOperations.has(key)) {
      assert.equal(id, laterOperations.get(key));
      continue;
    }
    assert.ok(baseline.has(key), `unexpected operation added during the cut: ${key}`);
  }
});
