import test from "node:test";
import assert from "node:assert/strict";
import { lineHash, ALLOWLIST_PATH } from "../scripts/publication-allowlist.mjs";
import { scratch, stage, node, ok, fails, write } from "./helpers.mjs";

const path = "backend/import.go";
const line = ['const token', ' = ', '"synthetic-value"'].join("");
const propose = (root, ...prefixes) => node(root, "scripts/publication-allowlist.mjs", "--propose", ...prefixes);

test("proposal aggregates current findings, scopes paths, deduplicates prefixes and leaks no values", (t) => {
  const root = scratch(t);
  stage(root, path, line);
  write(root, path, [line, line, line].join("\n"));
  stage(root, "types/import.go", line);
  stage(root, "backend-extra/import.go", line);
  stage(root, "web/import.ts", line);
  const result = propose(root, "backend/", path, "backend", "types");
  const proposed = JSON.parse(ok(result));
  assert.deepEqual(proposed, [
    { path, category: "literal-secret", lineSha256: lineHash(line), count: 3, reason: "TODO" },
    { path: "types/import.go", category: "literal-secret", lineSha256: lineHash(line), count: 1, reason: "TODO" },
  ]);
  assert.equal(result.stderr, "");
  assert.equal(result.stdout.includes("synthetic-value"), false);
  assert.equal(result.stdout.includes(line), false);
  assert.deepEqual(JSON.parse(ok(propose(root, path))), [proposed[0]]);
  assert.deepEqual(JSON.parse(ok(propose(root, "absent"))), []);
  assert.deepEqual(JSON.parse(ok(propose(root, "backend"))), [proposed[0]]);
});
test("proposal requires reviewed reasons before it can round-trip through the check", (t) => {
  const root = scratch(t);
  stage(root, path, [line, line].join("\n"));
  const proposed = JSON.parse(ok(propose(root, "backend")));
  stage(root, ALLOWLIST_PATH, JSON.stringify(proposed));
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "check-error");
  const reviewed = proposed.map((entry) => ({ ...entry, reason: "Reviewed synthetic public fixture." }));
  stage(root, ALLOWLIST_PATH, JSON.stringify(reviewed));
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  assert.deepEqual(JSON.parse(ok(propose(root, "backend"))), proposed);
});
test("proposal never creates operator-denylist exemptions", (t) => {
  const root = scratch(t);
  stage(root, path, line);
  write(root, ".publication-denylist.local.json", JSON.stringify(["synthetic-value"]));
  const proposed = JSON.parse(ok(propose(root, "backend")));
  assert.deepEqual(proposed.map((entry) => entry.category), ["literal-secret"]);
  stage(root, ALLOWLIST_PATH, JSON.stringify(proposed.map((entry) => ({ ...entry, reason: "Synthetic fixture." }))));
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "operator-denylist");
});
test("proposal fails without partial output or unsafe filenames on stdout or stderr", (t) => {
  const root = scratch(t);
  stage(root, path, line);
  const privatePath = "backend/" + ["private", "lan"].join(".");
  stage(root, privatePath, line);
  const result = propose(root, "backend");
  fails(result, "check-error");
  assert.equal(result.stdout, "");
  assert.equal(result.stderr.includes(privatePath), false);
  assert.equal(result.stderr.includes("synthetic-value"), false);
});
for (const args of [[], ["--propose"], ["--unknown", "backend"], ["--propose", "../backend"],
  ["--propose", "/backend"], ["--propose", "backend/*"], ["--propose", ""], ["--propose", "backend//"]])
  test(`proposal rejects invalid arguments ${JSON.stringify(args)}`, (t) => {
    const root = scratch(t);
    const result = node(root, "scripts/publication-allowlist.mjs", ...args);
    fails(result, "check-error");
    assert.equal(result.stdout, "");
  });
