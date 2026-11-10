import test from "node:test";
import assert from "node:assert/strict";
import { inspectText } from "../scripts/publication-patterns.mjs";
import { ALLOWLIST_PATH, lineHash } from "../scripts/publication-allowlist.mjs";
import { FIXTURES } from "../scripts/publication-fixtures.mjs";
import { scratch, stage, node, ok, fails, rawCommit, readJSON } from "./helpers.mjs";

const flagged = (text) => inspectText(text).some((finding) => finding.category === "network-address");

test("zero-padded spellings of the unspecified IPv6 address are safe", () => {
  for (const text of ["0::/system.slice", "0:0:0:0:0:0:0:0", "::0", "0::", "[0::]:80"]) assert.equal(flagged(text), false, text);
});
test("non-canonical spellings of other addresses are still reported", () => {
  // Split literals keep this file free of the addresses it tests.
  for (const text of ["fe80" + ":0::1", "FE80" + "::1", "::FFFF:" + "192.0.2.1", "::ffff:" + "c000:201", "2001:4860" + ":0:0::8888"])
    assert.equal(flagged(text), true, text);
  for (const text of ["2001:0DB8::1", "0:0:0:0:0:0:0:1"]) assert.equal(flagged(text), false, text);
});

const history = (root) => node(root, "scripts/check-publication.mjs", "--history");
const current = (root) => node(root, "scripts/check-publication.mjs");
const entry = (path, line) => ({ path, category: "network-address", lineSha256: lineHash(line), reason: "Synthetic reviewed exception for the history policy test." });
const policy = (root, extra) => stage(root, ALLOWLIST_PATH, JSON.stringify([...readJSON(root, ALLOWLIST_PATH), ...extra], null, 2) + "\n");
const finding = "Synthetic note at " + FIXTURES["network-address"][0];

test("a historical tree's unused entry does not fail the history scan", (t) => {
  const root = scratch(t);
  const base = readJSON(root, ALLOWLIST_PATH);
  stage(root, "sample.txt", "clean text\n");
  policy(root, [entry("sample.txt", "a line that no longer occurs")]);
  rawCommit(root);
  stage(root, ALLOWLIST_PATH, JSON.stringify(base, null, 2) + "\n");
  rawCommit(root);
  ok(history(root));
});
test("an unused entry in the current tree still fails", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", "clean text\n");
  policy(root, [entry("sample.txt", "a line that does not occur")]);
  rawCommit(root);
  fails(current(root), "stale-publication-allowlist");
  fails(history(root), "stale-publication-allowlist");
});
test("a historical finding without its own entry fails even when the current policy covers it", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", finding + "\n");
  rawCommit(root);
  policy(root, [entry("sample.txt", finding)]);
  rawCommit(root);
  ok(current(root));
  fails(history(root), "network-address");
});
