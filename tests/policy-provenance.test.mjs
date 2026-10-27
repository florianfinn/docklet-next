import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import test from "node:test";

test("policy provenance paths exist and never cite the pre-document revision", () => {
  const files = execFileSync("git", ["ls-files", "-z"], { encoding: "utf8" }).split("\0");
  let count = 0;
  for (const file of files.filter((name) => name.endsWith(".go") || name.endsWith("009_update_policies.sql"))) {
    const source = readFileSync(file, "utf8");
    const path = source.match(/^(?:\/\/|--) Source: docklet-next\/(docs\/design\/[^\n]+)/m)?.[1];
    if (!path) continue;
    assert.ok(readFileSync(path, "utf8").length > 0, file);
    if (path !== "docs/design/update-policies.md") continue;
    count++;
    const revision = source.match(/^(?:\/\/|--) Revision: ([a-f0-9]+)/m)?.[1];
    assert.match(revision, /^[a-f0-9]{40}$/);
    assert.notEqual(revision, "c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db", `${file}: revision predates source document`);
  }
  assert.ok(count >= 28);
  const sidecar = JSON.parse(readFileSync("scripts/provenance.json", "utf8")).sidecars["backend/resources/migrations/sqlite/009_update_policies.sql"];
  assert.notEqual(sidecar.revision, "c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db");
});

test("notification states have matching English and German catalogue entries", () => {
  const source = readFileSync("types/updater/policy.go", "utf8");
  const values = source.match(/NotificationState\s+string\s+`[^`]*enum:"([^"]+)"/)[1].split(",");
  assert.ok(values.includes("sending"));
  for (const locale of ["en", "de"]) {
    const catalogue = readFileSync(`web/src/features/images/messages/${locale}.ts`, "utf8");
    const states = catalogue.match(/notification: \{([^}]+)\}/)[1];
    for (const value of values) assert.ok(states.includes(`${value}:`) || states.includes(`"${value}":`), `${locale}: ${value}`);
  }
});
