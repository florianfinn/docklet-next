// Source: docklet-hub/scripts/commit-subject.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Executable subject/message guard with publication checks and real merge-parent detection.
import { readFileSync, existsSync } from "node:fs";
import { git, main, run, finish } from "./repository.mjs";
import { inspectText } from "./publication-patterns.mjs";

export const CONVENTIONAL =
  /^(?:feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(?:\([a-z0-9/-]+\))?!?: \S[^\r\n]*$/;
export function subjectAccepted(subject, parentCount = 1) {
  return (
    CONVENTIONAL.test(subject) ||
    (parentCount > 1 && /^Merge (?:pull request #\d+ from |branch '|remote-tracking branch ')/.test(subject))
  );
}
export function messageProblems(message, parentCount = 1) {
  const findings = inspectText(message, "commit-message");
  if (!subjectAccepted(message.split(/\r?\n/)[0], parentCount))
    findings.push({ path: "commit-message", category: "commit-subject" });
  return findings;
}
if (main(import.meta.url))
  run(() => {
    const mergeHead = git(["rev-parse", "--git-path", "MERGE_HEAD"]).trim();
    const message = readFileSync(process.argv[2], "utf8")
      .split("\n")
      .filter((line) => !line.startsWith("#"))
      .join("\n");
    finish(messageProblems(message, existsSync(mergeHead) ? 2 : 1));
  });
