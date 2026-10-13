import { createHash } from "node:crypto";

export const ALLOWLIST_PATH = "scripts/publication-allowlist.json";
// Only line terminators are normalized; indentation and trailing spaces stay significant.
export const lineHash = (line) => createHash("sha256").update(line.replace(/\r\n|\r/g, "\n")).digest("hex");
export const categories = new Set([
  "credential-pattern", "literal-secret", "workstation-path", "corrupted-text",
  "session-link", "private-hostname", "non-example-email", "network-address", "ai-co-author",
]);
export const findingKey = (entry) => JSON.stringify([entry.path, entry.category, entry.lineSha256 ?? entry.sha256]);
export function reviewedFindings(list, findings) {
  if (!Array.isArray(list)) throw new Error("publication-allowlist-format");
  const unused = new Map();
  for (const entry of list) {
    const fields = entry && Object.keys(entry).filter((field) => field !== "count").sort().join(",");
    if (!entry || !["category,path,reason,sha256", "category,lineSha256,path,reason"].includes(fields) ||
        typeof entry.path !== "string" || /[\\*?\[\]\x00-\x1f]/.test(entry.path) ||
        entry.path.split("/").some((part) => !part || part === "." || part === "..") ||
        !categories.has(entry.category) || !/^[a-f0-9]{64}$/.test(entry.lineSha256 ?? entry.sha256) ||
        typeof entry.reason !== "string" || !entry.reason.trim() || entry.reason.trim() === "TODO" ||
        (Object.hasOwn(entry, "count") && (!Number.isSafeInteger(entry.count) || entry.count < 1)) ||
        unused.has(findingKey(entry)))
      throw new Error("publication-allowlist-entry");
    unused.set(findingKey(entry), entry.count ?? 1);
  }
  const remaining = findings.filter((finding) => {
    const id = findingKey(finding);
    const count = unused.get(id) ?? 0;
    if (count === 0) return true;
    if (count === 1) unused.delete(id);
    else unused.set(id, count - 1);
    return false;
  });
  for (const entry of unused.values()) remaining.push({ path: ALLOWLIST_PATH, category: "stale-publication-allowlist" });
  return remaining.map(({ path, category }) => ({ path, category }));
}
