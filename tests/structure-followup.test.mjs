import test from "node:test";
import assert from "node:assert/strict";
import { sizeProblems, provenanceProblems } from "../scripts/check-structure.mjs";
import { scratch, stage, git, node, ok, fails, writeJSON, rawCommit, invoke, write } from "./helpers.mjs";

const header = "// Source: example/core/file.go\n// Revision: " + "a".repeat(40) +
  "\n// Retrieved: 2026-10-13\n// License: BSD-3-Clause\n// Deviations: None\n";
const content = (count) => header + Array.from({ length: count - 5 }, (_, i) => `// source line ${i}\n`).join("");
const check = (root, base) => node(root, "scripts/check-structure.mjs", "--only", "size", "--base", base);
for (const prefix of ["web/", "api/", "cli/", "deploy/"]) {
  test(`provenance prefix cannot authorize oversized own code: ${prefix}`, (t) => {
    const root = scratch(t);
    const base = rawCommit(root);
    const path = prefix + "own.txt";
    stage(root, path, content(5000));
    writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 5000 }, generated: [] });
    fails(check(root, base), "size-baseline-provenance");
  });
}
for (const name of ["constructor", "__proto__", "toString"]) {
  test(`prototype key has no implicit size or provenance exemption: ${name}`, () => {
    const file = { path: name, content: Buffer.from(content(5000)) };
    const provenance = { prefixes: [], files: [name], sidecars: {}, adoptedPrefixes: [] };
    const policy = { adopted: { [name]: 999999 }, generated: [] };
    const problems = sizeProblems([file], policy, { adopted: {} }, {}, provenance);
    for (const category of ["size-baseline-provenance", "size-baseline-new-entry"])
      assert.equal(problems.some((f) => f.category === category), true);
    assert.equal(sizeProblems([file], { adopted: {}, generated: [] }).some((f) => f.category === "file-size"), true);
    assert.deepEqual(provenanceProblems([file], provenance), []);
    assert.deepEqual(provenanceProblems([file], { ...provenance, files: [] }), []);
  });
}
for (const operation of ["rename", "copy"]) {
  for (const adopted of [false, true]) {
    test(`${operation} preserves ${adopted ? "adopted" : "own"} ceiling`, (t) => {
      const root = scratch(t);
      const old = "backend/original.go";
      const next = "backend/moved.go";
      const initial = adopted ? 5000 : 1000;
      stage(root, old, content(initial));
      if (adopted) stage(root, "scripts/size-baseline.json", JSON.stringify({ adopted: { [old]: initial }, generated: [] }));
      const base = rawCommit(root);
      if (operation === "rename") git(root, "mv", old, next);
      stage(root, next, content(initial + (adopted ? 3000 : 1)));
      const entries = operation === "copy" && adopted ? { [old]: initial } : {};
      writeJSON(root, "scripts/size-baseline.json", { adopted: { ...entries, [next]: initial + (adopted ? 3000 : 1) }, generated: [] });
      fails(check(root, base), adopted ? "size-baseline-growth" : "size-baseline-new-entry");
      fails(check(root, base), "file-size");
      stage(root, next, content(initial));
      writeJSON(root, "scripts/size-baseline.json", { adopted: { ...entries, ...(adopted ? { [next]: initial } : {}) }, generated: [] });
      ok(check(root, base));
    });
  }
}
test("copy cannot regrow below a stale adopted ceiling", (t) => {
  const root = scratch(t);
  stage(root, "backend/original.go", content(3000));
  stage(root, "scripts/size-baseline.json", JSON.stringify({ adopted: { "backend/original.go": 5000 }, generated: [] }));
  const base = rawCommit(root);
  stage(root, "backend/copied.go", content(4000));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/original.go": 5000, "backend/copied.go": 5000 }, generated: [] });
  fails(check(root, base), "file-size");
});
for (const [label, path, bad, good, category] of [
  ["size", "own.txt", "line\n".repeat(1001), "line\n", "file-size"],
  ["assets", "example.txt", Buffer.from([0, 1]), "text\n", "asset-allowlist"],
  ["licence", "LICENSES/BSD-3-Clause-Arcane.txt", "changed\n", null, "licence-integrity"],
  ["provenance", "backend/example.go", "package example\n", header + "package example\n", "provenance-header"],
]) {
  test(`real pre-commit rejects staged ${label} even with a clean working file`, (t) => {
    const root = scratch(t);
    const original = good ?? invoke(root, "git", ["show", ":" + path]).stdout;
    stage(root, path, bad);
    write(root, path, original);
    fails(invoke(root, "git", ["commit", "-m", "fix: synthetic guard test"]), category);
    stage(root, path, original);
    ok(invoke(root, "git", ["commit", "-m", "fix: synthetic guard test"]));
  });
}
test("pre-commit checks staged policies rather than unstaged repairs", (t) => {
  const root = scratch(t);
  const path = "backend/import.go";
  stage(root, path, content(5000));
  stage(root, "scripts/size-baseline.json", JSON.stringify({ adopted: { [path]: 6000 }, generated: [] }));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 5000 }, generated: [] });
  fails(invoke(root, "git", ["commit", "-m", "fix: synthetic staged policy"]), "size-baseline-new-entry");
  git(root, "add", "scripts/size-baseline.json");
  ok(invoke(root, "git", ["commit", "-m", "fix: synthetic staged policy"]));
});

test("root prototype source keeps its base count when copied into adoption roots", (t) => {
  const root = scratch(t);
  stage(root, "__proto__", content(1000));
  const base = rawCommit(root);
  stage(root, "backend/moved.go", content(1001));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/moved.go": 1001 }, generated: [] });
  fails(check(root, base), "size-baseline-new-entry");
});
test("large base permits a genuinely new oversized adoption", (t) => {
  const root = scratch(t);
  for (let i = 0; i < 201; i++) write(root, `docs/base-${i}.txt`, `base ${i}\n`);
  git(root, "add", ".");
  const base = rawCommit(root);
  stage(root, "backend/import.go", content(1001));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/import.go": 1001 }, generated: [] });
  ok(check(root, base));
});
