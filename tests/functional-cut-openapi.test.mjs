import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (name) => JSON.parse(readFileSync(new URL(`../api/${name}`, import.meta.url), "utf8"));
const methods = new Set(["get", "post", "put", "patch", "delete", "head", "options", "trace"]);
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
  assert.equal(current.size, manifest.remainingOperations);
  assert.equal(removed.size, manifest.removedOperations.length);
  assert.equal(removed.size, 159);
  for (const [key, id] of removed) {
    assert.equal(baseline.get(key), id, `removed operation must exist in the baseline: ${key}`);
    assert.equal(current.has(key), false, `removed route reappeared: ${key}`);
    assert.equal([...current.values()].includes(id), false, `removed operation ID reappeared: ${id}`);
  }
  for (const [key, id] of baseline) {
    if (removed.has(key)) continue;
    assert.equal(current.get(key), id, `retained operation changed: ${key}`);
  }
  for (const key of current.keys()) {
    assert.ok(baseline.has(key), `unexpected operation added during the cut: ${key}`);
  }
});
