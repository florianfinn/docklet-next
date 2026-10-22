import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { sha256 } from "../scripts/check-structure.mjs";
import { lineHash } from "../scripts/publication-allowlist-core.mjs";
import { scratch, stage, write, git, node, ok, fails, readJSON, writeJSON } from "./helpers.mjs";

test("scratch copies foundation helpers but no product trees or imported test suites", (t) => {
  const source = scratch(t);
  for (const [prefix, count] of [["web", 600], ["backend", 1100], ["types", 1],
    ["cli", 1], ["third_party", 1], ["api", 1], ["deploy", 1], ["future-product", 1], ["LICENSES/tools", 1]])
    for (let i = 0; i < count; i++) write(source, `${prefix}/fixture-${i}.txt`, "synthetic\n");
  write(source, "tests/recursive.test.mjs", 'throw new Error("must not run");\n');
  write(source, "tests/fixtures/product.mjs", "export const data = [];\n");
  write(source, "docs/unrelated.md", "Unrelated product documentation.\n");
  git(source, "add", ".");
  // A tracked product file need not even be readable to build the foundation fixture.
  rmSync(`${source}/web/fixture-0.txt`);
  const root = scratch(t, source);
  const paths = git(root, "ls-files", "-z").split("\0").filter(Boolean);
  assert.deepEqual(paths, git(scratch(t), "ls-files", "-z").split("\0").filter(Boolean));
  for (const path of ["web", "backend", "types", "cli", "third_party", "api", "deploy",
    "future-product", "LICENSES/tools", "docs/unrelated.md", "tests/recursive.test.mjs", "tests/fixtures/product.mjs"])
    assert.equal(existsSync(`${root}/${path}`), false, path);
  for (const path of ["tests/helpers.mjs", "tests/workflow-contracts.mjs", "scripts/pre-push.mjs",
    ".githooks/pre-push", ".github/workflows/checks.yml", "package.json", "pnpm-lock.yaml",
    "LICENSE", "NOTICE", "LICENSES/BSD-3-Clause-Arcane.txt", "docs/design/publication-policy.md"])
    assert.equal(paths.includes(path), true, path);
  assert.deepEqual(paths.filter((path) => path.endsWith(".test.mjs")), ["tests/smoke.test.mjs"]);
  ok(node(root, "scripts/run-checks.mjs", "test"));
});

test("scratch scopes exact policies to copied files and preserves their records and prefix rules", (t) => {
  const source = scratch(t);
  const asset = { path: "LICENSES/example.svg", sha256: sha256("<svg />\n"), source: "synthetic",
    license: "Apache-2.0", purpose: "Synthetic guard fixture" };
  stage(source, asset.path, "<svg />\n");
  const line = 'const ' + 'password' + ' = "' + 'synthetic-value";';
  const allowance = { path: "scripts/example.txt", category: "literal-secret", sha256: lineHash(line),
    reason: "Synthetic public guard input." };
  stage(source, allowance.path, line + "\n");
  writeJSON(source, "scripts/assets.json", [asset, { ...asset, path: "web/icon.svg" }]);
  writeJSON(source, "scripts/publication-allowlist.json", [allowance, { ...allowance, path: "backend/example.go" }]);
  const provenance = readJSON(source, "scripts/provenance.json");
  const original = structuredClone(provenance);
  provenance.files.push("backend/example.go");
  provenance.sidecars["web/package.json"] = provenance.sidecars[".claude/settings.json"];
  provenance.snapshots = { "web/src/": { source: "synthetic/snapshot" } };
  writeJSON(source, "scripts/provenance.json", provenance);
  writeJSON(source, "scripts/size-baseline.json", {
    adopted: { "backend/example.go": 1200 }, generated: ["pnpm-lock.yaml", "web/pnpm-lock.yaml"],
  });
  const license = readJSON(source, "scripts/license-policy.json");
  stage(source, "LICENSES/copy.txt", readFileSync(`${source}/${license.source}`));
  license.artifactCopies = ["LICENSES/copy.txt", "backend/dist/LICENSE"];
  writeJSON(source, "scripts/license-policy.json", license);
  const root = scratch(t, source);
  assert.deepEqual(readJSON(root, "scripts/assets.json"), [asset]);
  assert.deepEqual(readJSON(root, "scripts/publication-allowlist.json"), [allowance]);
  assert.deepEqual(readJSON(root, "scripts/provenance.json"), { ...original, snapshots: {} });
  assert.deepEqual(readJSON(root, "scripts/size-baseline.json"), { adopted: {}, generated: ["pnpm-lock.yaml"] });
  assert.deepEqual(readJSON(root, "scripts/license-policy.json"), { ...license, artifactCopies: ["LICENSES/copy.txt"] });
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  ok(node(root, "scripts/check-structure.mjs", "--staged"));
  assert.deepEqual(readJSON(source, "scripts/provenance.json"), provenance);
  stage(root, "backend/new.go", "package example\n");
  fails(node(root, "scripts/check-structure.mjs", "--staged"), "provenance-header");
});
