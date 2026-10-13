import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const migrationDir = "backend/resources/migrations/postgres";
const baseline = "001_baseline.sql";
// Independent of the manifest so rehashing a shipped baseline cannot certify it.
const baselineHash = "c39ff6da5f2a456e6f63261c4ec6ad13b584dc53fd5d4b501b6ca9638f573554";

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
    if (!Object.hasOwn(manifest, name)) problems.push(`unmanifested:${name}`);
    const match = /^(\d{3})_[a-z0-9_]+\.sql$/.exec(name);
    if (!match) { problems.push(`invalid-name:${name}`); continue; }
    const version = Number(match[1]);
    if (version < 1 || (name !== baseline && version <= 1)) problems.push(`invalid-version:${name}`);
    if (versions.has(version)) problems.push(`duplicate-version:${name}`);
    versions.add(version);
  }
  return problems;
}

function baseManifestProblems(manifest) {
  const base = process.env.CHECK_BASE_SHA;
  if (!base) return [];
  if (!/^[a-f0-9]{40}$/.test(base)) throw new Error("migration-integrity: invalid base revision");
  const previous = spawnSync("git", ["show", `${base}:scripts/migration-hashes.json`], { cwd: root });
  if (previous.status !== 0) return [];
  return Object.entries(JSON.parse(previous.stdout.toString("utf8")))
    .filter(([name, hash]) => manifest[name] !== hash)
    .map(([name]) => `changed-shipped:${name}`);
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
    const problems = [...migrationProblems(files, manifest), ...baseManifestProblems(manifest)];
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
  const files = new Map(readdirSync(join(root, migrationDir)).filter((name) => name.endsWith(".sql"))
    .map((name) => [name, readFileSync(join(root, migrationDir, name))]));
  const problems = [...migrationProblems(files, manifest), ...baseManifestProblems(manifest)];
  if (problems.length) throw new Error(`migration-integrity: ${problems.join(", ")}`);
}

if (process.argv[1] && relative(root, process.argv[1]) === "scripts/check-migrations.mjs") {
  checkMigrations();
}
