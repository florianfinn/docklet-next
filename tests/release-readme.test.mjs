// Source: docklet-next/README.md
// Revision: e32adfed592cb71a2c8a85c8a5e539e7522ae8bc
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original drift guard for the README quick-start Compose example.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";

const { parse } = createRequire(new URL("../web/package.json", import.meta.url))("yaml");
const readme = readFileSync("README.md", "utf8");
const template = parse(readFileSync("deploy/hub/compose.yaml", "utf8"));
const { next } = JSON.parse(readFileSync("deploy/release/version.json", "utf8"));

function quickStart() {
  const blocks = [...readme.matchAll(/```yaml\n([\s\S]*?)```/g)].map((m) => m[1]);
  assert.equal(blocks.length, 1, "README has exactly one YAML example");
  return parse(blocks[0]);
}

test("README quick start pins the official hub image at the release train version", () => {
  const example = quickStart();
  assert.deepEqual(Object.keys(example.services), ["hub"]);
  assert.equal(example.services.hub.image, `ghcr.io/florianfinn/docklet-next-hub:${next}`);
});

test("README quick start matches the template where the hub needs fixed values", () => {
  const hub = quickStart().services.hub;
  const source = template.services.hub;
  for (const key of ["restart", "stop_grace_period"]) assert.equal(hub[key], source[key], key);
  for (const key of ["DATABASE_URL", "DOCKLET_BACKUP_DIR", "PROJECTS_DIRECTORY", "DOCKER_CONFIG", "TZ"]) {
    assert.equal(hub.environment[key], source.environment[key], key);
  }
  assert.deepEqual(hub.volumes, source.volumes);
  assert.deepEqual(quickStart().volumes, template.volumes);
  assert.deepEqual(hub.ports, ["127.0.0.1:3552:3552"], "loopback only, template default port");
});

test("README quick start requires the deployment inputs and uses local sign-in", () => {
  const { environment } = quickStart().services.hub;
  // Intentional differences from the package: no image overrides, PUID/PGID or proxy sign-in inputs.
  assert.deepEqual(Object.keys(environment).sort(), ["APP_URL", "AUTH_MODE", "DATABASE_URL", "DOCKER_CONFIG",
    "DOCKLET_BACKUP_DIR", "ENCRYPTION_KEY", "PROJECTS_DIRECTORY", "TZ"]);
  assert.match(environment.APP_URL, /^\$\{APP_URL:\?/);
  assert.match(environment.ENCRYPTION_KEY, /^\$\{ENCRYPTION_KEY:\?/);
  assert.equal(environment.AUTH_MODE, "local");
});
