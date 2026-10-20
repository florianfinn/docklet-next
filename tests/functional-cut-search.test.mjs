import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const root = new URL("../", import.meta.url);
const manifest = JSON.parse(readFileSync(new URL("../docs/provenance/removed-feature-search.json", import.meta.url), "utf8"));
const provenance = /:\d+:\s*\/\/ (?:Source|Revision|Retrieved|License|Deviations):/;

// git grep searches tracked files only and is available wherever the repository is checked out.
function search(pattern, paths, globs = [], revision, caseSensitive = false) {
  const pathspecs = [];
  const includes = globs.filter((glob) => !glob.startsWith("!"));
  const excludes = globs.filter((glob) => glob.startsWith("!")).map((glob) => glob.slice(1));
  for (const path of paths) {
    if (includes.length === 0) pathspecs.push(`:(glob)${path}`);
    for (const glob of includes) pathspecs.push(`:(glob)${path}/**/${glob}`);
  }
  for (const glob of excludes) pathspecs.push(`:(exclude,glob)${glob}`);
  const args = ["grep", "--no-color", "--line-number", ...(caseSensitive ? [] : ["--ignore-case"]), "--extended-regexp", "-e", pattern, ...(revision ? [revision] : []), "--", ...pathspecs];
  const result = spawnSync("git", args, { cwd: root, encoding: "utf8" });
  if (result.error) throw result.error;
  assert.ok(result.status === 0 || result.status === 1, result.stderr);
  return result.stdout.trim().split("\n").filter(Boolean).filter((line) => !provenance.test(line));
}

test("removed feature identifiers have expected-empty source and configuration searches", () => {
  for (const query of manifest.queries) {
    // Decision #60 restores SQLite; its positive graph contract is tested separately.
    if (query.feature === "SQLite runtime") {
      assert.deepEqual(search(query.pattern, manifest.scope, manifest.globs,
        "1ead6d2c7e17c79b46c42eb8567c3a1d70730fd5"), query.expectedMatches);
      continue;
    }
    assert.deepEqual(search(query.pattern, manifest.scope, manifest.globs, undefined, query.caseSensitive), query.expectedMatches, query.feature);
  }
  for (const query of manifest.environment.queries) {
    assert.deepEqual(search(query.pattern, manifest.environment.scope), query.expectedMatches, query.feature);
  }
});

test("authentication search preserves acronym identifiers without spanning StreamFailure words", () => {
  const query = manifest.queries.find(({ feature }) => feature === "OIDC and passkeys");
  const positive = ["OIDCConfig", "oidcConfig", "PasskeyService", "passkeyService", "WebAuthn", "webauthn", "MFAConfig", "MfaEnabled", "mfa_secret", "enableMFA", "enableMfaAuthentication", "mfaconfig"];
  const negative = ["RuntimeStreamFailure", "streamFailure"];
  const directory = mkdtempSync(join(tmpdir(), "functional-cut-"));
  try {
    writeFileSync(join(directory, "feature.go"), [...positive, ...negative].join("\n") + "\n");
    const result = spawnSync("git", ["grep", "--no-index", "--no-color", ...(query.caseSensitive ? [] : ["--ignore-case"]), "--extended-regexp", "-e", query.pattern, "--", "feature.go"], { cwd: directory, encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(result.stdout.trim().split("\n").map((line) => line.slice("feature.go:".length)), positive);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test("checked-in OpenAPI excludes removed settings and webhook enum values", () => {
  const document = JSON.parse(readFileSync(new URL("../api/openapi.json", import.meta.url), "utf8"));
  const schemas = document.components.schemas;
  assert.equal(Object.hasOwn(schemas.SettingsUpdate.properties, "gitOperationTimeout"), false);
  assert.deepEqual(schemas.WebhookCreateInput.properties.actionType.enum,
    ["update", "start", "stop", "restart", "redeploy", "up", "down", "run"]);
  assert.equal(/gitOperationTimeout|GIT_OPERATION_TIMEOUT|GIT_WORK_DIR/.test(JSON.stringify(document)), false);
});
