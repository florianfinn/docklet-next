import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { scratch, write, invoke, ok, ROOT } from "./helpers.mjs";

const mutations = [
  {
    name: "loosened scan growth limit",
    path: "tests/publication-performance.test.mjs",
    mutate: (text) => text.replace("GROWTH_LIMIT = 32;", "GROWTH_LIMIT = 3200;"),
    tests: ["tests/publication-performance.test.mjs"],
    pattern: "scan guard keeps its growth limit",
  },
  {
    name: "bypassed scan growth comparison",
    path: "tests/publication-performance.test.mjs",
    mutate: (text) => text.replace("timing.full < GROWTH_LIMIT * timing.part;", "true;"),
    tests: ["tests/publication-performance.test.mjs"],
    pattern: "growth rule separates",
  },
  {
    name: "case-sensitive credential start",
    path: "scripts/publication-patterns.mjs",
    mutate: (text) => text.replace("(?<![A-Za-z0-9_-])", "(?<![a-z0-9_-])"),
    tests: ["tests/publication-performance.test.mjs"],
    pattern: "JWT upper-case overlap",
  },
  {
    name: "unknown Conventional type",
    path: "scripts/commit-subject.mjs",
    mutate: (text) => text.replace("feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert", ".*"),
    tests: ["tests/subjects.test.mjs"],
  },
  {
    name: "review contents write permission",
    path: ".github/workflows/review-status.yml",
    mutate: (text) => text.replace("contents: read", "contents: write"),
    tests: ["tests/workflow-contracts.test.mjs", "tests/workflow-contracts.mjs"],
  },
  {
    name: "missing access token alternative",
    path: "scripts/publication-assignments.mjs",
    mutate: (text) => text.replace("|token|", "|"),
    tests: ["tests/publication-patterns.test.mjs"],
  },
];
for (const mutation of mutations) {
  test(`regression suite kills mutation: ${mutation.name}`, (t) => {
    const root = scratch(t);
    for (const path of mutation.tests) write(root, path, readFileSync(`${ROOT}/${path}`));
    const args = ["--test", "--test-reporter=tap", ...(mutation.pattern ? ["--test-name-pattern", mutation.pattern] : []), mutation.tests[0]];
    ok(invoke(root, process.execPath, args));
    const original = readFileSync(`${root}/${mutation.path}`, "utf8");
    const changed = mutation.mutate(original);
    assert.notEqual(changed, original);
    write(root, mutation.path, changed);
    const result = invoke(root, process.execPath, args);
    assert.equal(result.status, 1, result.stdout + result.stderr);
    assert.match(result.stdout, /not ok/);
  });
}
