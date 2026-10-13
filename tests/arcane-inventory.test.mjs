import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const load = (name) => JSON.parse(readFileSync(new URL(`../docs/provenance/${name}.json`, import.meta.url)));
const modules = load("arcane-modules");
const binary = load("arcane-binary");
const licenses = load("arcane-license-packages");
const graph = [1, 2, 3, 4].flatMap((n) => load(`arcane-package-graph-${n}`).packages);

test("adoption snapshot includes declarations, replacements and every Arcane helper", () => {
  assert.equal(modules.declaredEntries, 419);
  assert.equal(modules.packages.reduce((n, p) => n + p.declarations.length, 0), 419);
  assert.equal(modules.packages.length, 334);
  assert.equal(new Set(modules.packages.map((p) => `${p.name}@${p.version}`)).size, 334);
  assert.equal(modules.packages.filter((p) => p.name.startsWith("go.getarcane.app/")).length, 11);
  assert.deepEqual(modules.unsourceableModules, []);
  assert.deepEqual(modules.blockedModules, []);
  for (const p of modules.packages) {
    assert.equal(p.disposition, "accepted");
    assert.equal(p.sourceAvailable, true);
    assert.ok(p.license && !/Unknown|NOASSERTION/.test(p.license));
    assert.ok(p.licenseFiles.length > 0);
    for (const f of [...p.licenseFiles, ...p.nestedLicenseEvidence]) assert.match(f.sha256, /^[a-f0-9]{64}$/);
    if (p.name.startsWith("go.getarcane.app/")) assert.match(p.sourceRevision, /^[a-f0-9]{40}$/);
    if (p.name.startsWith("github.com/getarcaneapp/arcane/")) assert.equal(p.sourceRevision, modules.revision);
    if (p.name === "github.com/docker/buildx") assert.equal(p.selectedVersion, "v0.32.1");
  }
});

test("every measured binary package joins to module and resolved licence evidence", () => {
  assert.equal(binary.moduleCount, 297);
  assert.equal(binary.packages.length, binary.moduleCount);
  assert.equal(graph.length, binary.packageCount);
  assert.equal(graph.length, 1587);
  assert.equal(new Set(graph.map((p) => p.name)).size, graph.length);
  assert.equal(licenses.packages.length, 535);
  assert.equal(licenses.manualEntryCount, 214);
  for (const p of graph) {
    const m = binary.packages.find((m) => m.name === p.module && m.version === p.version);
    const l = licenses.packages.find((l) => l.name === p.licenseGroup);
    assert.ok(m && l);
    assert.equal(l.module, m.name);
    assert.ok(l.licenseFiles.length > 0);
    assert.ok(p.targets.includes("hub") || p.targets.includes("agent"));
  }
});

test("mixed licences and the EDL choice survive inventory generation", () => {
  const paho = binary.packages.find((p) => p.name === "github.com/eclipse/paho.golang");
  assert.equal(paho.license, "EPL-2.0 OR EDL-1.0");
  assert.equal(paho.selectedAlternative, "EDL-1.0");
  assert.ok(paho.licenseFiles.some((f) => f.path === "edl-v10"));
  const yaml = binary.packages.find((p) => p.name === "gopkg.in/yaml.v3");
  assert.equal(yaml.license, "Apache-2.0 AND MIT");
  const securejoin = binary.packages.find((p) => p.name === "github.com/cyphar/filepath-securejoin");
  assert.equal(securejoin.license, "BSD-3-Clause AND MPL-2.0");
  const sys = modules.packages.filter((p) => p.name.startsWith("go.getarcane.app/sys/"));
  for (const p of sys) assert.ok(p.licenseFiles.some((f) => f.path === "repository:sys/LICENSE"));
});
