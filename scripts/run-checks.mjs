import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { git, tracked, main, run } from "./repository.mjs";
import { checkGo, checkGoFormatting } from "./go-checks.mjs";

function command(program, args, cwd = process.cwd()) {
  const result = spawnSync(program, args, { cwd, stdio: "inherit" });
  if (result.status !== 0) throw new Error("command-failed");
}
export function check(mode) {
  if (!["lint", "test", "text", "build"].includes(mode)) throw new Error("unknown-check");
  if (git(["config", "--get", "core.hooksPath"]).trim() !== ".githooks") throw new Error("hooks-not-active");
  if (["lint", "test"].includes(mode) && tracked().includes("web/src/api/generated/schema.ts") &&
      !tracked().includes("api/openapi.json")) throw new Error("missing-tracked-openapi-contract");
  if (mode === "lint") {
    for (const path of tracked().filter((p) => p.endsWith(".mjs")))
      command(process.execPath, ["--check", path]);
  }
  if (["lint", "test", "build"].includes(mode)) command(process.execPath, ["scripts/check-migrations.mjs"]);
  if (mode === "test") command(process.execPath, ["scripts/check-structure.mjs"]);
  if (["lint", "text", "build"].includes(mode)) {
    command(process.execPath, ["scripts/check-publication.mjs", "--worktree"]);
    command(process.execPath, ["scripts/check-structure.mjs"]);
    // The adopted workspace owns the inventory; standalone modules only need
    // their tool checks. Tracked paths also detect missing working-tree files.
    const paths = tracked();
    if (paths.includes("go.work") || paths.includes("docs/provenance/core-import.json"))
      command(process.execPath, ["scripts/inventory-core.mjs", "--check"]);
    if (mode === "lint" && paths.some((path) => path.endsWith(".go"))) checkGoFormatting();
  }
  if (["lint", "test"].includes(mode) && tracked().includes("web/src/api/generated/schema.ts"))
    command(process.execPath, ["scripts/openapi-contract.mjs", "check"]);
  if (mode === "test" || mode === "text") {
    const tests = tracked().filter((p) => /^tests\/.*\.test\.mjs$/.test(p));
    if (!tests.length) throw new Error("missing-tests");
    command(process.execPath, ["--test", ...tests]);
  }
  if (mode !== "text" && existsSync("backend/go.mod")) {
    checkGo(mode);
  }
  if (mode !== "text" && existsSync("web/package.json")) command("pnpm", ["--dir", "web", "run", mode]);
  if (mode === "build" && !existsSync("backend/go.mod") && !existsSync("web/package.json"))
    console.log("build [foundation-only; no product artefact]");
}
if (main(import.meta.url)) run(() => check(process.argv[2]));
