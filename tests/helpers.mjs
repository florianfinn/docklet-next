import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, chmodSync, rmSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";
import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";

export const ROOT = fileURLToPath(new URL("../", import.meta.url));
export function cleanEnvironment(input = process.env) {
  return Object.fromEntries(Object.entries(input).filter(([key]) =>
    !/^(?:GIT_|GITHUB_|RUNNER_|ACTIONS_|CI(?:_|$)|NODE_|NPM_|npm_|PNPM_|CHECK_BASE_SHA$)/.test(key)));
}
const env = cleanEnvironment();
export function invoke(root, program, args, options = {}) {
  return spawnSync(program, args, { cwd: root, encoding: "utf8", env, ...options });
}
export function ok(result) {
  assert.equal(result.status, 0, result.stdout + result.stderr);
  return result.stdout.trim();
}
export function fails(result, category) {
  assert.equal(result.status, 1, `expected guard failure: ${result.stdout}${result.stderr}`);
  if (category) assert.match(result.stdout + result.stderr, new RegExp(`\\[${category}\\]`));
}
export function write(root, path, content) {
  mkdirSync(dirname(resolve(root, path)), { recursive: true });
  writeFileSync(resolve(root, path), content);
}
export const git = (root, ...args) => ok(invoke(root, "git", args));
export const node = (root, script, ...args) => invoke(root, process.execPath, [script, ...args]);
export function scratch(t) {
  const root = mkdtempSync(resolve(tmpdir(), "docklet-case-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const paths = git(ROOT, "ls-files", "-z").split("\0").filter(Boolean);
  for (const path of paths) {
    if (path.startsWith("tests/")) continue;
    write(root, path, readFileSync(resolve(ROOT, path)));
    if (path.startsWith(".githooks/")) chmodSync(resolve(root, path), 0o755);
  }
  write(root, "tests/smoke.test.mjs", 'import test from "node:test"; test("scratch smoke", () => {});\n');
  git(root, "init", "-b", "main");
  git(root, "config", "user.name", "Example Contributor");
  git(root, "config", "user.email", "123+example@users.noreply.github.com");
  ok(node(root, "scripts/setup-git-hooks.mjs"));
  git(root, "add", ".");
  return root;
}
export function stage(root, path, content) {
  write(root, path, content);
  git(root, "add", "--", path);
}
export function readJSON(root, path) {
  return JSON.parse(readFileSync(resolve(root, path), "utf8"));
}
export function writeJSON(root, path, value) {
  write(root, path, JSON.stringify(value, null, 2) + "\n");
}
// Plumbing creates deliberately invalid history without weakening the tested hooks.
export function rawCommit(root, message = "test: synthetic history", identity = {}) {
  const tree = git(root, "write-tree");
  const parent = invoke(root, "git", ["rev-parse", "--verify", "HEAD"]);
  const args = ["commit-tree", tree, ...(parent.status === 0 ? ["-p", parent.stdout.trim()] : [])];
  const sha = ok(invoke(root, "git", args, { input: message + "\n", env: { ...env, ...identity } }));
  git(root, "update-ref", "refs/heads/main", sha);
  return sha;
}
