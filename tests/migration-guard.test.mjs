import { test } from "node:test";
import { strict as assert } from "node:assert";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync, copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { migrationProblems, baseManifestProblems } from "../scripts/check-migrations.mjs";

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
  writeFileSync(manifestPath, JSON.stringify({ "sqlite/001_baseline.sql": createHash("sha256").update(changed).digest("hex") }));
  rejects(/changed-baseline/);
  rmSync(baselinePath);
  rejects(/missing-baseline/);
});

test("one-time transition accepts only the exact seven-hash PostgreSQL base", () => {
 const prior = {"001_baseline.sql": "c39ff6da5f2a456e6f63261c4ec6ad13b584dc53fd5d4b501b6ca9638f573554", "002_run_traces.sql": "0f5b2c64b41be772a14b1231434d67ea21f28df3eca6ff2b952a2bbdb378d48b", "003_sync_receipt_lifecycle.sql": "671bcfc6ac6486430b793bf3d6c178a0c1c6f1fb18cc5b7eff61e1bb5180a578", "004_gate_owner.sql": "f3c8fdb6a543a7af2b70a8bea81354d9251e1214fe50a1bbe1f9081a64f08709", "005_gate_lease.sql": "61c4116c28a808fe45a4577a366835e8d842e67428302c3b213aa3cb733d9f89", "006_single_hub_gate.sql": "dbf959161d6b5554d211007cb84669843b3044a63b2b073d86434b2fccb8e67a", "007_activity_raw_output.sql": "51c48b1468886cb1725eadeb4f463ffc07c7f6b725466afe138728fed107efa8"};
 const next = JSON.parse(readFileSync(new URL("../scripts/migration-hashes.json", import.meta.url)));
 assert.deepEqual(baseManifestProblems(prior, next), []);
 assert.ok(baseManifestProblems({...prior, "001_baseline.sql": "0".repeat(64)}, next).length);
 assert.ok(baseManifestProblems({...prior, "008_unrelated.sql": "1".repeat(64)}, next).length);
 assert.ok(baseManifestProblems(prior, {...next, "sqlite/001_baseline.sql": "0".repeat(64)}).length);
 assert.ok(baseManifestProblems(next, {}).length);
 assert.ok(baseManifestProblems(next, {...next, "sqlite/001_baseline.sql": "0".repeat(64)}).length);
});
