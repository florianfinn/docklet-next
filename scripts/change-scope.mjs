// Source: docklet-hub/scripts/change-scope.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Root Node tests, conservative executable paths, licence tree, release-train metadata and fail-closed scope calculation.
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
const RELEASE_VERSION = "deploy/release/version.json";
const PROVENANCE = "scripts/provenance.json";
// A release train changes text, the next version and that file's provenance hash and note. The text scope still
// runs publication, structure (which checks the hash) and every root Node test, including the release tests.
export function isReleaseMetadata(root, base, head, paths) {
  if (!paths.includes(RELEASE_VERSION)) return false;
  if (!paths.every((path) => path === RELEASE_VERSION || path === PROVENANCE || isTextOnly([path]))) return false;
  try {
    const version = JSON.parse(git(["show", `${head}:${RELEASE_VERSION}`], root));
    // Stable versions only, with the release contract's rule against leading zeros (deploy/release/common.mjs).
    if (Object.keys(version).join() !== "next" || typeof version.next !== "string" ||
        !/^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/.test(version.next)) return false;
    if (!paths.includes(PROVENANCE)) return true;
    const fork = git(["merge-base", base, head], root).trim();
    const entryless = (ref) => {
      const document = JSON.parse(git(["show", `${ref}:${PROVENANCE}`], root));
      // JSON cannot carry a header, so its provenance lives in a sidecar entry.
      const entry = document.sidecars?.[RELEASE_VERSION];
      if (entry) document.sidecars[RELEASE_VERSION] = { ...entry, deviations: undefined, sha256: undefined };
      return JSON.stringify(document);
    };
    return entryless(fork) === entryless(head);
  } catch {
    return false;
  }
}
function scopeOf(root, base, head) {
  const paths = changedPaths(root, base, head);
  return isTextOnly(paths) || isReleaseMetadata(root, base, head, paths) ? "text" : "full";
}
export function changedPaths(root, base, head) {
  return git(["diff", "--name-only", "--no-renames", "-z", `${base}...${head}`], root)
    .split("\0")
    .filter(Boolean);
}
export function ciScope(root = process.cwd(), eventName = process.env.GITHUB_EVENT_NAME, event) {
  if (eventName !== "pull_request") return "full";
  const pr = (event ?? JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"))).pull_request;
  return scopeOf(root, pr.base.sha, pr.head.sha);
}
export function prePushScope(input, root = process.cwd()) {
  const heads = input
    .trim()
    .split("\n")
    .map((line) => line.trim().split(/\s+/)[1])
    .filter((sha) => sha && !/^0+$/.test(sha));
  if (!heads.length) return "full";
  try {
    return heads.every((head) => scopeOf(root, "origin/main", head) === "text") ? "text" : "full";
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
