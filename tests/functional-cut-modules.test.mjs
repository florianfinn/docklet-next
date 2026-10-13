import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const root = new URL("../", import.meta.url);
const manifest = JSON.parse(readFileSync(new URL("../docs/provenance/removed-modules.json", import.meta.url), "utf8"));

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

test("removed Go requirements match the baseline and tidied manifests", () => {
  const removed = [];
  const directRemoved = [];
  const before = new Set();
  const after = new Set();
  for (const path of manifest.comparedManifests) {
    const previous = requirements(execFileSync("git", ["show", `${manifest.baselineRevision}:${path}`], { cwd: root, encoding: "utf8" }));
    const current = requirements(readFileSync(new URL(`../${path}`, import.meta.url), "utf8"));
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
