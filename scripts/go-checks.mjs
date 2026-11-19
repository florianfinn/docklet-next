import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { tracked, main, run } from "./repository.mjs";

// Modules outside go.work (vendored third_party, isolated tool modules) build with the workspace off.
function workspaceOff(cwd) {
  if (cwd === process.cwd()) return false;
  if (!existsSync("go.work")) return cwd.startsWith("third_party/");
  const used = [...readFileSync("go.work", "utf8").matchAll(/^\s*\.\/(\S+)\s*$/gm)].map((m) => m[1]);
  return !used.includes(cwd);
}
function go(args, cwd = process.cwd(), overrides = {}) {
  // Container wrappers need their transport environment; Go tools and test
  // subprocesses must never inherit a Docker endpoint.
  const env = { ...process.env, ...overrides };
  const launcher = ["run", resolve("scripts/go-command.go"), "--"];
  if (workspaceOff(cwd)) launcher.push("--work-off");
  const result = spawnSync("go", [...launcher, ...args], {
    cwd, env, encoding: "utf8", stdio: ["inherit", "pipe", "inherit"],
  });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.status !== 0) throw new Error(`go-${args[0]}-failed`);
  return result.stdout ?? "";
}
export function checkGo(mode) {
  const modules = tracked().filter((path) => path.endsWith("/go.mod")).map(dirname);
  if (!modules.length) return;
  if (mode === "lint") {
    for (const module of modules) go(["vet", "./..."], module);
  } else if (mode === "test") {
    for (const module of modules) go(["test", "./..."], module);
  } else if (mode === "build") {
    mkdirSync(".scratch/build", { recursive: true });
    go(["build", "-o", ".scratch/build/hub", "./backend/cmd"]);
    go(["build", "-tags=exclude_frontend", "-o", ".scratch/build/agent", "./backend/cmd"], process.cwd(), { CGO_ENABLED: "0" });
    go(["build", "-o", ".scratch/build/schema-baseline", "./backend/cmd/schema-baseline"]);
  } else throw new Error("unknown-go-check");
}
export function checkGoFormatting() {
  // gofmt may only exist inside the same container as the go wrapper.
  go(["run", "./scripts/gofmt-check.go", "--", ...tracked().filter((path) => path.endsWith(".go"))]);
}
if (main(import.meta.url)) run(() => checkGo(process.argv[2]));
