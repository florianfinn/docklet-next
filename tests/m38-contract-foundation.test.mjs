// Source: docklet-next/issues/602
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M38 contract foundation guards for refused tag names and the container check route.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const spec = JSON.parse(readFileSync(new URL("../api/openapi.json", import.meta.url), "utf8"));

test("every refusedTags carrier accepts tag names only, at most eight", () => {
  const carriers = Object.entries(spec.components.schemas).filter(([, schema]) => schema.properties?.refusedTags).map(([name]) => name).sort();
  assert.deepEqual(carriers, ["TailnetConnection", "TailnetProblem", "TailnetProvisioning", "TailnetTestCheck"]);
  for (const name of carriers) {
    const field = spec.components.schemas[name].properties.refusedTags;
    assert.equal(field.maxItems, 8, name);
    assert.equal(field.items.pattern, "^tag:[a-z0-9-]+$", name);
  }
});

test("the container check is a POST that answers an image UpdateInfo", () => {
  const op = spec.paths["/environments/{id}/updater/containers/{containerId}/check"]?.post;
  assert.equal(op?.operationId, "check-container-update");
  const ref = op.responses["200"].content["application/json"].schema.$ref;
  const body = spec.components.schemas[ref.split("/").pop()];
  assert.match(body.properties.data.$ref ?? JSON.stringify(body.properties.data), /UpdateInfo/);
});
