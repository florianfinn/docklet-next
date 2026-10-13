import test from "node:test";
import assert from "node:assert/strict";
import { lineCount } from "../scripts/check-structure.mjs";
import { scratch, stage, git, node, ok, fails, writeJSON, rawCommit } from "./helpers.mjs";

const header =
  "// Source: example/core/file.go\n// Revision: " +
  "a".repeat(40) +
  "\n// Retrieved: 2026-10-13\n// License: BSD-3-Clause\n// Deviations: None\n";
const check = (root, ...args) => node(root, "scripts/check-structure.mjs", "--only", "size", ...args);
for (const ending of ["\n", "\r\n", "\r"]) {
  test(`line boundary and first adoption with ${JSON.stringify(ending)}`, (t) => {
    assert.equal(lineCount(Buffer.from("")), 0);
    assert.equal(lineCount(Buffer.from("a")), 1);
    assert.equal(lineCount(Buffer.from(`a${ending}b`)), 2);
    const root = scratch(t);
    const base = rawCommit(root);
    stage(root, "own.txt", ("x" + ending).repeat(1001));
    fails(check(root), "file-size");
    stage(root, "own.txt", ("x" + ending).repeat(1000));
    ok(check(root));
    const path = "backend/import.go";
    stage(root, path, (header + "x\n".repeat(4995)).replaceAll("\n", ending));
    writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 5000 }, generated: [] });
    ok(check(root, "--base", base));
    ok(check(root));
    writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 5001 }, generated: [] });
    fails(check(root, "--base", base), "size-baseline-new-entry");
    writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 4999 }, generated: [] });
    fails(check(root, "--base", base), "size-baseline-new-entry");
  });
}
test("first adoption checks path, header, newness and exact size together", (t) => {
  const root = scratch(t);
  const base = rawCommit(root);
  const path = "own.txt";
  stage(root, path, header + "x\n".repeat(4995));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 5000 }, generated: [] });
  fails(check(root, "--base", base), "size-baseline-provenance");
  git(root, "rm", "-f", path);
  stage(root, "backend/import.go", "x\n".repeat(5000));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/import.go": 5000 }, generated: [] });
  fails(check(root, "--base", base), "size-baseline-provenance");
  stage(root, "backend/import.go", header + "x\n".repeat(4995));
  ok(check(root, "--base", base));
  git(root, "add", ".");
  const adopted = rawCommit(root);
  stage(root, "backend/import.go", header + "x\n".repeat(4996));
  fails(check(root, "--base", adopted), "file-size");
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/import.go": 5001 }, generated: [] });
  fails(check(root, "--base", adopted), "size-baseline-growth");
});
for (const path of ["big.txt", "own/generated.txt", "api/generated/example.ts"]) {
  test(`arbitrary generated exemption rejects ${path}`, (t) => {
    const root = scratch(t);
    const base = rawCommit(root);
    const content = path.startsWith("api/") ? "x\n".repeat(5000) : "// Generated\n" + "x\n".repeat(4999);
    stage(root, path, content);
    writeJSON(root, "scripts/size-baseline.json", { adopted: {}, generated: [path] });
    fails(check(root, "--base", base), "generated-new-entry");
    fails(check(root), "generated-new-entry");
    git(root, "rm", "-f", path);
    fails(check(root), "stale-generated-entry");
  });
}
for (const path of [
  "pnpm-lock.yaml",
  "web/package-lock.json",
  "backend/go.sum",
  "yarn.lock",
  "api/generated/dto.ts",
  "web/src/generated/dto.ts",
]) {
  test(`reviewed generated class accepts ${path}`, (t) => {
    const root = scratch(t);
    const base = rawCommit(root);
    stage(root, path, "// Generated from schema\n" + "x\n".repeat(4999));
    writeJSON(root, "scripts/size-baseline.json", { adopted: {}, generated: [path] });
    ok(check(root, "--base", base));
    writeJSON(root, "scripts/size-baseline.json", { adopted: {}, generated: [] });
    fails(check(root, "--base", base), "file-size");
  });
}
