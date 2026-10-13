import test from "node:test";
import assert from "node:assert/strict";
import { lineHash, reviewedFindings, ALLOWLIST_PATH } from "../scripts/publication-allowlist.mjs";
import { inspectText } from "../scripts/publication-patterns.mjs";
import { scratch, stage, node, ok, fails, rawCommit, write } from "./helpers.mjs";

const path = "backend/import.go";
const line = ['const token', ' = ', '"synthetic-value"'].join("");
const entry = { path, category: "literal-secret", sha256: lineHash(line), reason: "Synthetic public protocol label, reviewed with imported code." };
const scan = (text, file = path) => inspectText(text, file, [], { detailed: true });
const policy = (root, entries) => stage(root, ALLOWLIST_PATH, JSON.stringify(entries));

test("reviewed entry consumes exactly one finding, never a category or directory", () => {
  assert.deepEqual(reviewedFindings([entry], scan(line)), []);
  assert.equal(reviewedFindings([entry], scan(line + "\n" + line)).length, 1);
  assert.equal(reviewedFindings([entry], scan(line + "\n" + line.replace("value", "other"))).length, 1);
  assert.equal(reviewedFindings([entry], scan(line, "backend/other.go")).length, 2);
});
test("line hash binds all whitespace and normalizes only line endings", () => {
  assert.equal(lineHash("x\r\ny"), lineHash("x\ny"));
  assert.notEqual(lineHash(line), lineHash(" " + line));
  assert.notEqual(lineHash(line), lineHash(line + " "));
  assert.deepEqual(scan(line + "\r\n"), scan(line + "\n"));
});
for (const [label, text, file] of [
  ["changed line", line.replace("value", "other"), path],
  ["stale entry", "package example", path],
  ["path mismatch", line, "backend/other.go"],
]) test(`real CLI rejects ${label}`, (t) => {
  const root = scratch(t);
  stage(root, file, text);
  policy(root, [entry]);
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "stale-publication-allowlist");
  if (label !== "stale entry") fails(node(root, "scripts/check-publication.mjs", "--staged"), "literal-secret");
});
test("real CLI uses staged policy and historical policy from each tree", (t) => {
  const root = scratch(t);
  stage(root, path, line);
  policy(root, [entry]);
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  const base = rawCommit(root);
  policy(root, []);
  write(root, ALLOWLIST_PATH, JSON.stringify([entry]));
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "literal-secret");
  policy(root, [entry]);
  stage(root, path, line + "\n" + line);
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "literal-secret");
  stage(root, path, "package example");
  policy(root, []);
  rawCommit(root);
  ok(node(root, "scripts/check-publication.mjs", "--history"));
  ok(node(root, "scripts/check-publication.mjs", "--history", base));
});
for (const change of [{ path: "backend/*" }, { path: "backend/" }, { path: "../import.go" },
  { reason: "" }, { sha256: "0" }, { category: "operator-denylist" }, { extra: true }])
  test(`invalid reviewed exception ${JSON.stringify(change)}`, () => assert.throws(() => reviewedFindings([{ ...entry, ...change }], [])));
test("duplicate exceptions fail", () => assert.throws(() => reviewedFindings([entry, entry], scan(line))));
test("operator denylist cannot be suppressed", () => {
  const findings = inspectText(line, path, ["synthetic-value"], { detailed: true });
  assert.equal(reviewedFindings([entry], findings)[0].category, "operator-denylist");
});
