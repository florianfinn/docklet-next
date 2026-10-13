import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { inspectRepository } from "../scripts/check-publication.mjs";
import { inspectStructure } from "../scripts/check-structure.mjs";
import { ROOT, git } from "./helpers.mjs";

test("the tracked repository passes publication and all structural guards", () => {
  assert.deepEqual(inspectRepository(ROOT, { staged: true }), []);
  assert.deepEqual(inspectRepository(ROOT), []);
  assert.deepEqual(inspectStructure(ROOT), []);
});
test("hooks are active and every design document is indexed", () => {
  assert.equal(git(ROOT, "config", "--get", "core.hooksPath"), ".githooks");
  const paths = git(ROOT, "ls-files", "-z").split("\0");
  const index = readFileSync(`${ROOT}/docs/design/README.md`, "utf8");
  for (const path of paths.filter(
    (p) => p.startsWith("docs/design/") && p.endsWith(".md") && !p.endsWith("README.md"),
  )) {
    assert.equal(index.includes(`](${path.slice("docs/design/".length)})`), true, path);
  }
});
