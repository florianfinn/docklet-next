import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = new URL("../", import.meta.url);
const command = fileURLToPath(new URL("../scripts/go-command.go", import.meta.url));
const inventory = JSON.parse(readFileSync(new URL("../docs/provenance/core-import.json", import.meta.url), "utf8"));
const forbidden = new Set(["github.com/libtnb/sqlite", "github.com/mattn/go-sqlite3", "modernc.org/sqlite", "modernc.org/libc"]);
const laterPackages = new Set([
  "github.com/florianfinn/docklet-next/types/capability",
  "github.com/florianfinn/docklet-next/types/contract",
]);

test("hub and agent binary source graphs exclude SQLite and modernc libc", () => {
  for (const target of inventory.targets) {
    const output = execFileSync("go", ["run", command, "--", "list", "-deps", ...target.tags,
      "-f", "{{if .Module}}{{.ImportPath}}|{{.Module.Path}}{{end}}", "./backend/cmd/..."], {
      cwd: root,
      encoding: "utf8",
      maxBuffer: 32 * 1024 * 1024,
      env: { ...process.env, CGO_ENABLED: target.cgo, GOOS: "linux", GOARCH: "amd64" },
    });
    const packages = output.trim().split("\n").filter(Boolean);
    const paths = packages.map((line) => line.split("|")[0]).sort();
    const originalPaths = paths.filter((path) => !laterPackages.has(path));
    assert.equal(originalPaths.length, target.packages, `${target.target} historical package count`);
    assert.deepEqual(paths.filter((path) => laterPackages.has(path)), [...laterPackages].sort());
    const digest = createHash("sha256").update(originalPaths.join("\n") + "\n").digest("hex");
    assert.equal(digest, target.packageGraphSha256, `${target.target} package graph`);
    for (const line of packages) {
      const module = line.split("|")[1];
      assert.equal(forbidden.has(module), false, `${target.target} includes ${module}`);
    }
  }
});
