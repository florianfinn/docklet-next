import test from "node:test";
import assert from "node:assert/strict";
import { subjectAccepted } from "../scripts/commit-subject.mjs";
import { textProblems } from "../scripts/check-pr-publication.mjs";

for (const type of [
  "feat",
  "fix",
  "docs",
  "style",
  "refactor",
  "perf",
  "test",
  "build",
  "ci",
  "chore",
  "revert",
]) {
  test(`Conventional type ${type} accepts valid subjects`, () => {
    for (const subject of [`${type}: example`, `${type}(api/v2)!: example (#2)`, `${type}!: example`]) {
      assert.equal(subjectAccepted(subject), true);
      assert.deepEqual(textProblems(subject), []);
    }
  });
}
for (const subject of [
  "wip: x",
  "Update docs: x",
  "Fix: x",
  "feat(Scope): x",
  "feat:x",
  "feat: ",
  "feat(): x",
]) {
  test(`Conventional subject rejects ${JSON.stringify(subject)}`, () => {
    assert.equal(subjectAccepted(subject), false);
    assert.equal(
      textProblems(subject).some((finding) => finding.category === "commit-subject"),
      true,
    );
  });
}
