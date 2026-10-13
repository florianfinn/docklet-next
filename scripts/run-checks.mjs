import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { git, tracked, main, run } from "./repository.mjs";

const GO_COMMANDS = {
  lint: ["vet", "./..."],
  test: ["test", "./..."],
  build: ["build", "./..."],
};

function command(program, args, cwd = process.cwd()) {
  const result = spawnSync(program, args, { cwd, stdio: "inherit" });
  if (result.status !== 0) throw new Error("command-failed");
}
export function check(mode) {
  if (!["lint", "test", "text", "build"].includes(mode)) throw new Error("unknown-check");
  if (git(["config", "--get", "core.hooksPath"]).trim() !== ".githooks") throw new Error("hooks-not-active");
  if (mode === "lint") {
    for (const path of tracked().filter((p) => p.endsWith(".mjs")))
      command(process.execPath, ["--check", path]);
  }
  if (["lint", "text", "build"].includes(mode)) {
    command(process.execPath, ["scripts/check-publication.mjs", "--worktree"]);
    command(process.execPath, ["scripts/check-structure.mjs"]);
  }
  if (mode === "test" || mode === "text") {
    const tests = tracked().filter((p) => /^tests\/.*\.test\.mjs$/.test(p));
    if (!tests.length) throw new Error("missing-tests");
    command(process.execPath, ["--test", ...tests]);
  }
  // Module owners extend these integration points when their first package lands.
  if (mode !== "text" && existsSync("backend/go.mod")) {
    command("go", GO_COMMANDS[mode], "backend");
  }
  if (mode !== "text" && existsSync("web/package.json")) command("pnpm", ["--dir", "web", "run", mode]);
  if (mode === "build" && !existsSync("backend/go.mod") && !existsSync("web/package.json"))
    console.log("build [foundation-only; no product artefact]");
}
if (main(import.meta.url)) run(() => check(process.argv[2]));
