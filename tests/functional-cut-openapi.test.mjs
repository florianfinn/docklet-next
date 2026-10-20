import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (name) => JSON.parse(readFileSync(new URL(`../api/${name}`, import.meta.url), "utf8"));
const methods = new Set(["get", "post", "put", "patch", "delete", "head", "options", "trace"]);
const laterOperations = new Map([
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
