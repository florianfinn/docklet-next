import { test } from "node:test";
import { strict as assert } from "node:assert";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { migrationProblems } from "../scripts/check-migrations.mjs";

test("shipped migration hash and version order are immutable", () => {
  const baseline = Buffer.from("-- baseline\n");
  const hash = createHash("sha256").update(baseline).digest("hex");
  const manifest = { "001_baseline.sql": hash };
  assert.deepEqual(migrationProblems(new Map([["001_baseline.sql", baseline]]), manifest, hash), []);
  assert.ok(migrationProblems(new Map([["001_baseline.sql", Buffer.from("-- changed\n")]]), manifest, hash)
    .includes("changed:001_baseline.sql"));
  assert.ok(migrationProblems(new Map(), manifest, hash).includes("deleted:001_baseline.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline], ["000_old.sql", baseline]]), manifest, hash)
    .includes("invalid-version:000_old.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline], ["001_extra.sql", baseline]]), manifest, hash)
    .includes("invalid-version:001_extra.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline], ["002_next.sql", baseline]]), manifest, hash)
    .includes("unmanifested:002_next.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline]]),
    { "001_baseline.sql": hash }, "0".repeat(64)).includes("changed-baseline"));
});

test("worktree guard rejects all manifest and baseline bypasses", (t) => {
  const source = resolve(import.meta.dirname, "../scripts/check-migrations.mjs");
  const root = mkdtempSync(join(tmpdir(), "docklet-migration-guard-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "scripts"), { recursive: true });
  mkdirSync(join(root, "backend/internal/schema"), { recursive: true });
  mkdirSync(join(root, "backend/resources/migrations/postgres"), { recursive: true });
  copyFileSync(source, join(root, "scripts/check-migrations.mjs"));
  writeFileSync(join(root, "backend/internal/schema/models.go"), "package schema\n");
  const baselinePath = join(root, "backend/resources/migrations/postgres/001_baseline.sql");
  const manifestPath = join(root, "scripts/migration-hashes.json");
  const run = () => spawnSync(process.execPath, [join(root, "scripts/check-migrations.mjs")], { encoding: "utf8" });
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
  writeFileSync(manifestPath, JSON.stringify({ "001_baseline.sql": createHash("sha256").update(changed).digest("hex") }));
  rejects(/changed-baseline/);
  rmSync(baselinePath);
  rejects(/missing-baseline/);
});
