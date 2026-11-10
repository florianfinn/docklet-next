import test from "node:test";
import { lineHash } from "../scripts/publication-allowlist.mjs";
import { COMMIT_ALLOWLIST_PATH } from "../scripts/check-publication.mjs";
import { FIXTURES } from "../scripts/publication-fixtures.mjs";
import { scratch, node, ok, fails, rawCommit, write } from "./helpers.mjs";

const line = "Synthetic note at " + FIXTURES["network-address"][0];
const message = "test: synthetic history\n\n" + line;
const entry = (commit, text = line) => ({
  path: `commit/${commit}`, category: "network-address", lineSha256: lineHash(text),
  reason: "Synthetic reviewed commit-message exception for the guard test.",
});
const policy = (root, entries) => write(root, COMMIT_ALLOWLIST_PATH, JSON.stringify(entries));
const history = (root, ...revisions) => node(root, "scripts/check-publication.mjs", "--history", ...revisions);

test("a published commit message finding fails without an exception", (t) => {
  const root = scratch(t);
  rawCommit(root, message);
  fails(history(root), "network-address");
});
test("an exact exception for the scanned commit and line clears the message", (t) => {
  const root = scratch(t);
  const commit = rawCommit(root, message);
  policy(root, [entry(commit)]);
  ok(history(root));
});
test("a changed line or another commit is not covered and the entry goes stale", (t) => {
  const root = scratch(t);
  const commit = rawCommit(root, message);
  policy(root, [entry(commit, line + " ")]);
  fails(history(root), "network-address");
  fails(history(root), "stale-publication-allowlist");
  const other = rawCommit(root, message);
  policy(root, [entry(commit)]);
  fails(history(root, other), "network-address");
});
test("an entry for a commit outside the scanned range waits instead of going stale", (t) => {
  const root = scratch(t);
  const clean = rawCommit(root);
  const flagged = rawCommit(root, message);
  policy(root, [entry(flagged)]);
  ok(history(root, clean));
  ok(history(root));
});
test("malformed commit exceptions are refused", (t) => {
  const root = scratch(t);
  const commit = rawCommit(root);
  for (const bad of [{ ...entry(commit), path: "scripts/file.json" }, { ...entry(commit), path: `commit/${commit.slice(0, 12)}` }]) {
    policy(root, [bad]);
    fails(history(root), "check-error");
  }
});
