// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { appendFileSync, readFileSync } from "node:fs";
import { isMain, run, version } from "../deploy/release/common.mjs";

export function verifyVersion({ event, ref, runNumber, next, candidateVersion }) {
  if (event === "pull_request") {
    version(next);
    if (!/^[1-9][0-9]*$/.test(String(runNumber)) || next.includes("-")) throw new Error("invalid dry-run version inputs");
    return `${next}-rc.${runNumber}`;
  }
  if (event !== "push" || !ref?.startsWith("refs/tags/v")) throw new Error("release requires a v tag push");
  const v = version(ref.slice("refs/tags/v".length));
  if (v !== version(candidateVersion)) throw new Error("version and tag mismatch");
  return v;
}
if (isMain(import.meta.url)) {
  const { GITHUB_EVENT_NAME: event, GITHUB_REF: ref, GITHUB_RUN_NUMBER: runNumber, GITHUB_SHA: sha, GITHUB_OUTPUT: output } = process.env;
  const { next } = JSON.parse(readFileSync("deploy/release/version.json"));
  // A tag names the current release train, including prereleases; a different train requires version.json first.
  const candidateVersion = next + (ref?.slice("refs/tags/v".length).match(/-.+$/)?.[0] ?? "");
  const v = verifyVersion({ event, ref, runNumber, next, candidateVersion: event === "push" ? candidateVersion : undefined });
  if (event === "push") run("git", ["merge-base", "--is-ancestor", sha, "origin/main"]);
  if (output) appendFileSync(output, `version=${v}\n`);
  console.log(v);
}
