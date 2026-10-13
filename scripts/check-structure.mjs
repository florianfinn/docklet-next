import { createHash } from "node:crypto";
import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { files, json, git, main, run, finish } from "./repository.mjs";

export const sha256 = (content) => createHash("sha256").update(content).digest("hex");
export const lineCount = (content) => {
  const value = content.toString("utf8");
  return value ? value.split(/\r\n|\r|\n/).length - (/[\r\n]$/.test(value) ? 1 : 0) : 0;
};
const relative = (path) =>
  typeof path === "string" &&
  path.length > 0 &&
  !path.startsWith("/") &&
  !path.includes("\\") &&
  !path.split("/").some((p) => p === ".." || p === "." || !p);
const digest = (value) => typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
export function provenanceFields(text) {
  const field = (name) => text.match(new RegExp("^(?://|#|\\*)? ?" + name + ": (.+)$", "m"))?.[1]?.trim();
  return {
    source: field("Source"),
    revision: field("Revision"),
    retrieved: field("Retrieved"),
    license: field("License"),
    deviations: field("Deviations"),
  };
}
export function validProvenance(record) {
  return (
    record &&
    typeof record.source === "string" &&
    record.source.includes("/") &&
    /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(record.revision ?? "") &&
    /^\d{4}-\d{2}-\d{2}$/.test(record.retrieved ?? "") &&
    typeof record.license === "string" &&
    record.license.length > 0 &&
    typeof record.deviations === "string" &&
    record.deviations.length > 0
  );
}
// Exemptions are constrained in code; editing the baseline cannot admit arbitrary files.
const LOCKFILES = new Set(["pnpm-lock.yaml", "package-lock.json", "go.sum", "yarn.lock"]);
const GENERATED_DIRECTORIES = ["api/generated/", "web/src/generated/"];
function generatedFile(file) {
  if (LOCKFILES.has(file.path.split("/").at(-1))) return true;
  return (
    GENERATED_DIRECTORIES.some((prefix) => file.path.startsWith(prefix)) &&
    /\bgenerated\b/i.test(file.content.toString("utf8", 0, 4096))
  );
}
function headerRecord(file) {
  return (
    file &&
    provenanceFields(
      file.content
        .toString("utf8")
        .split(/\r\n|\r|\n/)
        .slice(0, 40)
        .join("\n"),
    )
  );
}
export function sizeProblems(list, policy, previous = { adopted: {} }, previousCounts = {}, provenance = {}) {
  const found = [];
  if (!policy || !policy.adopted || !Array.isArray(policy.generated)) throw new Error("size-policy");
  const byPath = new Map(list.map((file) => [file.path, file]));
  const generated = new Set();
  for (const path of policy.generated) {
    if (!relative(path)) throw new Error("generated-policy");
    const file = byPath.get(path);
    if (!file) found.push({ path, category: "stale-generated-entry" });
    else if (!generatedFile(file)) found.push({ path, category: "generated-new-entry" });
    else generated.add(path);
  }
  for (const [path, ceiling] of Object.entries(policy.adopted)) {
    if (!relative(path) || !Number.isInteger(ceiling) || ceiling <= 0) throw new Error("size-policy");
    const file = byPath.get(path);
    const covered =
      provenance.files?.includes(path) || provenance.prefixes?.some((prefix) => path.startsWith(prefix));
    if (!covered || !validProvenance(headerRecord(file)))
      found.push({ path, category: "size-baseline-provenance" });
    // First adoption admits only a new file with an exact ceiling, never an existing own file.
    if (
      previous &&
      previous.adopted?.[path] === undefined &&
      (Object.hasOwn(previousCounts, path) || !file || ceiling !== lineCount(file.content))
    )
      found.push({ path, category: "size-baseline-new-entry" });
    if (previous?.adopted?.[path] !== undefined && ceiling > previous.adopted[path])
      found.push({ path, category: "size-baseline-growth" });
    if (!file) found.push({ path, category: "stale-size-baseline" });
  }
  for (const file of list) {
    if (generated.has(file.path) || file.content.includes(0)) continue;
    const ceiling =
      policy.adopted[file.path] === undefined
        ? 1000
        : Math.min(policy.adopted[file.path], previousCounts[file.path] ?? Infinity);
    if (lineCount(file.content) > ceiling) found.push({ path: file.path, category: "file-size" });
  }
  return found;
}
export function provenanceProblems(list, policy) {
  if (!Array.isArray(policy.prefixes) || !Array.isArray(policy.files) || !policy.sidecars)
    throw new Error("provenance-policy");
  const findings = [];
  for (const path of [...policy.files, ...Object.keys(policy.sidecars)]) {
    if (!relative(path)) throw new Error("provenance-path");
    if (!list.some((f) => f.path === path)) findings.push({ path, category: "stale-provenance" });
  }
  for (const file of list) {
    const required = policy.files.includes(file.path) || policy.prefixes.some((p) => file.path.startsWith(p));
    if (!required && !policy.sidecars[file.path]) continue;
    const record = policy.sidecars[file.path] ?? headerRecord(file);
    if (!validProvenance(record)) findings.push({ path: file.path, category: "provenance-header" });
    if (policy.sidecars[file.path] && record.sha256 !== sha256(file.content))
      findings.push({ path: file.path, category: "provenance-hash" });
  }
  return findings;
}
const MEDIA = /\.(?:png|jpe?g|gif|webp|avif|ico|svg|woff2?|ttf|otf|pdf|blend|glb|gltf|bin|exe|wasm)$/i;
const EXCLUDED = /\.(?:mp[34]|wav|ogg|webm|mov|avi|flac|m4a|mkv|m4v|flv|aac|opus|wma|aiff)$/i;
export function binaryOrMedia(file) {
  if (MEDIA.test(file.path) || EXCLUDED.test(file.path) || file.content.includes(0)) return true;
  try {
    new TextDecoder("utf-8", { fatal: true }).decode(file.content);
    return false;
  } catch {
    return true;
  }
}
export function assetProblems(list, policy) {
  if (!Array.isArray(policy)) throw new Error("asset-policy");
  const entries = new Map();
  for (const entry of policy) {
    if (
      !relative(entry.path) ||
      !digest(entry.sha256) ||
      !entry.source ||
      !entry.license ||
      !entry.purpose ||
      entries.has(entry.path)
    )
      throw new Error("asset-entry");
    entries.set(entry.path, entry);
  }
  const found = [];
  for (const file of list) {
    if (EXCLUDED.test(file.path) || /(?:^|\/)(?:render-cache|render-caches)\//i.test(file.path)) {
      found.push({ path: file.path, category: "excluded-media" });
      continue;
    }
    if (!binaryOrMedia(file) && !entries.has(file.path)) continue;
    if (entries.get(file.path)?.sha256 !== sha256(file.content))
      found.push({ path: file.path, category: "asset-allowlist" });
  }
  for (const path of entries.keys())
    if (!list.some((f) => f.path === path)) found.push({ path, category: "stale-asset" });
  return found;
}
export function licenseProblems(list, policy, root) {
  if (!relative(policy.source) || !digest(policy.sha256) || !Array.isArray(policy.artifactCopies))
    throw new Error("licence-policy");
  const found = [];
  const licence = list.find((f) => f.path === policy.source);
  if (!licence || sha256(licence.content) !== policy.sha256)
    found.push({ path: policy.source, category: "licence-integrity" });
  for (const path of policy.artifactCopies) {
    if (!relative(path)) throw new Error("licence-copy-path");
    if (!existsSync(resolve(root, path)) || sha256(readFileSync(resolve(root, path))) !== policy.sha256)
      found.push({ path, category: "licence-artifact-copy" });
  }
  return found;
}
export function inspectStructure(root, { only, base } = {}) {
  const list = files(root);
  if (!base) {
    const heads = git(["rev-list", "--all", "--max-count=1"], root).trim();
    if (heads) base = git(["rev-parse", "HEAD"], root).trim();
  }
  const previous = base
    ? JSON.parse(git(["show", `${base}:scripts/size-baseline.json`], root))
    : { adopted: {} };
  const previousCounts = {};
  if (base)
    for (const file of files(root, { revision: base })) {
      previousCounts[file.path] = lineCount(file.content);
    }
  const checks = {
    size: () =>
      sizeProblems(
        list,
        json(resolve(root, "scripts/size-baseline.json")),
        previous,
        previousCounts,
        json(resolve(root, "scripts/provenance.json")),
      ),
    provenance: () => provenanceProblems(list, json(resolve(root, "scripts/provenance.json"))),
    assets: () => assetProblems(list, json(resolve(root, "scripts/assets.json"))),
    license: () => licenseProblems(list, json(resolve(root, "scripts/license-policy.json")), root),
  };
  if (only && !checks[only]) throw new Error("unknown-check");
  return (only ? [checks[only]] : Object.values(checks)).flatMap((check) => check());
}
if (main(import.meta.url))
  run(() => {
    const args = process.argv.slice(2);
    const only = args.includes("--only") ? args[args.indexOf("--only") + 1] : undefined;
    const base = args.includes("--base") ? args[args.indexOf("--base") + 1] : process.env.CHECK_BASE_SHA;
    finish(inspectStructure(process.cwd(), { only, base: base || undefined }));
  });
