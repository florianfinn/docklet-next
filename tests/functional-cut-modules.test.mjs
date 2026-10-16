import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";

// The functional-cut evidence predates the separately approved SQLite restoration.
const cutRevision = "1ead6d2c7e17c79b46c42eb8567c3a1d70730fd5";

const manifest = JSON.parse(readFileSync(new URL("../docs/provenance/removed-modules.json", import.meta.url), "utf8"));
const baseline = JSON.parse(readFileSync(new URL("./fixtures/pre-cut-go-requirements.json", import.meta.url), "utf8"));

function requirements(body) {
  const result = new Map();
  let block = false;
  for (let line of body.split("\n")) {
    line = line.trim();
    if (line === "require (") { block = true; continue; }
    if (block && line === ")") { block = false; continue; }
    if (line.startsWith("require ")) line = line.slice(8);
    else if (!block) continue;
    const [name, version] = line.split(/\s+/);
    if (/^v\d/.test(version ?? "")) result.set(name, { version, direct: !line.includes("// indirect") });
  }
  return result;
}

test("historical removed Go requirements match the functional-cut manifests", () => {
  assert.equal(baseline.sourceRevision, manifest.baselineRevision);
  assert.deepEqual(Object.keys(baseline.requirements).sort(), [...manifest.comparedManifests].sort());
  const removed = [];
  const directRemoved = [];
  const before = new Set();
  const after = new Set();
  for (const path of manifest.comparedManifests) {
    const previous = new Map(baseline.requirements[path].map(([name, version, direct]) => [name, { version, direct }]));
    assert.equal(previous.size, baseline.requirements[path].length, `duplicate baseline requirement: ${path}`);
    const current = requirements(execFileSync("git", ["show", `${cutRevision}:${path}`], { encoding: "utf8" }));
    for (const name of previous.keys()) before.add(name);
    for (const name of current.keys()) after.add(name);
    for (const [name, value] of [...previous].sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)) {
      if (!current.has(name)) removed.push({ manifest: path, module: name, version: value.version, wasDirect: value.direct });
      if (value.direct && !current.get(name)?.direct) directRemoved.push({ manifest: path, module: name, removedCompletely: !current.has(name) });
    }
  }
  assert.deepEqual(removed, manifest.removedRequirements);
  assert.deepEqual(directRemoved, manifest.removedDirectRequirements);
  assert.deepEqual([...before].filter((name) => !after.has(name)).sort(), manifest.removedModulePaths);
  for (const name of manifest.remainingIndirectSqliteRequirements) {
    assert.ok(after.has(name), `documented indirect requirement disappeared: ${name}`);
  }
});

test("current requirements retain the functional cut except the approved SQLite restoration", () => {
  const current = new Set(manifest.comparedManifests.flatMap((path) =>
    [...requirements(readFileSync(new URL(`../${path}`, import.meta.url), "utf8")).keys()]));
  assert.deepEqual(manifest.removedModulePaths.filter((name) => current.has(name)), [
    "github.com/dustin/go-humanize", "github.com/libtnb/sqlite",
    "github.com/ncruces/go-strftime", "github.com/remyoudompheng/bigfft",
    "modernc.org/libc", "modernc.org/mathutil", "modernc.org/memory",
  ]);
});
