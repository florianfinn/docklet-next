// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original guard that M17 typed failures stay on their own operations.

import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const spec = JSON.parse(readFileSync(new URL("../api/openapi.json", import.meta.url), "utf8"));
// Responses of routes that existed before M17, exported from main at the milestone base.
const before = JSON.parse(readFileSync(new URL("./fixtures/pre-m17-history-responses.json", import.meta.url), "utf8"));
const typed = {
  "get-history-sample": ["502", "503"],
  "get-container-history": ["400", "503"],
  "get-project-history": ["400", "503"],
  "get-host-history": ["400", "503"],
  "get-runtime-conditions": ["503"],
  "get-history-capacity": ["503"],
  "get-my-ui-preferences": ["403"],
  "put-my-ui-preferences": ["400", "403", "413"],
};

test("routes that predate M17 keep their responses", () => {
  for (const [operationId, { method, path, responses }] of Object.entries(before))
    assert.deepEqual(spec.paths[path][method].responses, responses, operationId);
});

test("history and preference problems appear only at the statuses their operations return", () => {
  const found = {};
  for (const item of Object.values(spec.paths))
    for (const operation of Object.values(item)) {
      const statuses = Object.entries(operation.responses ?? {})
        .filter(([, response]) => /HistoryProblem|PreferencesProblem/.test(JSON.stringify(response)))
        .map(([status]) => status).sort();
      if (statuses.length) found[operation.operationId] = statuses;
    }
  assert.deepEqual(found, typed);
});
