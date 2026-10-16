import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { basename, dirname, join, relative } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const migrationDir = "backend/resources/migrations";
const baseline = "sqlite/001_baseline.sql";
// Independent of the manifest so rehashing a shipped baseline cannot certify it.
const baselineHash = "bdd3f75d6f7e48bf9749e41f1dd6bd7c81d8142f832c43894052887ebe91ddb2";

export function migrationProblems(files, manifest, pinnedBaselineHash = baselineHash) {
  const problems = [];
  const names = Object.keys(manifest).sort();
  if (!names.includes(baseline)) problems.push("missing-baseline-manifest");
  if (!files.has(baseline)) problems.push("missing-baseline");
  else if (createHash("sha256").update(files.get(baseline)).digest("hex") !== pinnedBaselineHash)
    problems.push("changed-baseline");
  for (const name of names) {
    if (!files.has(name)) problems.push(`deleted:${name}`);
    else if (createHash("sha256").update(files.get(name)).digest("hex") !== manifest[name])
      problems.push(`changed:${name}`);
  }
  const versions = new Set();
  for (const name of files.keys()) {
    if (!/^sqlite\/[^/]+$/.test(name)) problems.push(`invalid-path:${name}`);
    if (!Object.hasOwn(manifest, name)) problems.push(`unmanifested:${name}`);
    const match = /^(\d{3})_[a-z0-9_]+\.sql$/.exec(basename(name));
    if (!match) { problems.push(`invalid-name:${name}`); continue; }
    const version = Number(match[1]);
    if (version < 1 || (name !== baseline && version <= 1)) problems.push(`invalid-version:${name}`);
    if (versions.has(version)) problems.push(`duplicate-version:${name}`);
    versions.add(version);
  }
  return problems;
}

const legacyManifest = {
  "001_baseline.sql": "c39ff6da5f2a456e6f63261c4ec6ad13b584dc53fd5d4b501b6ca9638f573554",
  "002_run_traces.sql": "0f5b2c64b41be772a14b1231434d67ea21f28df3eca6ff2b952a2bbdb378d48b",
  "003_sync_receipt_lifecycle.sql": "671bcfc6ac6486430b793bf3d6c178a0c1c6f1fb18cc5b7eff61e1bb5180a578",
  "004_gate_owner.sql": "f3c8fdb6a543a7af2b70a8bea81354d9251e1214fe50a1bbe1f9081a64f08709",
  "005_gate_lease.sql": "61c4116c28a808fe45a4577a366835e8d842e67428302c3b213aa3cb733d9f89",
  "006_single_hub_gate.sql": "dbf959161d6b5554d211007cb84669843b3044a63b2b073d86434b2fccb8e67a",
  "007_activity_raw_output.sql": "51c48b1468886cb1725eadeb4f463ffc07c7f6b725466afe138728fed107efa8"
};

export function baseManifestProblems(previous, manifest) {
  const exact = (a, b) => JSON.stringify(Object.entries(a).sort()) === JSON.stringify(Object.entries(b).sort());
  if (exact(previous, legacyManifest) && exact(manifest, { [baseline]: baselineHash })) return [];
  return Object.entries(previous).filter(([name, hash]) => manifest[name] !== hash)
    .map(([name]) => `changed-shipped:${name}`);
}
function comparisonProblems(manifest) {
 const base = process.env.CHECK_BASE_SHA;
 if (!base) return [];
 if (!/^[a-f0-9]{40}$/.test(base)) throw new Error("migration-integrity: invalid base revision");
 const previous = spawnSync("git", ["show", `${base}:scripts/migration-hashes.json`], {cwd: root});
 if (previous.status !== 0) return [];
 const old = JSON.parse(previous.stdout.toString("utf8"));
 const problems = baseManifestProblems(old, manifest);
 if (!problems.length && Object.hasOwn(old, "001_baseline.sql")) {
  for (const [name, hash] of Object.entries(legacyManifest)) {
   const file = spawnSync("git", ["show", `${base}:${migrationDir}/postgres/${name}`], {cwd: root});
   if (file.status !== 0 || createHash("sha256").update(file.stdout).digest("hex") !== hash)
    problems.push(`changed-transition-source:${name}`);
  }
 }
 return problems;
}
function migrationFiles(dir, prefix = "") {
 return readdirSync(dir, {withFileTypes: true}).flatMap((entry) => entry.isDirectory()
  ? migrationFiles(join(dir, entry.name), prefix + entry.name + "/")
  : entry.name.endsWith(".sql") ? [[prefix + entry.name, readFileSync(join(dir, entry.name))]] : []);
}

export function checkMigrations() {
  const staged = process.argv.includes("--staged");
  const manifestPath = join(root, "scripts/migration-hashes.json");
  if (staged) {
    const listing = spawnSync("git", ["ls-files", "-z", "--", migrationDir], { cwd: root, encoding: "utf8" });
    if (listing.status !== 0) throw new Error("migration-integrity: cannot read index");
    const paths = listing.stdout.split("\0").filter(Boolean);
    const product = spawnSync("git", ["ls-files", "--error-unmatch", "backend/internal/schema/models.go"],
      { cwd: root, stdio: "ignore" }).status === 0;
    const manifestBlob = spawnSync("git", ["show", ":scripts/migration-hashes.json"], { cwd: root });
    if (manifestBlob.status !== 0) {
      if (paths.length || product) throw new Error("migration-integrity: deleted manifest");
      return;
    }
    const manifest = JSON.parse(manifestBlob.stdout.toString("utf8"));
    const files = new Map(paths.map((path) => {
      const blob = spawnSync("git", ["show", `:${path}`], { cwd: root });
      if (blob.status !== 0) throw new Error(`migration-integrity: unreadable staged file ${path}`);
      return [path.slice(migrationDir.length + 1), blob.stdout];
    }));
    const problems = [...migrationProblems(files, manifest), ...comparisonProblems(manifest)];
    if (problems.length) throw new Error(`migration-integrity: ${problems.join(", ")}`);
    return;
  }
  const migrationPath = join(root, migrationDir);
  if (!existsSync(migrationPath) && !existsSync(manifestPath) &&
      !existsSync(join(root, "backend/internal/schema/models.go"))) return;
  if (!existsSync(manifestPath)) {
    throw new Error("migration-integrity: deleted manifest");
  }
  if (!existsSync(migrationPath)) {
    throw new Error("migration-integrity: deleted baseline");
  }
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  const files = new Map(migrationFiles(join(root, migrationDir)));
  const problems = [...migrationProblems(files, manifest), ...comparisonProblems(manifest)];
  if (problems.length) throw new Error(`migration-integrity: ${problems.join(", ")}`);
}

if (process.argv[1] && relative(root, process.argv[1]) === "scripts/check-migrations.mjs") {
  checkMigrations();
}
