import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const root = new URL("../", import.meta.url);
const command = fileURLToPath(new URL("../scripts/go-command.go", import.meta.url));
const inventory = JSON.parse(readFileSync(new URL("../docs/provenance/core-import.json", import.meta.url), "utf8"));
const forbidden = new Set(["github.com/jackc/pgx/v5", "github.com/jackc/pgerrcode", "github.com/jackc/pgpassfile", "github.com/jackc/pgservicefile", "github.com/jackc/puddle/v2", "gorm.io/driver/postgres"]);

test("hub and agent graphs use pure-Go SQLite and exclude PostgreSQL", () => {
  for (const target of inventory.targets) {
    const output = execFileSync("go", ["run", command, "--", "list", "-deps", ...target.tags,
      "-f", "{{if .Module}}{{.ImportPath}}|{{.Module.Path}}{{end}}", "./backend/cmd"], {
      cwd: root,
      encoding: "utf8",
      maxBuffer: 32 * 1024 * 1024,
      env: { ...process.env, CGO_ENABLED: target.cgo, GOOS: "linux", GOARCH: "amd64" },
    });
    const packages = output.trim().split("\n").filter(Boolean);
    const paths = packages.map((line) => line.split("|")[0]).sort();
    assert.equal(target.cgo,"0");
    assert.equal(target.entrypoint,"./backend/cmd");
    assert.equal(paths.includes("github.com/florianfinn/docklet-next/backend/cmd/schema-baseline"),false);
    assert.equal(paths.length,target.packages);
    const digest=createHash("sha256").update(paths.join("\n")+"\n").digest("hex");
    assert.equal(digest,target.packageGraphSha256,`${target.target} package graph`);
    for(const module of ["github.com/libtnb/sqlite","modernc.org/sqlite","modernc.org/libc"])
      assert.ok(packages.some(line=>line.split("|")[1]===module),module);
    const inspected=JSON.parse(execFileSync("go",["run",command,"--","run","scripts/core-license-files.go","inspect",...target.tags.map(flag=>flag.replace("-tags=",""))],{cwd:root,encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"}}));
    const expected=JSON.parse(readFileSync(new URL("../docs/provenance/core-compiled-notices.json",import.meta.url))).files.map(({module,version,path,sha256})=>({module,version,path,sha256}));
    assert.deepEqual(inspected,expected,`${target.target} compiled licence evidence`);
    for (const line of packages) {
      const module = line.split("|")[1];
      assert.equal(forbidden.has(module) || line.includes("/components/postgres"), false, `${target.target} includes ${module}`);
    }
  }
});
