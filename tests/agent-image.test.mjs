// Source: docklet-next/issues/84
// Revision: 8c30a9afd998fed11dc97f61068507f57560553a
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original registry-free image packaging and CI contract guard.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

test("agent target ships only the agent executable and defaults to agent mode", () => {
  const dockerfile = readFileSync("deploy/hub/Dockerfile", "utf8");
  const agent = dockerfile.split("FROM ${RUNTIME_IMAGE} AS agent\n")[1]?.split("FROM ${RUNTIME_IMAGE} AS hub\n")[0];
  assert.ok(agent, "separate agent target is required");
  assert.match(agent, /COPY --from=build \/out\/docklet-agent \/usr\/local\/bin\/docklet-agent/);
  assert.doesNotMatch(agent, /\/out\/docklet-hub|--from=web/);
  assert.match(agent, /ENV AGENT_MODE=true EDGE_AGENT=false/);
  assert.match(agent, /ENTRYPOINT \["\/usr\/local\/bin\/docklet-agent"\]/);
  const workflow = readFileSync(".github/workflows/checks.yml", "utf8");
  assert.match(workflow, /docker build --target agent --tag docklet-next-agent:0\.0\.0/);
  assert.match(workflow, /node scripts\/smoke-agent-image.mjs docklet-next-agent:0\.0\.0/);
  assert.match(workflow, /docker build --tag docklet-next-hub:0\.0\.0/);
  assert.doesNotMatch(workflow, /docker push/);
});

test("deployment notes name onboarding authorization, retired endpoint and agent image requirements", () => {
  const readme = readFileSync("deploy/hub/README.md", "utf8");
  assert.match(readme, /Environment creation, updates and deletion require a\nglobal administrator/);
  assert.match(readme, /`GET \/api\/environments\/\{id\}\/deployment` returns HTTP 410/);
  assert.match(readme, /DOCKLET_AGENT_IMAGE/);
  assert.match(readme, /Ensure the selected agent image\nexists on each target host/);
});
