// Source: docklet-hub/.githooks/pre-push
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Read ref updates without shell interpolation; inspect outgoing commit trees and policy ceilings; lint only, tests in CI.
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { prePushScope } from "./change-scope.mjs";
import { inspectRepository } from "./check-publication.mjs";
import { inspectStructure } from "./check-structure.mjs";
import { git, run, finish } from "./repository.mjs";

run(() => {
  const input = readFileSync(0, "utf8");
  const updates = input
    .trim()
    .split("\n")
    .filter(Boolean)
    .map((line) => line.split(/\s+/));
  const revisions = [];
  for (const [, local, , remote] of updates) {
    if (!/^[a-f0-9]{40}$/.test(local ?? "") || !/^[a-f0-9]{40}$/.test(remote ?? ""))
      throw new Error("invalid-ref-update");
    if (/^0+$/.test(local)) continue;
    let remoteExists = false;
    if (!/^0+$/.test(remote))
      try {
        git(["cat-file", "-e", remote]);
        remoteExists = true;
      } catch {
        /* Inspect all reachable commits. */
      }
    revisions.push(remoteExists ? `${remote}..${local}` : local);
  }
  if (updates.length && !revisions.length) return;
  finish(inspectRepository(process.cwd(), { history: true, revisions }));
  let base;
  try {
    base = git(["rev-parse", "--verify", "origin/main"]).trim();
  } catch {
    /* Initial publication has no baseline. */
  }
  finish(inspectStructure(process.cwd(), { base }));
  if (process.exitCode) return;
  // Lint covers publication, structure, migrations, vet and typechecks; the full test suite runs in CI.
  const modes = prePushScope(input) === "text" ? ["text"] : ["lint"];
  for (const mode of modes) {
    const result = spawnSync(process.execPath, ["scripts/run-checks.mjs", mode], { stdio: "inherit" });
    if (result.status !== 0) {
      process.exitCode = 1;
      return;
    }
  }
});
