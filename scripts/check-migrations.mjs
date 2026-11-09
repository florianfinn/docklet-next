import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { basename, dirname, join, relative } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const migrationDir = "backend/resources/migrations";
const baseline = "sqlite/001_baseline.sql";
// Independent of the manifest so rehashing a shipped baseline cannot certify it.
const baselineHash = "817f3ea53b4b15ab07397b69b76790198c2f1a19f97e01280c7efe25bf511ada";

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

// Pinned independently: only this complete prerelease chain may be replaced once.
export const prereleaseManifest = {
  "sqlite/001_baseline.sql": "a07e07cf4bd91985e8f72bc7b21903510426384cfede6a7e18d1b9022937cecd",
  "sqlite/002_hub_backup_archive.sql": "524cb0ebf27896c4831e5b5ee89f3767265fe12835d8f17a99e99e4fb431ad78",
  "sqlite/003_runtime_receipts.sql": "2e1e2103a728617ca1ea772b68faf087b3d9d78bf4f1024c997ffc6ec7776593",
  "sqlite/004_runtime_output.sql": "201c706f1f1bec837cad49d10765d9e6ed8c46863743f5a76cbc696460f4b672",
  "sqlite/005_runtime_registration.sql": "8007eedd540421ded14d14d6d63162cae941a379b90e0dab85c7cbdd61844d50",
  "sqlite/006_image_protection.sql": "6dde1831c62671066954b88c86e1ad5a000c318637dc151c15548fe994c18c5f",
  "sqlite/007_image_lifecycle.sql": "1d1274ec1007d56fb8699e721b1e2935fbe835d1270e35ce452c48b8629f16fe",
  "sqlite/008_restore_retention.sql": "d727135fbd5e6419f195dcd8cd6ddbef486d0ac01afd0f584e1440d1334dcb1e",
  "sqlite/009_update_policies.sql": "b01863370bb14cf82b2865d4787531e28463afc9bae05c2d814c4c3e2e97ebc9",
  "sqlite/010_vulnerability_attempts.sql": "8e26fdcff420f772c29f423426e20fb3680c6263fcaa786a78f625566399cd9f",
  "sqlite/011_vulnerability_notifications.sql": "a8c887b2e5254ddeea1fade74d401ca009cf3569e205f93e0328f9fa60536369",
  "sqlite/012_volume_backup_lifecycle.sql": "56b090eebdd21109bc97548b17eab2eded39c3fa585a373d2862b717e94e4254",
  "sqlite/013_notifications_auto_heal.sql": "7243770613547b4785451281343ff88091b7f81dbd1f97168d16838f9d87f328",
  "sqlite/014_container_host_operations.sql": "20e41700aca7a5f53780ed33c138366b7708139c208c0222c3640e54ef0359db",
  "sqlite/015_simple_users.sql": "b3681f6932e07bb8f3af0331765655f5cba9868ed2e1a45def9dfa0f742f87d2",
  "sqlite/016_runtime_history.sql": "0a6ea63519f1a7cf124fb572b93718f3c7d0d89491e87e18d25e0dab13eda34d",
  "sqlite/017_service_widgets.sql": "a17d03e2fe73b30c8057c3f2a048936287b880ad815babaaf9f46f61c8d2de5c",
  "sqlite/018_update_policy_windows.sql": "2b7429be2f2ff9d0ad34cd4a3b9b260bb2f8ade534a6e6278ee7edd219743412",
  "sqlite/019_named_targets_conditions.sql": "b31e707c021a18929204a49ccccab15e3ee305fcb8ce31499dcf0e58504eaada",
  "sqlite/020_remove_webhooks.sql": "3d426fa10628f63e5f2c91a50f02d285cd752012400c775d4b701c01bc00a521"
};

export function baseManifestProblems(previous, manifest) {
  const exact = (a, b) => JSON.stringify(Object.entries(a).sort()) === JSON.stringify(Object.entries(b).sort());
  const release = manifest[baseline] === baselineHash && Object.keys(manifest).every((name) =>
    name === baseline || (/^sqlite\/(\d{3})_[a-z0-9_]+\.sql$/.test(name) && Number(basename(name).slice(0, 3)) > 1 && (!Object.hasOwn(prereleaseManifest, name) || manifest[name] !== prereleaseManifest[name])));
  // Forward entries may accompany the collapse; migrationProblems verifies their bytes.
  if (exact(previous, prereleaseManifest) && release) return [];
  if (!Object.hasOwn(previous, baseline)) return ["invalid-transition-base"];
  return Object.entries(previous).filter(([name, hash]) => manifest[name] !== hash)
    .map(([name]) => `changed-shipped:${name}`);
}
function comparisonProblems(manifest) {
 const base = process.env.CHECK_BASE_SHA;
 if (!base) return [];
 if (!/^[a-f0-9]{40}$/.test(base)) throw new Error("migration-integrity: invalid base revision");
 const previous = spawnSync("git", ["show", `${base}:scripts/migration-hashes.json`], {cwd: root});
 if (previous.status !== 0) throw new Error("migration-integrity: unreadable base manifest");
 const old = JSON.parse(previous.stdout.toString("utf8"));
 const problems = baseManifestProblems(old, manifest);

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
