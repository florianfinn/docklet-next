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
// Scratch repositories contain guard infrastructure, never imported product trees.
const FOUNDATION_FILES = new Set([
  "package.json", "pnpm-lock.yaml", "LICENSE", "NOTICE", "AGENTS.md",
  "README.md", "CONTRIBUTING.md", "SECURITY.md", ".gitignore", ".editorconfig",
  ".env.example", ".claude/settings.json", "docs/design/publication-policy.md",
  "docs/design/review-workflow.md", "tests/helpers.mjs", "tests/workflow-contracts.mjs",
]);
function foundationPath(path) {
  return FOUNDATION_FILES.has(path) ||
    ["scripts/", ".githooks/", ".github/", "LICENSES/"].some((prefix) => path.startsWith(prefix));
}
function scopePolicies(root, paths) {
  const retained = new Set(paths);
  const has = (path) => retained.has(path);
  const entries = (value) => Object.fromEntries(Object.entries(value).filter(([path]) => has(path)));
  for (const path of ["scripts/assets.json", "scripts/publication-allowlist.json"])
    writeJSON(root, path, readJSON(root, path).filter((entry) => has(entry.path)));
  const size = readJSON(root, "scripts/size-baseline.json");
  size.adopted = entries(size.adopted);
  size.generated = size.generated.filter(has);
  writeJSON(root, "scripts/size-baseline.json", size);
  const provenance = readJSON(root, "scripts/provenance.json");
  provenance.files = provenance.files.filter(has);
  provenance.sidecars = entries(provenance.sidecars);
  if (provenance.snapshots) provenance.snapshots = Object.fromEntries(
    Object.entries(provenance.snapshots).filter(([prefix]) => paths.some((path) => path.startsWith(prefix))),
  );
  // Keep prefix rules so explicitly added product fixtures still require provenance.
  writeJSON(root, "scripts/provenance.json", provenance);
  const license = readJSON(root, "scripts/license-policy.json");
  license.artifactCopies = license.artifactCopies.filter(has);
  writeJSON(root, "scripts/license-policy.json", license);
}
export function scratch(t, source = ROOT) {
  const root = mkdtempSync(resolve(tmpdir(), "docklet-case-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const paths = git(source, "ls-files", "-z").split("\0").filter(foundationPath);
  for (const path of paths) {
    write(root, path, readFileSync(resolve(source, path)));
    if (path.startsWith(".githooks/")) chmodSync(resolve(root, path), 0o755);
  }
  scopePolicies(root, paths);
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

// Explicit PATH stubs exercise module dispatch without depending on installed tools.
export function stubModuleChecks(root) {
  for (const program of ["go", "pnpm"]) {
    const path = `.git/tools/${program}`;
    write(root, path, `#!/usr/bin/env node
import { appendFileSync } from "node:fs";
import { basename, relative } from "node:path";
const program = basename(process.argv[1]);
let args = process.argv.slice(2);
// Record the tool command inside the Docker-isolating Go launcher.
if (program === "go" && args[0] === "run" && basename(args[1]) === "go-command.go") {
  args = args.slice(3);
  if (args[0] === "--work-off") args = args.slice(1);
}
appendFileSync(process.env.MODULE_CHECK_LOG, JSON.stringify({
  program, args, cwd: relative(process.env.MODULE_CHECK_ROOT, process.cwd()),
}) + "\\n");
if ([program, ...args].join(" ") === process.env.MODULE_CHECK_FAIL) process.exit(1);
`);
    chmodSync(resolve(root, path), 0o755);
  }
  return { ...env, PATH: `${resolve(root, ".git/tools")}:${env.PATH}`,
    MODULE_CHECK_ROOT: root, MODULE_CHECK_LOG: resolve(root, ".git/module-checks.log") };
}
