import test from "node:test";
import assert from "node:assert/strict";
import { scratch, write, git, rawCommit, writeJSON, invoke, ok, fails } from "./helpers.mjs";

const header = "// Source: example/core/file.go\n// Revision: " + "a".repeat(40) +
  "\n// Retrieved: 2026-10-13\n// License: BSD-3-Clause\n// Deviations: None\n";
const content = (count, label) => header + Array.from({ length: count - 5 }, (_, i) => `// ${label} source line ${i}\n`).join("");
const check = (root, base, staged) => invoke(root, process.execPath, ["scripts/check-structure.mjs", "--only", "size", "--base", base,
  ...(staged ? ["--staged"] : [])], { timeout: 10000 });
test("1200-file base accepts a first 950-file adoption with oversized files", (t) => {
  const root = scratch(t);
  const existing = git(root, "ls-files", "-z").split("\0").filter(Boolean).length;
  for (let i = existing; i < 1200; i++) write(root, `docs/base-${i}.txt`, `base ${i}\n`);
  git(root, "add", ".");
  const base = rawCommit(root);
  assert.equal(git(root, "ls-tree", "-r", "--name-only", base).split("\n").length, 1200);
  const adopted = {};
  for (let i = 0; i < 950; i++) {
    const path = `backend/import-${i}.go`;
    const count = i < 5 ? 1500 + i * 100 : 10;
    write(root, path, content(count, `import-${i}`));
    if (count > 1000) adopted[path] = count;
  }
  writeJSON(root, "scripts/size-baseline.json", { adopted, generated: [] });
  git(root, "add", ".");
  for (const staged of [false, true]) {
    const start = performance.now();
    ok(check(root, base, staged));
    t.diagnostic(`1200 base + 950 import (${staged ? "staged" : "worktree"}): ${(performance.now() - start).toFixed(1)} ms`);
  }
});
for (const operation of ["rename", "copy"])
  test(`large import ${operation} retains source ceiling and base count`, (t) => {
    const root = scratch(t);
    for (let i = 0; i < 1200; i++) write(root, `docs/base-${i}.txt`, `base ${i}\n`);
    const old = "backend/original.go", next = "backend/moved.go";
    write(root, old, content(1500, "original"));
    writeJSON(root, "scripts/size-baseline.json", { adopted: { [old]: 2000 }, generated: [] });
    git(root, "add", ".");
    const base = rawCommit(root);
    if (operation === "rename") git(root, "mv", old, next);
    for (let i = 0; i < 950; i++) write(root, `backend/new-${i}.go`, content(10, `new-${i}`));
    const retained = operation === "copy" ? { [old]: 2000 } : {};
    for (const [count, ceiling, category] of [[1600, 2100, "size-baseline-growth"], [1600, 2000, "file-size"], [1500, 1500, null]]) {
      write(root, next, content(count, "original"));
      writeJSON(root, "scripts/size-baseline.json", { adopted: { ...retained, [next]: ceiling }, generated: [] });
      git(root, "add", ".");
      for (const staged of [false, true]) {
        const start = performance.now();
        const result = check(root, base, staged);
        if (category) fails(result, category); else ok(result);
        t.diagnostic(`${operation} ${category ?? "green"} (${staged ? "staged" : "worktree"}): ${(performance.now() - start).toFixed(1)} ms`);
      }
    }
  });
for (const initial of [999, 50])
  test(`own ${initial}-line source cannot reset its ceiling on adoption`, (t) => {
    const root = scratch(t);
    const old = "own.txt", next = "backend/import.go";
    const source = content(initial, "original".repeat(initial === 50 ? 100 : 1));
    write(root, old, source);
    git(root, "add", ".");
    const base = rawCommit(root);
    write(root, next, source + "x\n".repeat(1001 - initial));
    writeJSON(root, "scripts/size-baseline.json", { adopted: { [next]: 1001 }, generated: [] });
    git(root, "add", ".");
    for (const staged of [false, true]) fails(check(root, base, staged), "size-baseline-new-entry");
  });
