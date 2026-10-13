import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { sha256 } from "../scripts/check-structure.mjs";
import { scratch, stage, write, git, invoke, ok, fails, rawCommit, readJSON, writeJSON,
  stubModuleChecks } from "./helpers.mjs";

const record = {
  source: "example/modules", revision: "a".repeat(40), retrieved: "2026-10-13",
  license: "Apache-2.0", deviations: "Synthetic module dispatch fixture.",
};
function modules(t) {
  const root = scratch(t);
  const env = stubModuleChecks(root);
  const provenance = readJSON(root, "scripts/provenance.json");
  for (const [path, content] of [["backend/go.mod", "module example.com/synthetic\n"],
    ["web/package.json", '{"name":"synthetic","private":true}\n']]) {
    stage(root, path, content);
    provenance.sidecars[path] = { ...record, sha256: sha256(content) };
  }
  writeJSON(root, "scripts/provenance.json", provenance);
  git(root, "add", ".");
  const head = rawCommit(root);
  return { root, env, head, input: `refs/heads/main ${head} refs/heads/main ${"0".repeat(40)}\n` };
}
const calls = (root) => readFileSync(`${root}/.git/module-checks.log`, "utf8").trim().split("\n").map(JSON.parse);
const expected = [
  { program: "go", args: ["vet", "./..."], cwd: "backend" },
  { program: "pnpm", args: ["--dir", "web", "run", "lint"], cwd: "" },
  { program: "go", args: ["test", "./..."], cwd: "backend" },
  { program: "pnpm", args: ["--dir", "web", "run", "test"], cwd: "" },
];

test("real pre-push invokes Go and web lint and tests when synthetic modules exist", (t) => {
  const { root, env, input } = modules(t);
  ok(invoke(root, "sh", [".githooks/pre-push"], { env, input }));
  assert.deepEqual(calls(root), expected);
});

for (const [index, command] of ["go vet ./...", "pnpm --dir web run lint",
  "go test ./...", "pnpm --dir web run test"].entries()) {
  test(`real pre-push propagates module failure: ${command}`, (t) => {
    const { root, env, input } = modules(t);
    fails(invoke(root, "sh", [".githooks/pre-push"], {
      input, env: { ...env, MODULE_CHECK_FAIL: command },
    }), "check-error");
    assert.deepEqual(calls(root), expected.slice(0, index + 1));
  });
}

test("text-only pre-push skips module commands even when modules exist", (t) => {
  const { root, env, head: base } = modules(t);
  git(root, "update-ref", "refs/remotes/origin/main", base);
  stage(root, "docs/example.md", "Example documentation.\n");
  const head = rawCommit(root);
  write(root, ".git/module-checks.log", "");
  ok(invoke(root, "sh", [".githooks/pre-push"], {
    env, input: `refs/heads/main ${head} refs/heads/main ${base}\n`,
  }));
  assert.equal(readFileSync(`${root}/.git/module-checks.log`, "utf8"), "");
});
