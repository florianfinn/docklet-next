import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const root = new URL("../", import.meta.url);
const excludedFiles = new Map([
  ["backend/internal/config/buildables_config.go", ["AUTO_LOGIN_USERNAME", "AUTO_LOGIN_PASSWORD", "AUTO_LOGIN_PASSWORD_FILE", "AUTO_LOGIN_PASSWORD__FILE"]],
]);
const commentedConfigOverrides = new Set(["DOCKER_HOST", "PROJECTS_DIRECTORY", "HTTP_CLIENT_TIMEOUT"]);
// Keep optional M15 proxy settings unset in copied examples; empty mapping enables users.
const commentedStartupOptions = new Set(["AUTH_PROXY_ADMIN_USERS", "AUTH_PROXY_ADMIN_USERS_FILE", "AUTH_PROXY_ADMIN_USERS__FILE", "AUTH_PROXY_LOGOUT_URL", "AUTH_PROXY_LOGOUT_URL_FILE", "AUTH_PROXY_LOGOUT_URL__FILE"]);

// The CLI has its own process environment; this guard covers hub and agent runtime sources.
// Docker client.FromEnv and http.ProxyFromEnvironment use library-defined keys, not product keys.

function goFiles(directory) {
  return readdirSync(new URL(directory, root), { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return goFiles(`${path}/`);
    if (!entry.name.endsWith(".go") || entry.name.endsWith("_test.go")) return [];
    if (excludedFiles.has(path)) return [];
    return [path];
  });
}

function names(source, pattern) {
  return [...source.matchAll(pattern)].map((match) => match[1]);
}

test(".env.example covers hub and agent runtime environment keys exactly", () => {
  const files = goFiles("backend/");
  for (const [path, excluded] of excludedFiles) {
    const contents = readFileSync(new URL(path, root), "utf8");
    const tags = names(contents, /`env:"([A-Za-z_][A-Za-z_0-9]*)"/g);
    const direct = names(contents, /\b(?:os\.(?:Getenv|LookupEnv)|getenv)\("([A-Za-z_][A-Za-z_0-9]*)"\)/g);
    const fileTags = names(contents, /`env:"([A-Za-z_][A-Za-z_0-9]*)"[^`]*options:"[^"]*\bfile\b[^"]*"/g);
    const actual = [...tags, ...direct, ...fileTags.flatMap((key) => [`${key}_FILE`, `${key}__FILE`])].sort();
    assert.deepEqual(actual, [...excluded].sort(), `excluded runtime keys changed in ${path}`);
  }
  const source = files.map((path) => readFileSync(new URL(path, root), "utf8")).join("\n");
  const tags = names(source, /`env:"([A-Za-z_][A-Za-z_0-9]*)"/g);
  const direct = names(source, /\b(?:os\.(?:Getenv|LookupEnv)|getenv)\("([A-Za-z_][A-Za-z_0-9]*)"\)/g);
  const expected = new Set([...tags, ...direct]);
  const requiredActive = new Set(expected);
  const settings = readFileSync(new URL("backend/internal/settings/model.go", root), "utf8");
  const retiredSource = readFileSync(new URL("backend/internal/settings/vulnerability.go", root), "utf8");
  const retiredBody = retiredSource.match(/func retiredVulnerabilitySetting\([\s\S]*?\n\}/)?.[0];
  assert.ok(retiredBody, "vulnerability retirement producer must exist");
  const retired = new Set([...retiredBody.matchAll(/case\s+([^:\n]+):/g)]
    .flatMap((match) => names(match[1], /"([^"]+)"/g)));
  assert.equal(retired.size, 7, "reviewed vulnerability retirement vocabulary changed");
  const settingFields = names(settings, /`key:"([^"]+)"/g)
    .map((tag) => tag.split(","))
    .filter((parts) => !parts.slice(1).includes("internal") && !retired.has(parts[0]));
  const envOverrides = new Set();
  for (const [key, ...attributes] of settingFields) {
    const name = key.replace(/([a-z0-9])([A-Z])/g, "$1_$2").toUpperCase();
    if (attributes.includes("envOverride")) envOverrides.add(name);
    expected.add(name);
    expected.add(`${name}_FILE`);
    expected.add(`${name}__FILE`);
  }
  const fileOptionTags = names(source, /`env:"([A-Za-z_][A-Za-z_0-9]*)"[^`]*options:"[^"]*\bfile\b[^"]*"/g);
  for (const key of fileOptionTags) {
    expected.add(`${key}_FILE`);
    expected.add(`${key}__FILE`);
    requiredActive.add(`${key}_FILE`);
    requiredActive.add(`${key}__FILE`);
  }

  const example = readFileSync(new URL(".env.example", root), "utf8");
  const entries = names(example, /^(?:# )?([A-Za-z_][A-Za-z_0-9]*)=/gm);
  const actual = new Set(entries);
  const active = new Set(names(example, /^([A-Za-z_][A-Za-z_0-9]*)=/gm));
  assert.equal(entries.length, actual.size, ".env.example contains duplicate keys");
  assert.deepEqual([...expected].filter((key) => !actual.has(key)).sort(), [], "missing runtime keys");
  assert.deepEqual([...actual].filter((key) => !expected.has(key)).sort(), [], "unused example keys");
  assert.deepEqual([...requiredActive].filter((key) => !active.has(key) && !commentedConfigOverrides.has(key) && !commentedStartupOptions.has(key)).sort(), [], "config and direct runtime keys must be active");
  assert.deepEqual([...requiredActive].filter((key) => envOverrides.has(key)).sort(), [...commentedConfigOverrides].sort(), "config-backed settings override allowlist changed");
  assert.deepEqual([...commentedConfigOverrides].filter((key) => !requiredActive.has(key) || active.has(key)).sort(), [], "config-backed settings overrides must stay commented");
  assert.deepEqual([...commentedStartupOptions].filter((key) => !requiredActive.has(key) || active.has(key)).sort(), [], "optional proxy settings must stay unset");
  assert.deepEqual([...envOverrides].filter((key) => active.has(key)).sort(), [], "settings overrides must stay commented");
  assert.deepEqual([...expected].filter((key) => !requiredActive.has(key) && active.has(key)).sort(), [], "settings-only keys must stay commented");
});
