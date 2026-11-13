import { test } from "node:test";
import { strict as assert } from "node:assert";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync, copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { cleanEnvironment } from "./helpers.mjs";
import { migrationProblems, baseManifestProblems, prereleaseManifest } from "../scripts/check-migrations.mjs";

test("shipped migration hash and version order are immutable", () => {
  const baseline = Buffer.from("-- baseline\n");
  const hash = createHash("sha256").update(baseline).digest("hex");
  const manifest = { "sqlite/001_baseline.sql": hash };
  assert.deepEqual(migrationProblems(new Map([["sqlite/001_baseline.sql", baseline]]), manifest, hash), []);
  assert.ok(migrationProblems(new Map([["sqlite/001_baseline.sql", Buffer.from("-- changed\n")]]), manifest, hash)
    .includes("changed:sqlite/001_baseline.sql"));
  assert.ok(migrationProblems(new Map(), manifest, hash).includes("deleted:sqlite/001_baseline.sql"));
  assert.ok(migrationProblems(new Map([["sqlite/001_baseline.sql", baseline], ["sqlite/000_old.sql", baseline]]), manifest, hash)
    .includes("invalid-version:sqlite/000_old.sql"));
  assert.ok(migrationProblems(new Map([["sqlite/001_baseline.sql", baseline], ["sqlite/001_extra.sql", baseline]]), manifest, hash)
    .includes("invalid-version:sqlite/001_extra.sql"));
  assert.ok(migrationProblems(new Map([["sqlite/001_baseline.sql", baseline], ["sqlite/002_next.sql", baseline]]), manifest, hash)
    .includes("unmanifested:sqlite/002_next.sql"));
  assert.ok(migrationProblems(new Map([["sqlite/001_baseline.sql", baseline]]),
    { "sqlite/001_baseline.sql": hash }, "0".repeat(64)).includes("changed-baseline"));
});

test("worktree guard rejects all manifest and baseline bypasses", (t) => {
  const source = resolve(import.meta.dirname, "../scripts/check-migrations.mjs");
  const root = mkdtempSync(join(tmpdir(), "docklet-migration-guard-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "scripts"), { recursive: true });
  mkdirSync(join(root, "backend/internal/schema"), { recursive: true });
  mkdirSync(join(root, "backend/resources/migrations/sqlite"), { recursive: true });
  copyFileSync(source, join(root, "scripts/check-migrations.mjs"));
  writeFileSync(join(root, "backend/internal/schema/models.go"), "package schema\n");
  const baselinePath = join(root, "backend/resources/migrations/sqlite/001_baseline.sql");
  const manifestPath = join(root, "scripts/migration-hashes.json");
  const run = () => spawnSync(process.execPath, [join(root, "scripts/check-migrations.mjs")], { encoding: "utf8", env: cleanEnvironment() });
  const rejects = (pattern) => {
    const result = run();
    assert.equal(result.status, 1);
    assert.match(result.stderr, pattern);
  };

  writeFileSync(baselinePath, "-- changed baseline\n");
  rejects(/deleted manifest/);
  rmSync(baselinePath);
  rejects(/deleted manifest/);
  const changed = Buffer.from("-- changed baseline\n");
  writeFileSync(baselinePath, changed);
  writeFileSync(manifestPath, JSON.stringify({ "sqlite/001_baseline.sql": createHash("sha256").update(changed).digest("hex") }));
  rejects(/changed-baseline/);
  rmSync(baselinePath);
  rejects(/missing-baseline/);
  writeFileSync(baselinePath, readFileSync(new URL("../backend/resources/migrations/sqlite/001_baseline.sql", import.meta.url)));
  writeFileSync(manifestPath, readFileSync(new URL("../scripts/migration-hashes.json", import.meta.url)));
  const liveManifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  for (const path of Object.keys(liveManifest)) {
    copyFileSync(new URL(`../backend/resources/migrations/${path}`, import.meta.url), join(root, "backend/resources/migrations", path));
  }
  const complete = run();
  assert.equal(complete.status, 0, complete.stdout + complete.stderr);
  const absentBase = spawnSync(process.execPath, [join(root, "scripts/check-migrations.mjs")], {
    encoding: "utf8", env: { ...cleanEnvironment(), CHECK_BASE_SHA: "f".repeat(40) },
  });
  assert.equal(absentBase.status, 1);
  assert.match(absentBase.stderr, /unreadable base manifest/);
});

test("release transition accepts the exact pinned prerelease chain, with manifested forward entries", () => {
  const release = JSON.parse(readFileSync(new URL("../scripts/migration-hashes.json", import.meta.url)));
  const baselinePath = "sqlite/001_baseline.sql";
  const forwardVersion = Math.max(...Object.keys(release).map((path) => Number(path.match(/\/(\d+)_/)[1]))) + 1;
  const forwardPath = `sqlite/${String(forwardVersion).padStart(3, "0")}_fixture.sql`;
  const releaseFiles = Object.keys(release).map((path) => [path, readFileSync(new URL(`../backend/resources/migrations/${path}`, import.meta.url))]);
  const forwardBytes = Buffer.from("-- +goose Up\nCREATE TABLE fixture(id TEXT);\n");
  const forwardHash = createHash("sha256").update(forwardBytes).digest("hex");
  const next = {...release, [forwardPath]: forwardHash};
  assert.deepEqual(baseManifestProblems(prereleaseManifest, release), []);
  assert.deepEqual(baseManifestProblems(prereleaseManifest, next), []);
  assert.deepEqual(migrationProblems(new Map([...releaseFiles, [forwardPath, forwardBytes]]), next), []);
  assert.deepEqual(baseManifestProblems(release, next), []);
  for (const name of Object.keys(prereleaseManifest)) {
    const altered = {...prereleaseManifest, [name]: "0".repeat(64)};
    assert.ok(baseManifestProblems(altered, release).includes(`changed-shipped:${name}`));
  }
  assert.ok(baseManifestProblems(prereleaseManifest, {...release, [baselinePath]: "0".repeat(64)})
    .includes(`changed-shipped:${baselinePath}`));
  const partial = {...prereleaseManifest};
  delete partial["sqlite/020_remove_webhooks.sql"];
  assert.ok(baseManifestProblems(prereleaseManifest, partial).includes("changed-shipped:sqlite/020_remove_webhooks.sql"));
  assert.ok(baseManifestProblems(prereleaseManifest, {...release, "sqlite/020_remove_webhooks.sql": prereleaseManifest["sqlite/020_remove_webhooks.sql"]}).length);
  assert.ok(migrationProblems(new Map([...releaseFiles, [forwardPath, forwardBytes]]), release)
    .includes(`unmanifested:${forwardPath}`));
  assert.ok(baseManifestProblems(release, {}).includes(`changed-shipped:${baselinePath}`));
  assert.ok(baseManifestProblems(release, {...release, [baselinePath]: "0".repeat(64)}).includes(`changed-shipped:${baselinePath}`));
  assert.deepEqual(baseManifestProblems({}, release), ["invalid-transition-base"]);
});

test("frozen prerelease fixture hashes match the independent transition pin", () => {
  const hashes = JSON.parse(readFileSync(new URL("../backend/internal/schema/testdata/prerelease-chain/hashes.json", import.meta.url)));
  assert.deepEqual(Object.fromEntries(Object.entries(hashes).map(([name, hash]) => [`sqlite/${name}`, hash])), prereleaseManifest);
});
