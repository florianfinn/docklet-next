// Source: docklet-hub/scripts/change-scope.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Root Node tests, conservative executable paths, licence tree and fail-closed scope calculation.
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { git, main, run } from "./repository.mjs";

const EXECUTED =
  /^(?:scripts|tests|backend|cli|types|third_party|web|api|deploy|\.githooks|\.github|\.claude)\/|(?:^|\/)src\//;
export function isTextOnly(paths) {
  return (
    paths.length > 0 &&
    paths.every(
      (path) =>
        !EXECUTED.test(path) &&
        (/\.md$/.test(path) || /^(?:LICENSE|NOTICE)$/.test(path) || /^LICENSES\/[^/]+$/.test(path)),
    )
  );
}
export function changedPaths(root, base, head) {
  return git(["diff", "--name-only", "--no-renames", "-z", `${base}...${head}`], root)
    .split("\0")
    .filter(Boolean);
}
export function ciScope(root = process.cwd(), eventName = process.env.GITHUB_EVENT_NAME, event) {
  if (eventName !== "pull_request") return "full";
  const pr = (event ?? JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"))).pull_request;
  return isTextOnly(changedPaths(root, pr.base.sha, pr.head.sha)) ? "text" : "full";
}
export function prePushScope(input, root = process.cwd()) {
  const heads = input
    .trim()
    .split("\n")
    .map((line) => line.trim().split(/\s+/)[1])
    .filter((sha) => sha && !/^0+$/.test(sha));
  if (!heads.length) return "full";
  try {
    return heads.every((head) => isTextOnly(changedPaths(root, "origin/main", head))) ? "text" : "full";
  } catch {
    return "full";
  }
}
if (main(import.meta.url))
  run(() => {
    const mode = process.argv[2];
    if (mode === "--ci") console.log(ciScope());
    else if (mode === "--pre-push") console.log(prePushScope(readFileSync(0, "utf8")));
    else if (mode === "--run-text-tests")
      process.exitCode =
        spawnSync(process.execPath, ["scripts/run-checks.mjs", "text"], { stdio: "inherit" }).status ?? 1;
    else if (mode === "--assert-text") {
      if (!isTextOnly(process.argv.slice(3))) {
        console.error("scope [executable-change]");
        process.exitCode = 1;
      }
    } else throw new Error("unknown-mode");
  });
