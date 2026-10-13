import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const migrationDir = "backend/resources/migrations/postgres";
const baseline = "001_baseline.sql";

export function migrationProblems(files, manifest) {
  const problems = [];
  const names = Object.keys(manifest).sort();
  if (!names.includes(baseline)) problems.push("missing-baseline-manifest");
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

export function checkMigrations() {
  const staged = process.argv.includes("--staged");
  const manifestPath = join(root, "scripts/migration-hashes.json");
  if (staged) {
    const manifestBlob = spawnSync("git", ["show", ":scripts/migration-hashes.json"], { cwd: root });
    if (manifestBlob.status !== 0) {
      const baselineBlob = spawnSync("git", ["show", `:${migrationDir}/${baseline}`], { cwd: root, stdio: "ignore" });
      if (baselineBlob.status === 0) throw new Error("migration-integrity: deleted manifest");
      return;
    }
    const manifest = JSON.parse(manifestBlob.stdout.toString("utf8"));
    const listing = spawnSync("git", ["ls-files", "-z", "--", migrationDir], { cwd: root, encoding: "utf8" });
    if (listing.status !== 0) throw new Error("migration-integrity: cannot read index");
    const files = new Map(listing.stdout.split("\0").filter(Boolean).map((path) => {
      const blob = spawnSync("git", ["show", `:${path}`], { cwd: root });
      if (blob.status !== 0) throw new Error(`migration-integrity: unreadable staged file ${path}`);
      return [path.slice(migrationDir.length + 1), blob.stdout];
    }));
    const problems = migrationProblems(files, manifest);
    if (problems.length) throw new Error(`migration-integrity: ${problems.join(", ")}`);
    return;
  }
  if (!existsSync(manifestPath)) {
    const tracked = spawnSync("git", ["ls-files", "--error-unmatch", "scripts/migration-hashes.json"],
      { cwd: root, stdio: "ignore" });
    if (tracked.status === 0) throw new Error("migration-integrity: deleted manifest");
    return;
  }
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  const files = new Map(readdirSync(join(root, migrationDir)).filter((name) => name.endsWith(".sql"))
    .map((name) => [name, readFileSync(join(root, migrationDir, name))]));
  const problems = migrationProblems(files, manifest);
  if (problems.length) throw new Error(`migration-integrity: ${problems.join(", ")}`);
}

if (process.argv[1] && relative(root, process.argv[1]) === "scripts/check-migrations.mjs") {
  checkMigrations();
}
