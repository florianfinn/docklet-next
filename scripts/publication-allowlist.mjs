import { files, main, run } from "./repository.mjs";
import { inspectText, inspectPath } from "./publication-patterns.mjs";
import { loadDenylist } from "./check-publication.mjs";
import { categories, findingKey } from "./publication-allowlist-core.mjs";
export { lineHash, reviewedFindings, ALLOWLIST_PATH } from "./publication-allowlist-core.mjs";

export function proposeFindings(root, prefixes) {
  if (!prefixes.length || prefixes.some((prefix) => typeof prefix !== "string" ||
      /[\\*?\[\]\x00-\x1f]/.test(prefix) ||
      prefix.replace(/\/$/, "").split("/").some((part) => !part || part === "." || part === "..")))
    throw new Error("publication-proposal-prefix");
  const roots = prefixes.map((prefix) => prefix.replace(/\/$/, ""));
  const denylist = loadDenylist(root);
  const proposed = new Map();
  for (const { path, mode, content } of files(root)) {
    if (!roots.some((prefix) => path === prefix || path.startsWith(prefix + "/"))) continue;
    // Unsafe filenames cannot be included even in an unreviewed proposal.
    if (inspectPath(path, denylist).length || !["100644", "100755"].includes(mode))
      throw new Error("publication-proposal-path");
    if (content.includes(0)) continue;
    const text = new TextDecoder("utf-8", { fatal: true }).decode(content);
    for (const finding of inspectText(text, path, denylist, { detailed: true })) {
      if (!categories.has(finding.category)) continue;
      const id = findingKey(finding);
      const entry = proposed.get(id);
      if (entry) entry.count++;
      else proposed.set(id, { path, category: finding.category, lineSha256: finding.sha256, count: 1, reason: "TODO" });
    }
  }
  return [...proposed.values()];
}
if (main(import.meta.url)) run(() => {
  const [mode, ...prefixes] = process.argv.slice(2);
  if (mode !== "--propose") throw new Error("publication-proposal-usage");
  console.log(JSON.stringify(proposeFindings(process.cwd(), prefixes), null, 2));
});
