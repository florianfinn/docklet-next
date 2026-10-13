import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";

const root = new URL("../", import.meta.url);
const manifest = JSON.parse(readFileSync(new URL("../docs/provenance/removed-feature-search.json", import.meta.url), "utf8"));
const provenance = /:\d+:\s*\/\/ (?:Source|Revision|Retrieved|License|Deviations):/;

function search(pattern, paths, globs = []) {
  const args = ["--no-heading", "--line-number", "--color", "never", "--ignore-case"];
  for (const glob of globs) args.push("--glob", glob);
  args.push("--regexp", pattern, ...paths);
  const result = spawnSync("rg", args, { cwd: root, encoding: "utf8" });
  if (result.error) throw result.error;
  assert.ok(result.status === 0 || result.status === 1, result.stderr);
  return result.stdout.trim().split("\n").filter(Boolean).filter((line) => !provenance.test(line));
}

test("removed feature identifiers have expected-empty source and configuration searches", () => {
  for (const query of manifest.queries) {
    assert.deepEqual(search(query.pattern, manifest.scope, manifest.globs), query.expectedMatches, query.feature);
  }
  for (const query of manifest.environment.queries) {
    assert.deepEqual(search(query.pattern, manifest.environment.scope), query.expectedMatches, query.feature);
  }
});
