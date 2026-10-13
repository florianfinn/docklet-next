import { test } from "node:test";
import { strict as assert } from "node:assert";
import { createHash } from "node:crypto";
import { migrationProblems } from "../scripts/check-migrations.mjs";

test("shipped migration hash and version order are immutable", () => {
  const baseline = Buffer.from("-- baseline\n");
  const hash = createHash("sha256").update(baseline).digest("hex");
  const manifest = { "001_baseline.sql": hash };
  assert.deepEqual(migrationProblems(new Map([["001_baseline.sql", baseline]]), manifest), []);
  assert.ok(migrationProblems(new Map([["001_baseline.sql", Buffer.from("-- changed\n")]]), manifest)
    .includes("changed:001_baseline.sql"));
  assert.ok(migrationProblems(new Map(), manifest).includes("deleted:001_baseline.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline], ["000_old.sql", baseline]]), manifest)
    .includes("invalid-version:000_old.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline], ["001_extra.sql", baseline]]), manifest)
    .includes("invalid-version:001_extra.sql"));
  assert.ok(migrationProblems(new Map([["001_baseline.sql", baseline], ["002_next.sql", baseline]]), manifest)
    .includes("unmanifested:002_next.sql"));
});
