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

for (const count of [0, -1, 1.5, "2", null, true, Number.MAX_SAFE_INTEGER + 1, NaN, Infinity, undefined])
  test(`invalid occurrence count ${JSON.stringify(count)}`, () => {
    assert.throws(() => reviewedFindings([{ ...entry, count }], scan(line)));
  });
for (const reason of ["TODO", " TODO ", "   "])
  test(`unreviewed reason ${JSON.stringify(reason)} fails the real CLI`, (t) => {
    const root = scratch(t);
    stage(root, path, line);
    policy(root, [{ ...entry, reason }]);
    fails(node(root, "scripts/check-publication.mjs", "--staged"), "check-error");
  });
test("count requires exact multiplicity with no unused allowance", (t) => {
  const root = scratch(t);
  policy(root, [{ ...entry, count: 2 }]);
  stage(root, path, [line, line].join("\n"));
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  stage(root, path, line);
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "stale-publication-allowlist");
  stage(root, path, "package example");
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "stale-publication-allowlist");
  stage(root, path, [line, line, line].join("\n"));
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "literal-secret");
});
test("explicit one and omitted count are equivalent", () => {
  for (const text of [line, [line, line].join("\n"), "package example"])
    assert.deepEqual(reviewedFindings([{ ...entry, count: 1 }], scan(text)), reviewedFindings([entry], scan(text)));
});
test("count is per finding even when a line matches twice", () => {
  const text = line + "; " + line;
  const findings = scan(text);
  assert.equal(findings.length, 2);
  assert.deepEqual(reviewedFindings([{ ...entry, sha256: lineHash(text), count: 2 }], findings), []);
  assert.deepEqual(reviewedFindings([{ ...entry, sha256: lineHash(text), count: 1 }], findings), [{ path, category: entry.category }]);
});
test("canonical lineSha256 accepts exact counts and shares duplicate identity with sha256", () => {
  const { sha256, ...rest } = entry;
  const canonical = { ...rest, lineSha256: sha256, count: 2 };
  assert.deepEqual(reviewedFindings([canonical], scan([line, line].join("\n"))), []);
  assert.throws(() => reviewedFindings([canonical, { ...canonical, count: 1 }], scan(line)));
  assert.throws(() => reviewedFindings([entry, canonical], scan(line)));
  assert.throws(() => reviewedFindings([{ ...canonical, sha256 }], scan(line)));
  assert.throws(() => reviewedFindings([{ ...rest, count: 1 }], scan(line)));
});
test("count cannot cross path, category or line-hash boundaries", () => {
  for (const extra of [{ path: "backend/other.go" }, { category: "credential-pattern" }, { sha256: lineHash("different") }]) {
    const findings = [{ ...entry }, { ...entry, ...extra }];
    const result = reviewedFindings([{ ...entry, count: 2 }], findings);
    assert.deepEqual(result, [
      { path: findings[1].path, category: findings[1].category },
      { path: ALLOWLIST_PATH, category: "stale-publication-allowlist" },
    ]);
  }
});
test("count remains bound to each staged and historical tree", (t) => {
  const root = scratch(t);
  stage(root, path, [line, line].join("\n"));
  policy(root, [{ ...entry, count: 1 }]);
  rawCommit(root);
  policy(root, [{ ...entry, count: 2 }]);
  rawCommit(root);
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  fails(node(root, "scripts/check-publication.mjs", "--history"), "literal-secret");
  write(root, ALLOWLIST_PATH, JSON.stringify([{ ...entry, count: 3 }]));
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  fails(node(root, "scripts/check-publication.mjs", "--worktree"), "stale-publication-allowlist");
});
