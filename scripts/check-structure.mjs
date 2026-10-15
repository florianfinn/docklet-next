import { createHash } from "node:crypto";
import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { files, git, main, run, finish } from "./repository.mjs";

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
  const field = (name) => text.match(new RegExp("^(?://|--|#|\\*)? ?" + name + ": (.+)$", "m"))?.[1]?.trim();
  return {
    source: field("Source"),
    revision: field("Revision"),
    retrieved: field("Retrieved"),
    license: field("License"),
    deviations: field("Deviations"),
  };
}
const MIGRATION_PREFIX = "backend/resources/migrations/postgres/";
const PROVENANCE_FIELDS = ["source", "revision", "retrieved", "license", "deviations"];
const SHIPPED_MIGRATION_OVERRIDES = new Set([
  "001_baseline.sql", "002_run_traces.sql", "003_sync_receipt_lifecycle.sql",
  "004_gate_owner.sql", "005_gate_lease.sql", "006_single_hub_gate.sql",
  "007_activity_raw_output.sql",
].map((name) => MIGRATION_PREFIX + name));
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
const GENERATED_DIRECTORIES = ["api/generated/", "web/src/generated/", "web/src/api/generated/"];
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
export function sizeProblems(
  list, policy, previous = { adopted: {} }, previousCounts = {}, provenance = {}, origins = new Map(),
) {
  const found = [];
  const currentCeilings = new Map(Object.entries(policy?.adopted ?? {}));
  const oldCeilings = new Map(Object.entries(previous?.adopted ?? {}));
  const counts = new Map(Object.entries(previousCounts));
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
    const covered = provenance.adoptedPrefixes?.some((prefix) => path.startsWith(prefix));
    const origin = origins.get(path) ?? path;
    if (!covered || !validProvenance(headerRecord(file)))
      found.push({ path, category: "size-baseline-provenance" });
    // First adoption admits only a new file with an exact ceiling, never an existing own file.
    if (
      previous &&
      !oldCeilings.has(origin) &&
      (counts.has(origin) || !file || ceiling !== lineCount(file.content))
    )
      found.push({ path, category: "size-baseline-new-entry" });
    if (oldCeilings.has(origin) && ceiling > oldCeilings.get(origin))
      found.push({ path, category: "size-baseline-growth" });
    if (!file) found.push({ path, category: "stale-size-baseline" });
  }
  for (const file of list) {
    if (generated.has(file.path) || file.content.includes(0)) continue;
    const origin = origins.get(file.path) ?? file.path;
    const ceiling = currentCeilings.has(file.path)
      ? Math.min(currentCeilings.get(file.path), counts.get(origin) ?? Infinity, oldCeilings.get(origin) ?? Infinity)
      : 1000;
    if (lineCount(file.content) > ceiling) found.push({ path: file.path, category: "file-size" });
  }
  return found;
}
export function provenanceProblems(list, policy) {
  if (!Array.isArray(policy.prefixes) || !Array.isArray(policy.files) || !policy.sidecars)
    throw new Error("provenance-policy");
  const findings = [];
  const sidecars = new Map(Object.entries(policy.sidecars));
  const overrides = policy.migrationHeaderOverrides ?? {};
  if (!overrides || Array.isArray(overrides) || typeof overrides !== "object") throw new Error("provenance-policy");
  for (const [path, fields] of Object.entries(overrides)) {
    if (!SHIPPED_MIGRATION_OVERRIDES.has(path) || !sidecars.has(path) ||
        !fields || Array.isArray(fields) || typeof fields !== "object" ||
        !Object.keys(fields).length || Object.entries(fields).some(([field, value]) =>
          !PROVENANCE_FIELDS.includes(field) || typeof value !== "string" ||
          !value.length || value !== sidecars.get(path)[field])) throw new Error("provenance-policy");
  }
  for (const path of [...policy.files, ...Object.keys(policy.sidecars)]) {
    if (!relative(path)) throw new Error("provenance-path");
    if (!list.some((f) => f.path === path)) findings.push({ path, category: "stale-provenance" });
  }
  for (const file of list) {
    const required = policy.files.includes(file.path) || policy.prefixes.some((p) => file.path.startsWith(p));
    if (!required && !sidecars.has(file.path) && !file.path.startsWith(MIGRATION_PREFIX)) continue;
    if (file.path.startsWith(MIGRATION_PREFIX) && !sidecars.has(file.path))
      findings.push({ path: file.path, category: "migration-provenance-sidecar" });
    const record = sidecars.get(file.path) ?? headerRecord(file);
    if (!validProvenance(record)) findings.push({ path: file.path, category: "provenance-header" });
    if (sidecars.has(file.path) && record.sha256 !== sha256(file.content))
      findings.push({ path: file.path, category: "provenance-hash" });
    if (file.path.startsWith(MIGRATION_PREFIX) && sidecars.has(file.path)) {
      const header = headerRecord(file);
      const exempt = overrides[file.path] ?? {};
      for (const field of PROVENANCE_FIELDS) {
        if (Object.hasOwn(exempt, field)) {
          if (header[field] === record[field])
            findings.push({ path: file.path, category: "stale-migration-header-override" });
        } else if (header[field] !== record[field]) {
          findings.push({ path: file.path, category: "migration-provenance-mismatch" });
        }
      }
    }
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
export function inspectStructure(root, { only, base, staged = false } = {}) {
  const list = files(root, { staged });
  const byPath = new Map(list.map((file) => [file.path, file]));
  const policy = (path) => JSON.parse(byPath.get(path).content.toString("utf8"));
  if (!base) {
    const heads = git(["rev-list", "--all", "--max-count=1"], root).trim();
    if (heads) base = git(["rev-parse", "HEAD"], root).trim();
  }
  const previous = base
    ? JSON.parse(git(["show", `${base}:scripts/size-baseline.json`], root))
    : { adopted: {} };
  const previousCounts = Object.create(null);
  const origins = new Map();
  const baseFiles = base ? files(root, { revision: base }) : [];
  for (const file of baseFiles) previousCounts[file.path] = lineCount(file.content);
  if (base) {
    const prefixes = policy("scripts/provenance.json").adoptedPrefixes ?? [];
    const current = policy("scripts/size-baseline.json").adopted;
    const destinations = list.filter((file) => !Object.hasOwn(previousCounts, file.path) &&
      prefixes.some((prefix) => file.path.startsWith(prefix)) &&
      (Object.hasOwn(current, file.path) || lineCount(file.content) > 1000));
    const sizes = destinations.map((file) => file.content.length).sort((a, b) => a - b);
    // Git's 50% similarity requires the smaller blob to be at least half the larger.
    // Keep adopted sources and size-compatible own sources, including short long-line files.
    const sources = baseFiles.filter((file) => {
      if (Object.hasOwn(previous.adopted, file.path)) return true;
      let low = 0, high = sizes.length;
      while (low < high) {
        const middle = (low + high) >>> 1;
        if (sizes[middle] * 2 < file.content.length) low = middle + 1;
        else high = middle;
      }
      return low < sizes.length && sizes[low] <= file.content.length * 2;
    });
    if (sources.length && destinations.length) {
      const paths = [...new Set([...sources, ...destinations].map((file) => file.path))];
      // Literal pathspecs constrain similarity work; no repository-wide candidate cap.
      const changes = git([
        "diff", ...(staged ? ["--cached"] : []), "--name-status", "-z",
        "--find-renames", "--find-copies", "--find-copies-harder", "-l0", base, "--",
        ...paths.map((path) => ":(literal)" + path),
      ], root).split("\0");
      for (let i = 0; i < changes.length && changes[i];) {
        const status = changes[i++];
        const from = changes[i++];
        if (/^[RC]/.test(status)) origins.set(changes[i++], from);
      }
    }
  }
  const checks = {
    size: () =>
      sizeProblems(
        list,
        policy("scripts/size-baseline.json"),
        previous,
        previousCounts,
        policy("scripts/provenance.json"),
        origins,
      ),
    provenance: () => provenanceProblems(list, policy("scripts/provenance.json")),
    assets: () => assetProblems(list, policy("scripts/assets.json")),
    license: () => licenseProblems(list, policy("scripts/license-policy.json"), root),
  };
  if (only && !checks[only]) throw new Error("unknown-check");
  const found = (only ? [checks[only]] : Object.values(checks)).flatMap((check) => check());
  return found;
}
if (main(import.meta.url))
  run(() => {
    const args = process.argv.slice(2);
    const only = args.includes("--only") ? args[args.indexOf("--only") + 1] : undefined;
    const base = args.includes("--base") ? args[args.indexOf("--base") + 1] : process.env.CHECK_BASE_SHA;
    finish(inspectStructure(process.cwd(), { only, base: base || undefined, staged: args.includes("--staged") }));
  });
