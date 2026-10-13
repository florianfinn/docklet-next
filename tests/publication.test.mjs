import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { inspectText, inspectPath, publicIdentity } from "../scripts/publication-patterns.mjs";
import { FIXTURES } from "../scripts/publication-fixtures.mjs";
import { scratch, stage, write, git, node, ok, fails, rawCommit, writeJSON, invoke } from "./helpers.mjs";

for (const [category, values] of Object.entries(FIXTURES)) {
  for (const [index, value] of values.entries())
    test(`publication rejects ${category} fixture ${index + 1}`, (t) => {
      const root = scratch(t);
      stage(root, "sample.txt", value);
      const result = node(root, "scripts/check-publication.mjs", "--staged");
      fails(result, category);
      assert.equal(result.stderr, `sample.txt [${category}]\n`);
      assert.equal(result.stderr.includes(value), false);
      stage(root, "sample.txt", "Synthetic fixture at example.com and 192.0.2.1, 2001:db8::1\n");
      ok(node(root, "scripts/check-publication.mjs", "--staged"));
    });
}
const cases = [
  ["credential-pattern", "ghp_" + "A".repeat(36)],
  ["credential-pattern", "github_pat_" + "A".repeat(36)],
  ["credential-pattern", "AKIA" + "A".repeat(16)],
  ["credential-pattern", "-----BEGIN " + "PRIVATE KEY-----"],
  ["literal-secret", ["password", "="].join("") + '"synthetic-secret-value"'],
  ["workstation-path", "C:" + "\\Users\\synthetic\\project"],
  ["network-address", "2001:4860" + ":4860::8888"],
  ["network-address", "::ffff:" + FIXTURES["network-address"][0]],
  ["corrupted-text", "bad \uFFFD"],
  ["corrupted-text", "bad \u00C3\u00BC"],
  ["corrupted-text", "bad \u00E2\u20AC\u201D"],
  ["corrupted-text", "bad \x07"],
  ["ai-co-author", "Co-authored-by: " + "Codex <123+codex@users.noreply.github.com>"],
  ["ai-co-author", "Co-authored-by: " + "Claude Code"],
  ["session-link", "https://claude.ai/" + "code/session_example"],
  ["session-link", "https://chatgpt.com/" + "share/example"],
];
for (const [index, [category, value]] of cases.entries())
  test(`publication pattern red/green ${index + 1}: ${category}`, (t) => {
    const root = scratch(t);
    stage(root, "sample.txt", value);
    fails(node(root, "scripts/check-publication.mjs", "--staged"), category);
    stage(root, "sample.txt", "Safe text, valid UTF-8: café. Assisted-by: Codex\n");
    ok(node(root, "scripts/check-publication.mjs", "--staged"));
  });
test("publication exact fixture exceptions never cover adjacent values or other files", () => {
  const value = FIXTURES["network-address"][0];
  assert.deepEqual(inspectText(value, "scripts/publication-fixtures.mjs"), []);
  assert.equal(inspectText(value, "tests/another.test.mjs").length, 1);
  assert.equal(inspectText(value.replace(/31$/, "32"), "scripts/publication-fixtures.mjs").length, 1);
  assert.equal(
    inspectText(value, "scripts/publication-fixtures.mjs", [value])[0].category,
    "operator-denylist",
  );
});
test("index content is checked even when the worktree is clean", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", FIXTURES["private-hostname"][0]);
  write(root, "sample.txt", "safe\n");
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "private-hostname");
  ok(node(root, "scripts/check-publication.mjs", "--worktree"));
});
test("removed bad history remains blocked", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", FIXTURES["private-hostname"][0]);
  rawCommit(root);
  stage(root, "sample.txt", "safe\n");
  rawCommit(root);
  fails(node(root, "scripts/check-publication.mjs", "--history"), "private-hostname");
  ok(node(root, "scripts/check-publication.mjs", "--worktree"));
});
test("invalid UTF-8 fails while valid international text passes", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", Buffer.from([0xc3, 0x28]));
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "invalid-utf8");
  stage(root, "sample.txt", "Valid café and Grüße\n");
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
});
test("runtime env and private artefacts fail even when force added", (t) => {
  const root = scratch(t);
  for (const [path, category] of [
    [".env", "runtime-env-file"],
    ["fixture.key", "private-artifact"],
    [".claude/local.json", "private-artifact"],
  ]) {
    write(root, path, "synthetic\n");
    git(root, "add", "-f", path);
    fails(node(root, "scripts/check-publication.mjs", "--staged"), category);
    git(root, "rm", "--cached", path);
  }
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
});
test("local denylist is optional, literal, case insensitive, fail-closed and never tracked", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", "private-fixture.example.com\n");
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  writeJSON(root, ".publication-denylist.local.json", ["PRIVATE-FIXTURE.EXAMPLE.COM"]);
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "operator-denylist");
  stage(root, "sample.txt", "safe\n");
  ok(node(root, "scripts/check-publication.mjs", "--staged"));
  git(root, "add", "-f", ".publication-denylist.local.json");
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "private-artifact");
  git(root, "rm", "--cached", ".publication-denylist.local.json");
  write(root, ".publication-denylist.local.json", "broken");
  fails(node(root, "scripts/check-publication.mjs"), "check-error");
  writeJSON(root, ".publication-denylist.local.json", []);
  ok(node(root, "scripts/check-publication.mjs"));
});
test("unsafe filenames never leak through diagnostics", (t) => {
  const root = scratch(t);
  const path = FIXTURES["private-hostname"][0];
  stage(root, path, "safe");
  const result = node(root, "scripts/check-publication.mjs", "--staged");
  fails(result, "private-hostname");
  assert.equal(result.stderr.includes(path), false);
});
test("human names and noreply identities are distinct from AI coauthors", () => {
  assert.equal(publicIdentity("Example Contributor", "123+example@users.noreply.github.com"), true);
  assert.equal(publicIdentity("GitHub", "noreply@github.com"), true);
  assert.equal(publicIdentity("Codex", "123+example@users.noreply.github.com"), false);
  assert.equal(publicIdentity("Example Contributor", "person@example.com"), false);
  assert.deepEqual(inspectText("Co-authored-by: Claude Martin <123+example@users.noreply.github.com>"), []);
  assert.deepEqual(inspectPath(".env.example"), []);
  assert.deepEqual(
    inspectText(
      readFileSync(new URL("../scripts/publication-fixtures.mjs", import.meta.url), "utf8"),
      "scripts/publication-fixtures.mjs",
    ),
    [],
  );
});

test("PR CI checks the incoming history instead of a synthetic merge subject", (t) => {
  const root = scratch(t);
  const clean = rawCommit(root);
  rawCommit(root, "Synthetic platform merge subject");
  fails(node(root, "scripts/check-publication.mjs", "--history"), "commit-subject");
  writeJSON(root, ".git/event.json", { pull_request: { head: { sha: clean } } });
  ok(
    invoke(root, process.execPath, ["scripts/check-publication.mjs", "--history"], {
      env: {
        ...process.env,
        GITHUB_EVENT_NAME: "pull_request",
        GITHUB_EVENT_PATH: `${root}/.git/event.json`,
      },
    }),
  );
});
