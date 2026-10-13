// Source: docklet-hub/scripts/setup-git-hooks.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Fail on conflicting hook configuration; skip source archives; concise English output.
import { chmodSync } from "node:fs";
import { git, main, run } from "./repository.mjs";

export function setup(root = process.cwd()) {
  try {
    git(["rev-parse", "--git-dir"], root);
  } catch {
    console.log("hooks [source-archive-skipped]");
    return;
  }
  let current = "";
  try {
    current = git(["config", "--local", "core.hooksPath"], root).trim();
  } catch {
    /* No local setting. */
  }
  if (current && current !== ".githooks") throw new Error("conflicting-hooks-path");
  git(["config", "--local", "core.hooksPath", ".githooks"], root);
  for (const hook of ["pre-commit", "commit-msg", "pre-push"]) chmodSync(`${root}/.githooks/${hook}`, 0o755);
}
if (main(import.meta.url)) run(() => setup());
