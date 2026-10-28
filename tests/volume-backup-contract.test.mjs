// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original v9 volume vocabulary, localization and admission contract proofs.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import test from "node:test";
import { agentFixture } from "../scripts/agent-contract.mjs";
const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), "utf8");

test("v9 backup vocabulary is fingerprinted and translated in both locales", () => {
  const text = read("types/contract/testdata/agent-hub-v9.json");
  const fixture = JSON.parse(text);
  const source = read("types/contract/agent.go");
  assert.match(source, /const AgentHubVersion = 9/);
  assert.match(source, /const MinimumAgentVersion = 9/);
  assert.ok(source.includes(createHash("sha256").update(text).digest("hex")));
  assert.equal(fixture.version, 9);
  assert.equal(fixture.minimumAgentContractVersion, 9);
  const problem = read("types/backup/problem.go");
  const codes = problem.match(/Code\s+string\s+`[^`]*enum:"([^"]+)"/)[1].split(",");
  const reasons = problem.match(/Reason\s+string\s+`[^`]*enum:"([^"]+)"/)[1].split(",");
  assert.deepEqual(fixture.volumeBackups.errorCodes, codes);
  assert.deepEqual(fixture.volumeBackups.uploadReasons, reasons);
  for (const locale of ["en", "de"]) {
    const catalogue = read(`web/src/features/volumes/messages/${locale}.ts`);
    for (const key of [...codes, ...reasons]) assert.ok(catalogue.includes(`${JSON.stringify(key)}:`), `${locale}: ${key}`);
  }
  assert.deepEqual(agentFixture(Buffer.from(read("api/openapi.json")), Buffer.from(text)), Buffer.from(text));
});

test("backup admissions advertise accepted IDs and removed routes stay absent", () => {
  const document = JSON.parse(read("api/openapi.json"));
  const fixture = JSON.parse(read("types/contract/testdata/agent-hub-v9.json")).volumeBackups;
  for (const path of [
    "/environments/{id}/volumes/{volumeName}/backups",
    "/environments/{id}/volumes/{volumeName}/backups/{backupId}/restore",
    "/environments/{id}/volumes/{volumeName}/backups/{backupId}/restore-files",
    "/environments/{id}/volumes/{volumeName}/backups/upload",
    "/environments/{id}/volumes/backups/repository/check",
    "/environments/{id}/volumes/backups/targets/{targetId}/test",
    "/backup-targets/{id}/test",
  ]) {
    const operation = document.paths[path].post;
    assert.ok(operation.responses["202"], path);
    assert.equal(operation.responses["200"], undefined, path);
    assert.ok(fixture.routes.some((route) => route.path === path && route.method === "POST"), path);
  }
  for (const path of fixture.retiredRoutes) assert.equal(document.paths[path], undefined, path);
  assert.equal(document.paths["/environments/{id}/volumes/backups/targets"], undefined, "hub never receives target sync");
  assert.ok(document["x-agent-volume-target-sync"].request.$ref);
  assert.equal(fixture.uploadLimit, 4 * 2 ** 30);
  assert.equal(fixture.expandedUploadLimit, 16 * 2 ** 30);
  assert.equal(fixture.edgeDownloadLimit, 512 * 2 ** 20);
});
