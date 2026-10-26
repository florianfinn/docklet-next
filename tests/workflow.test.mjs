import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { subjectAccepted } from "../scripts/commit-subject.mjs";
import { isTextOnly, ciScope, prePushScope } from "../scripts/change-scope.mjs";
import { parseReview, statusFor, eventStatus } from "../scripts/review-status.mjs";
import { FIXTURES } from "../scripts/publication-fixtures.mjs";
import {
  scratch,
  stage,
  write,
  git,
  node,
  ok,
  fails,
  invoke,
  rawCommit,
  writeJSON,
  ROOT,
  stubModuleChecks,
} from "./helpers.mjs";

test("commit-msg rejects invalid subjects and accepts Conventional Commits including revert", (t) => {
  const root = scratch(t);
  write(root, ".git/message", "unstructured subject\n");
  fails(invoke(root, "sh", [".githooks/commit-msg", ".git/message"]), "commit-subject");
  write(root, ".git/message", "revert: restore expected behaviour (#2)\n\nAssisted-by: Codex\n");
  ok(invoke(root, "sh", [".githooks/commit-msg", ".git/message"]));
  assert.equal(subjectAccepted("Merge branch 'feature/example'", 1), false);
  assert.equal(subjectAccepted("Merge branch 'feature/example'", 2), true);
});
test("commit-msg rejects AI coauthors and session links", (t) => {
  const root = scratch(t);
  for (const [body, category] of [
    ["Co-authored-by: " + "Codex <123+codex@users.noreply.github.com>", "ai-co-author"],
    ["https://claude.ai/" + "code/session_fixture", "session-link"],
  ]) {
    write(root, ".git/message", "test: check metadata\n\n" + body);
    fails(invoke(root, "sh", [".githooks/commit-msg", ".git/message"]), category);
  }
  write(root, ".git/message", "test: clean metadata\n");
  ok(invoke(root, "sh", [".githooks/commit-msg", ".git/message"]));
});
test("PR event CLI rejects bad title, coauthor and session metadata without shell expansion", (t) => {
  const root = scratch(t);
  const eventPath = `${root}/.git/pr-event.json`;
  const check = () =>
    invoke(root, process.execPath, ["scripts/check-pr-publication.mjs"], {
      env: { ...process.env, GITHUB_EVENT_PATH: eventPath, GITHUB_EVENT_NAME: "pull_request" },
    });
  for (const [title, body, category] of [
    ["bad title", "", "commit-subject"],
    ["docs: clarify rules", "Co-authored-by: " + "Codex", "ai-co-author"],
    ["docs: clarify rules", "https://chatgpt.com/" + "share/example", "session-link"],
  ]) {
    writeJSON(root, ".git/pr-event.json", { pull_request: { title, body } });
    fails(check(), category);
  }
  writeJSON(root, ".git/pr-event.json", {
    pull_request: { title: "revert: restore policy", body: "Literal `$(exit 1)` is text." },
  });
  ok(check());
});
test("pre-commit is active and rejects staged violations before a real commit", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", FIXTURES["private-hostname"][0]);
  fails(invoke(root, "git", ["commit", "-m", "test: guard proof"]), "private-hostname");
  assert.notEqual(invoke(root, "git", ["rev-parse", "--verify", "HEAD"]).status, 0);
  stage(root, "sample.txt", "safe\n");
  git(root, "commit", "-m", "test: clean hook proof");
  assert.equal(git(root, "rev-list", "--count", "HEAD"), "1");
});
test("history rejects private author, private committer, AI identity and bad commit message", (t) => {
  const root = scratch(t);
  const base = rawCommit(root);
  const identityCases = [
    { GIT_AUTHOR_EMAIL: "person@example.com" },
    { GIT_COMMITTER_EMAIL: "person@example.com" },
    { GIT_AUTHOR_NAME: "Codex" },
  ];
  for (const identity of identityCases) {
    rawCommit(root, "test: identity fixture", identity);
    fails(node(root, "scripts/check-publication.mjs", "--history", "HEAD"), "private-commit-identity");
    git(root, "reset", "--soft", base);
  }
  rawCommit(root, "bad subject");
  fails(node(root, "scripts/check-publication.mjs", "--history"), "commit-subject");
  git(root, "reset", "--soft", base);
  ok(node(root, "scripts/check-publication.mjs", "--history"));
});
test("pre-push checks the supplied outgoing head and runs the clean lint chain", (t) => {
  const root = scratch(t);
  const env = stubModuleChecks(root);
  const base = rawCommit(root);
  stage(root, "sample.txt", FIXTURES["private-hostname"][0]);
  const bad = rawCommit(root);
  git(root, "reset", "--hard", base);
  const input = (sha) => `refs/heads/main ${sha} refs/heads/main ${"0".repeat(40)}\n`;
  fails(invoke(root, "sh", [".githooks/pre-push"], { env, input: input(bad) }), "private-hostname");
  ok(invoke(root, "sh", [".githooks/pre-push"], { env, input: input(base) }));
});
test("pre-push propagates lint failures and leaves the test suite to CI", (t) => {
  const root = scratch(t);
  const env = stubModuleChecks(root);
  stage(root, "broken.mjs", "export const = ;\n");
  let head = rawCommit(root);
  const input = (sha) => `refs/heads/main ${sha} refs/heads/main ${"0".repeat(40)}\n`;
  fails(invoke(root, "sh", [".githooks/pre-push"], { env, input: input(head) }), "check-error");
  stage(root, "broken.mjs", "export const value = 1;\n");
  stage(
    root,
    "tests/smoke.test.mjs",
    'import test from "node:test"; test("synthetic failure", () => { throw new Error("fixture"); });\n',
  );
  head = rawCommit(root);
  // A failing test no longer blocks the push; CI runs the full suite on every pushed head.
  ok(invoke(root, "sh", [".githooks/pre-push"], { env, input: input(head) }));
  fails(invoke(root, process.execPath, ["scripts/run-checks.mjs", "test"], { env }), "check-error");
});
test("text-only push still rejects altered licence text", (t) => {
  const root = scratch(t);
  const env = stubModuleChecks(root);
  const base = rawCommit(root);
  git(root, "update-ref", "refs/remotes/origin/main", base);
  stage(root, "LICENSES/BSD-3-Clause-Arcane.txt", "changed\n");
  const head = rawCommit(root);
  const input = `refs/heads/main ${head} refs/heads/main ${base}\n`;
  assert.equal(prePushScope(input, root), "text");
  fails(invoke(root, "sh", [".githooks/pre-push"], { env, input }), "licence-integrity");
});
test("scope is conservative for executable trees, renames and missing refs", (t) => {
  const root = scratch(t);
  for (const path of ["README.md", "docs/design/example.md", "LICENSE", "NOTICE", "LICENSES/example.txt"]) {
    assert.equal(isTextOnly([path]), true);
    ok(node(root, "scripts/change-scope.mjs", "--assert-text", path));
  }
  for (const path of [
    "scripts/example.md",
    "web/src/example.md",
    ".github/example.md",
    "backend/example.md",
    "package.json",
  ]) {
    assert.equal(isTextOnly([path]), false);
    fails(node(root, "scripts/change-scope.mjs", "--assert-text", path), "executable-change");
  }
  assert.equal(isTextOnly([]), false);
  assert.equal(ciScope(root, "push"), "full");
  assert.equal(prePushScope("", root), "full");
  assert.equal(prePushScope(`refs/heads/x ${"a".repeat(40)} refs/heads/x ${"0".repeat(40)}`, root), "full");
  const base = rawCommit(root);
  git(root, "update-ref", "refs/remotes/origin/main", base);
  stage(root, "docs/example.md", "Example\n");
  const head = rawCommit(root);
  assert.equal(
    ciScope(root, "pull_request", { pull_request: { base: { sha: base }, head: { sha: head } } }),
    "text",
  );
  git(root, "mv", "scripts/run-checks.mjs", "docs/runner.md");
  const renamed = rawCommit(root);
  assert.equal(
    ciScope(root, "pull_request", { pull_request: { base: { sha: head }, head: { sha: renamed } } }),
    "full",
  );
});
test("hook setup fails for a conflicting path and succeeds idempotently", (t) => {
  const root = scratch(t);
  git(root, "config", "core.hooksPath", ".other-hooks");
  fails(node(root, "scripts/setup-git-hooks.mjs"), "check-error");
  git(root, "config", "--unset", "core.hooksPath");
  ok(node(root, "scripts/setup-git-hooks.mjs"));
  ok(node(root, "scripts/setup-git-hooks.mjs"));
  assert.equal(git(root, "config", "core.hooksPath"), ".githooks");
});
const head = "a".repeat(40);
const review = (sha = head, result = "pass") =>
  `Independent review\nReviewer: Example reviewer; other provider\n` +
  `Head: ${sha}\nResult: ${result}\n\nChecked scope and publication.\n`;
