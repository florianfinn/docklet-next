// Source: docklet-hub/scripts/review-status.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Ordered header, trusted owner events, pending new heads, offline proof and sanitized failures.
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { main, run } from "./repository.mjs";

export const MARKER = "Independent review";
export function parseReview(body) {
  const lines = (body ?? "").split(/\r?\n/);
  if (
    lines[0] !== MARKER ||
    !/^Reviewer: \S.+$/.test(lines[1] ?? "") ||
    !/^Head: [a-f0-9]{40}$/.test(lines[2] ?? "") ||
    !/^Result: (?:pass|fail)$/.test(lines[3] ?? "") ||
    (lines.length > 4 && lines[4] !== "")
  )
    return null;
  return { reviewer: lines[1].slice(10), head: lines[2].slice(6), result: lines[3].slice(8) };
}
export function statusFor(review, pr) {
  if (pr.state !== "open") return null;
  if (!review) return { state: "failure", description: "Review comment is malformed" };
  if (review.head !== pr.head.sha)
    return { state: "failure", description: "Review does not name the current head" };
  return review.result === "pass"
    ? { state: "success", description: "Independent review passed" }
    : { state: "failure", description: "Independent review found blocking issues" };
}
export function eventStatus(event, pr, eventName) {
  if (eventName === "issue_comment") {
    if (
      !event.issue?.pull_request ||
      event.action !== "created" ||
      event.comment?.author_association !== "OWNER" ||
      !event.comment.body.startsWith(MARKER)
    )
      return null;
    return statusFor(parseReview(event.comment.body), pr);
  }
  if (eventName === "pull_request_target" && pr.state === "open") {
    return { state: "pending", description: "Independent review required for this head" };
  }
  return null;
}
if (main(import.meta.url))
  run(() => {
    const offline = process.argv[2] === "--check";
    const event = JSON.parse(readFileSync(offline ? process.argv[3] : process.env.GITHUB_EVENT_PATH, "utf8"));
    if (offline) {
      const status = eventStatus(event, event.pr, event.eventName);
      console.log(`review [${status?.state ?? "ignored"}]`);
      if (status?.state !== "success") process.exitCode = 1;
      return;
    }
    const eventName = process.env.GITHUB_EVENT_NAME;
    if (eventName === "push") {
      console.log("review [merge-evidence-retained-on-pr]");
      return;
    }
    const repo = process.env.GITHUB_REPOSITORY;
    if (!/^[a-z0-9_.-]+\/[a-z0-9_.-]+$/i.test(repo)) throw new Error("repository-name");
    const number = event.issue?.number ?? event.pull_request?.number;
    if (!Number.isSafeInteger(number)) throw new Error("pull-request-number");
    const gh = (args) => execFileSync("gh", args, { encoding: "utf8", stdio: ["pipe", "pipe", "pipe"] });
    const pr = JSON.parse(gh(["api", `repos/${repo}/pulls/${number}`]));
    const status = eventStatus(event, pr, eventName);
    if (!status) return;
    gh([
      "api",
      "--method",
      "POST",
      `repos/${repo}/statuses/${pr.head.sha}`,
      "-f",
      "context=review",
      "-f",
      `state=${status.state}`,
      "-f",
      `description=${status.description}`,
    ]);
    console.log(`review [${status.state}]`);
    if (status.state === "failure") process.exitCode = 1;
  });
