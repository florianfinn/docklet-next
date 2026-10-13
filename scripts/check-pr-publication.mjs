// Source: docklet-hub/scripts/check-pr-publication.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: English-only diagnostics; publication and session/co-author checks; push verifies commit text.
import { readFileSync } from "node:fs";
import { CONVENTIONAL } from "./commit-subject.mjs";
import { inspectText, inspectRepository } from "./check-publication.mjs";
import { main, run, finish } from "./repository.mjs";

export function textProblems(title, body = "") {
  const problems = inspectText(title + "\n\n" + body, "pull-request");
  if (!CONVENTIONAL.test(title)) problems.push({ path: "pull-request", category: "commit-subject" });
  return problems;
}
if (main(import.meta.url))
  run(() => {
    const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"));
    if (event.pull_request) finish(textProblems(event.pull_request.title, event.pull_request.body ?? ""));
    else if (process.env.GITHUB_EVENT_NAME === "push")
      finish(inspectRepository(process.cwd(), { history: true }));
    else throw new Error("missing-pull-request");
  });
