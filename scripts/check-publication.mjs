// Source: docklet-hub/scripts/check-publication.mjs
// Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Node-only staged/worktree/history scans, strict identities, local denylist and sanitized failures.
import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { files, git, main, run, finish } from "./repository.mjs";
import { inspectText, inspectPath, safePath, publicIdentity } from "./publication-patterns.mjs";
import { ALLOWLIST_PATH, reviewedFindings } from "./publication-allowlist.mjs";
import { messageProblems } from "./commit-subject.mjs";
export { inspectText } from "./publication-patterns.mjs";

export function loadDenylist(root) {
  const path = resolve(root, ".publication-denylist.local.json");
  if (!existsSync(path)) return [];
  const list = JSON.parse(readFileSync(path, "utf8"));
  if (!Array.isArray(list) || list.some((v) => typeof v !== "string" || !v.trim()))
    throw new Error("denylist-format");
  return list;
}
function inspectFiles(list, denylist) {
  const allowlist = JSON.parse(list.find((file) => file.path === ALLOWLIST_PATH)?.content.toString("utf8") ?? "[]");
  const findings = list.flatMap(({ path, mode, content }) => {
    const findings = inspectPath(path, denylist);
    const label = safePath(path, denylist);
    if (!["100644", "100755"].includes(mode))
      findings.push({ path: label, category: "unsupported-git-entry" });
    if (!content.includes(0)) {
      try {
        new TextDecoder("utf-8", { fatal: true }).decode(content);
      } catch {
        findings.push({ path: label, category: "invalid-utf8" });
      }
      findings.push(
        ...inspectText(content.toString("utf8"), path, denylist, { detailed: true }),
      );
    }
    return findings;
  });
  return reviewedFindings(allowlist, findings).map((f) => ({ ...f, path: safePath(f.path, denylist) }));
}
export function inspectRepository(root, { staged = false, history = false, revisions = [] } = {}) {
  const denylist = loadDenylist(root);
  const findings = inspectFiles(files(root, { staged }), denylist);
  if (history) {
    const refs = revisions.length ? revisions : ["--all"];
    const commits = git(["rev-list", ...refs], root)
      .trim()
      .split("\n")
      .filter(Boolean);
    for (const commit of commits) {
      const fields = git(["show", "-s", "--format=%an%x00%ae%x00%cn%x00%ce%x00%P%x00%B", commit], root).split(
        "\0",
      );
      const [author, authorEmail, committer, committerEmail, parents, body] = fields;
      if (!publicIdentity(author, authorEmail) || !publicIdentity(committer, committerEmail))
        findings.push({ path: "commit", category: "private-commit-identity" });
      findings.push(...messageProblems(body, parents.trim().split(" ").filter(Boolean).length));
      findings.push(...inspectText(body, "commit-message", denylist));
      const list = files(root, { revision: commit });
      findings.push(...inspectFiles(list, denylist));
    }
  }
  return findings;
}
if (main(import.meta.url))
  run(() => {
    const args = process.argv.slice(2);
    const at = args.indexOf("--text");
    const revisions = args.filter((v) => /^[a-f0-9]{40}(?:\.\.[a-f0-9]{40})?$/.test(v));
    if (args.includes("--history") && !revisions.length && process.env.GITHUB_EVENT_NAME === "pull_request") {
      const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, "utf8"));
      const head = event.pull_request?.head?.sha;
      if (!/^[a-f0-9]{40}$/.test(head ?? "")) throw new Error("invalid-pull-request-head");
      revisions.push(head);
    }
    if (at >= 0) finish(inspectText(readFileSync(args[at + 1], "utf8"), "text", loadDenylist(process.cwd())));
    else
      finish(
        inspectRepository(process.cwd(), {
          staged: args.includes("--staged"),
          history: args.includes("--history"),
          revisions,
        }),
      );
  });
