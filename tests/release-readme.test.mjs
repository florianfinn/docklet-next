// Source: docklet-next/README.md
// Revision: e32adfed592cb71a2c8a85c8a5e539e7522ae8bc
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original drift guard for the README quick-start Compose example.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const readme = readFileSync("README.md", "utf8");
const template = readFileSync("deploy/hub/compose.yaml", "utf8");
const { next } = JSON.parse(readFileSync("deploy/release/version.json", "utf8"));

function quickStartCompose() {
  const blocks = [...readme.matchAll(/```yaml\n([\s\S]*?)```/g)].map((m) => m[1]);
  assert.equal(blocks.length, 1, "README has exactly one YAML example");
  return blocks[0];
}

test("README quick start pins the official hub image at the release train version", () => {
  const images = [...quickStartCompose().matchAll(/^\s*image:\s*(\S+)\s*$/gm)].map((m) => m[1]);
  assert.deepEqual(images, [`ghcr.io/florianfinn/docklet-next-hub:${next}`]);
});

test("README quick start keeps the template's fixed paths, volumes and port", () => {
  const example = quickStartCompose().split("\n").map((line) => line.trimEnd());
  const fixed = template.split("\n").map((line) => line.trimEnd()).filter((line) =>
    /^\s+(DATABASE_URL|DOCKLET_BACKUP_DIR|PROJECTS_DIRECTORY|DOCKER_CONFIG|stop_grace_period|restart):/.test(line) ||
    /^\s+- (hub-data|hub-backups|\/var\/run\/docker\.sock):/.test(line) ||
    /^\s+(hub-data|hub-backups): \{\}$/.test(line));
  assert.equal(fixed.length, 11, "template still has the expected fixed lines");
  for (const line of fixed) assert.ok(example.includes(line), `README example keeps: ${line.trim()}`);
  assert.ok(example.includes('      - "127.0.0.1:3552:3552"'), "README example publishes on loopback only");
  assert.ok(example.some((line) => line.includes("ENCRYPTION_KEY: ${ENCRYPTION_KEY:?")), "README example requires a key");
});