const pr = { state: "open", head: { sha: head } };
test("review CLI rejects malformed, stale, failing and non-owner evidence", (t) => {
  const root = scratch(t);
  const fixture = (body) => ({
    eventName: "issue_comment",
    action: "created",
    issue: { pull_request: {} },
    comment: { author_association: "OWNER", body },
    pr,
  });
  for (const body of [
    "Independent review\n\n" + review(),
    review("b".repeat(40)),
    review(head, "fail"),
    review().replace("Result: pass", "Result: pass\nResult: fail"),
  ]) {
    writeJSON(root, ".git/review.json", fixture(body));
    fails(node(root, "scripts/review-status.mjs", "--check", ".git/review.json"), "failure");
  }
  const outsider = fixture(review());
  outsider.comment.author_association = "CONTRIBUTOR";
  writeJSON(root, ".git/review.json", outsider);
  fails(node(root, "scripts/review-status.mjs", "--check", ".git/review.json"), "ignored");
  writeJSON(root, ".git/review.json", fixture(review()));
  ok(node(root, "scripts/review-status.mjs", "--check", ".git/review.json"));
});
test("review status binds current open head and ignores edited evidence", () => {
  assert.equal(parseReview(review()).head, head);
  assert.equal(statusFor(parseReview(review()), pr).state, "success");
  assert.equal(statusFor(parseReview(review()), { ...pr, state: "closed" }), null);
  assert.equal(
    eventStatus(
      {
        action: "edited",
        issue: { pull_request: {} },
        comment: { author_association: "OWNER", body: review() },
      },
      pr,
      "issue_comment",
    ),
    null,
  );
  assert.equal(eventStatus({}, pr, "pull_request_target").state, "pending");
  assert.equal(eventStatus({}, pr, "push"), null);
});
