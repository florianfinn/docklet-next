import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, rmSync } from "node:fs";
import { resolve } from "node:path";
import { sha256 } from "../scripts/check-structure.mjs";
import { ROOT, scratch, write, git, invoke, ok, fails, rawCommit, stubModuleChecks,
  readJSON } from "./helpers.mjs";

test("repository lint rejects drift in the real imported workspace before module tools run", async (t) => {
  const root = scratch(t);
  // This integration test deliberately needs the imported tree and its exact
  // policies; generic guard fixtures remain foundation-only.
  const paths = git(ROOT, "ls-files", "-z").split("\0").filter(Boolean);
  for (const path of paths) write(root, path, readFileSync(resolve(ROOT, path)));
  git(root, "add", "-f", "--", ...paths);
  rawCommit(root);
  const env = stubModuleChecks(root);
  const lint = () => invoke(root, process.execPath, ["scripts/run-checks.mjs", "lint"], { env });
  ok(lint());
  const log = resolve(root, ".git/module-checks.log");
  const calls = readFileSync(log, "utf8").trim().split("\n").map(JSON.parse);
  assert.equal(calls.filter((call) => call.args[0] === "vet").length,
    paths.filter((path) => path.endsWith("/go.mod")).length);
  assert.equal(calls.filter((call) => call.args[1] === "./scripts/gofmt-check.go").length, 1);

  for (const path of ["backend/go.mod", "backend/go.sum", "go.work", "go.work.sum"]) {
    await t.test(`rejects changed ${path}`, () => {
      const original = readFileSync(resolve(root, path));
      const provenance = readFileSync(resolve(root, "scripts/provenance.json"));
      try {
        write(root, ".git/module-checks.log", "");
        const changed = Buffer.concat([original, Buffer.from("\n")]);
        write(root, path, changed);
        // A refreshed provenance sidecar must not excuse a stale inventory.
        const policy = readJSON(root, "scripts/provenance.json");
        if (policy.sidecars[path]) {
          policy.sidecars[path].sha256 = sha256(changed);
          write(root, "scripts/provenance.json", JSON.stringify(policy) + "\n");
        }
        const result = lint();
        fails(result, "check-error");
        assert.match(result.stderr, /core-inventory-lockfile-drift/);
        assert.equal(readFileSync(log, "utf8"), "");
      } finally {
        write(root, path, original);
        write(root, "scripts/provenance.json", provenance);
      }
    });
  }
  await t.test("rejects a missing inventory", () => {
    const path = "docs/provenance/core-import.json";
    const original = readFileSync(resolve(root, path));
    try {
      rmSync(resolve(root, path));
      fails(lint(), "check-error");
    } finally {
      write(root, path, original);
    }
  });
  await t.test("rejects adopted licence drift in the inventory check and repository lint", () => {
    const path = "third_party/arcane/acfs/LICENSE";
    const original = readFileSync(resolve(root, path));
    try {
      write(root, path, Buffer.concat([original, Buffer.from("\n")]));
      const result = invoke(root, process.execPath, ["scripts/inventory-core.mjs", "--check"], { env });
      assert.equal(result.status, 1);
      assert.match(result.stderr, /adopted-license-drift/);
      fails(lint());
    } finally {
      write(root, path, original);
    }
  });
});
