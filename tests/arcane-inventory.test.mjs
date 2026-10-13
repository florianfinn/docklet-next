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
  assert.equal(paho.license, "EPL-2.0 OR BSD-3-Clause");
  assert.equal(paho.selectedAlternative, "BSD-3-Clause");
  assert.ok(paho.licenseFiles.some((f) => f.path === "edl-v10"));
  const yaml = binary.packages.find((p) => p.name === "gopkg.in/yaml.v3");
  assert.equal(yaml.license, "Apache-2.0 AND MIT");
  const securejoin = binary.packages.find((p) => p.name === "github.com/cyphar/filepath-securejoin");
  assert.equal(securejoin.license, "BSD-3-Clause AND MPL-2.0");
  const sys = modules.packages.filter((p) => p.name.startsWith("go.getarcane.app/sys/"));
  for (const p of sys) assert.ok(p.licenseFiles.some((f) => f.path === "repository:sys/LICENSE"));
});

test("every licence term and every unclassified evidence file has explicit approval", async () => {
  const { assertLicense, reviews } = await import("../scripts/arcane-license-policy.mjs");
  for (const p of [...modules.packages, ...binary.packages, ...licenses.packages]) {
    assert.doesNotThrow(() => assertLicense(p.license, p.selectedAlternative), p.name);
    for (const f of [...p.licenseFiles, ...(p.nestedLicenseEvidence ?? [])]) {
      if (f.detectedLicense && f.review?.scope !== "non-code")
        assert.doesNotThrow(() => assertLicense(f.detectedLicense, p.selectedAlternative), `${p.name}/${f.path}`);
      if (!f.detectedLicense || f.review) {
        const r = reviews.find((r) => r.module === (p.module ?? p.name) && r.version === p.version && r.path === f.path);
        assert.ok(r?.reason && r.classification);
        assert.equal(f.sha256, r.sha256);
        assert.equal(f.review.reason, r.reason);
      }
    }
  }
  for (const p of binary.packages) for (const term of p.reportedPackageLicenses)
    assert.ok(p.license.includes(term), `${p.name} omitted ${term}`);
});

test("combined and nested grants include all reviewed module licence families", () => {
  const expected = {
    "github.com/klauspost/compress": "Apache-2.0 AND BSD-3-Clause AND MIT",
    "github.com/emirpasic/gods": "BSD-2-Clause AND ISC",
    "github.com/jackc/pgerrcode": "MIT AND PostgreSQL",
    "go.opentelemetry.io/otel": "Apache-2.0 AND BSD-3-Clause",
    "github.com/wneessen/go-mail": "BSD-3-Clause AND MIT",
    "gotest.tools/v3": "Apache-2.0 AND BSD-3-Clause",
  };
  for (const [name, license] of Object.entries(expected)) assert.equal(binary.packages.find((p) => p.name === name).license, license);
  const mail = binary.packages.find((p) => p.name === "github.com/wneessen/go-mail");
  assert.equal(mail.nestedLicenseEvidence.filter((f) => f.path.startsWith("LICENSES/")).length, 3);
  const paho = binary.packages.find((p) => p.name === "github.com/eclipse/paho.golang");
  assert.ok(paho.copyright.some((s) => s.includes("Contributors to the Eclipse Foundation")));
});

test("libc header exceptions are exact, reasoned and restricted to the SQLite cut", async () => {
  const { reviews } = await import("../scripts/arcane-license-policy.mjs");
  const p = binary.packages.find((p) => p.name === "modernc.org/libc");
  assert.equal(p.compiledLicenseNotices.length, 2);
  for (const f of p.compiledLicenseNotices) {
    const r = reviews.find((r) => r.module === p.name && r.path === f.path);
    assert.equal(f.sha256, r.sha256);
    assert.match(f.license, /LGPL-2.1-or-later/);
    assert.match(f.reason, /section 5/);
    assert.match(f.reason, /#7/);
    assert.equal(f.arrivesOnlyVia, "modernc.org/sqlite");
    assert.deepEqual(f.targets, ["hub", "agent"]);
  }
  assert.match(p.compiledLicenseNotices.find((f) => f.path.startsWith("uuid/")).license, /GPL-3.0-or-later WITH GCC-exception-3.1/);
});

test("offline scanner input and import tree hashes are complete", async () => {
  const { hash } = await import("../scripts/arcane-license-policy.mjs");
  const input = readFileSync(new URL("../docs/provenance/arcane-scanner-input.json", import.meta.url));
  assert.equal(hash(input), modules.tool.reportSha256);
  assert.equal(hash(input), binary.tool.reportSha256);
  assert.equal(hash(input), licenses.tool.reportSha256);
  const rows = JSON.parse(input);
  assert.equal(rows.length, 535);
  assert.ok(rows.every((r) => Object.keys(r).join(",") === "name,license"));
  const imports = modules.packages.filter((p) => p.sourceTree);
  assert.equal(new Set(imports.map((p) => p.name)).size, 14);
  for (const p of imports) assert.match(p.sourceTree.sha1, /^[a-f0-9]{40}$/);
  for (const p of modules.packages.filter((p) => !p.binaryPackageCount && p.declarations.some((d) => d.manifest === "cli/go.mod")))
    assert.deepEqual(p.scope, ["runtime (cli)"]);
  for (const p of licenses.packages) for (const f of p.evidence) {
    assert.match(f.sha256, /^[a-f0-9]{64}$/);
    assert.ok(!/Unknown|\/master\/|\/main\//.test(f.source));
    assert.ok(/\/@v\/[^/]+\.zip$|\/[a-f0-9]{40}(?:\/|$)/.test(f.source));
  }
});
