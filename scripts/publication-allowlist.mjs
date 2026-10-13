import { createHash } from "node:crypto";

export const ALLOWLIST_PATH = "scripts/publication-allowlist.json";
// Only line terminators are normalized; indentation and trailing spaces stay significant.
export const lineHash = (line) => createHash("sha256").update(line.replace(/\r\n|\r/g, "\n")).digest("hex");
const categories = new Set([
  "credential-pattern", "literal-secret", "workstation-path", "corrupted-text",
  "session-link", "private-hostname", "non-example-email", "network-address", "ai-co-author",
]);
const key = (entry) => JSON.stringify([entry.path, entry.category, entry.sha256]);
export function reviewedFindings(list, findings) {
  if (!Array.isArray(list)) throw new Error("publication-allowlist-format");
  const unused = new Map();
  for (const entry of list) {
    if (!entry || Object.keys(entry).sort().join(",") !== "category,path,reason,sha256" ||
        typeof entry.path !== "string" || /[\\*?\[\]\x00-\x1f]/.test(entry.path) ||
        entry.path.split("/").some((part) => !part || part === "." || part === "..") ||
        !categories.has(entry.category) || !/^[a-f0-9]{64}$/.test(entry.sha256) ||
        typeof entry.reason !== "string" || !entry.reason.trim() || unused.has(key(entry)))
      throw new Error("publication-allowlist-entry");
    unused.set(key(entry), entry);
  }
  const remaining = findings.filter((finding) => !unused.delete(key(finding)));
  for (const entry of unused.values()) remaining.push({ path: ALLOWLIST_PATH, category: "stale-publication-allowlist" });
  return remaining.map(({ path, category }) => ({ path, category }));
}
