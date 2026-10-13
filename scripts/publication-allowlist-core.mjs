import { createHash } from "node:crypto";

export const ALLOWLIST_PATH = "scripts/publication-allowlist.json";
// Only line terminators are normalized; indentation and trailing spaces stay significant.
export const lineHash = (line) => createHash("sha256").update(line.replace(/\r\n|\r/g, "\n")).digest("hex");
export const categories = new Set([
  "credential-pattern", "literal-secret", "workstation-path",
  "private-hostname", "non-example-email", "network-address",
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
        typeof entry.reason !== "string" || entry.reason.trim().length < 12 || /^(?:todo|tbd|fixme|reason)\b|<[^>]*>/i.test(entry.reason.trim()) ||
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

// JSON.parse validates syntax; this token pass rejects duplicate decoded object keys.
export function parseAllowlist(text) {
  const parsed = JSON.parse(text);
  const stack = [];
  const tokens = new RegExp(String.raw`"(?:\\[\s\S]|[^"\\])*"|[{}\[\]:,]|[^\s{}\[\]:,]+`, "g");
  let token;
  while ((token = tokens.exec(text))) {
    const value = token[0];
    if (value === "{") stack.push({ keys: new Set(), key: true });
    else if (value === "[") stack.push(null);
    else if (value === "}" || value === "]") stack.pop();
    else {
      const object = stack.at(-1);
      if (!object) continue;
      if (value === ",") object.key = true;
      else if (value === ":") object.key = false;
      else if (object.key && value.startsWith('"')) {
        const key = JSON.parse(value);
        if (object.keys.has(key)) throw new Error("publication-allowlist-duplicate-field");
        object.keys.add(key);
      }
    }
  }
  return parsed;
}
