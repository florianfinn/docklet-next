import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const root = new URL("../", import.meta.url);

function goFiles(directory) {
  return readdirSync(new URL(directory, root), { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return goFiles(`${path}/`);
    if (!entry.name.endsWith(".go") || entry.name.endsWith("_test.go")) return [];
    if (path.startsWith("backend/internal/testpg/") || path.endsWith("buildables_config.go")) return [];
    return [path];
  });
}

function names(source, pattern) {
  return [...source.matchAll(pattern)].map((match) => match[1]);
}

test(".env.example covers hub and agent runtime environment keys exactly", () => {
  const files = goFiles("backend/");
  const source = files.map((path) => readFileSync(new URL(path, root), "utf8")).join("\n");
  const tags = names(source, /`env:"([A-Za-z_][A-Za-z_0-9]*)"/g);
  const direct = names(source, /\b(?:os\.(?:Getenv|LookupEnv)|getenv)\("([A-Za-z_][A-Za-z_0-9]*)"\)/g);
  const expected = new Set([...tags, ...direct]);
  const settings = readFileSync(new URL("backend/internal/settings/model.go", root), "utf8");
  const settingFields = names(settings, /`key:"([^"]+)"/g)
    .map((tag) => tag.split(","))
    .filter((parts) => !parts.slice(1).includes("internal"));
  for (const [key] of settingFields) {
    const name = key.replace(/([a-z0-9])([A-Z])/g, "$1_$2").toUpperCase();
    expected.add(name);
    expected.add(`${name}_FILE`);
    expected.add(`${name}__FILE`);
  }
  const fileOptionTags = names(source, /`env:"([A-Za-z_][A-Za-z_0-9]*)"[^`]*options:"[^"]*\bfile\b[^"]*"/g);
  for (const key of fileOptionTags) {
    expected.add(`${key}_FILE`);
    expected.add(`${key}__FILE`);
  }

  const example = readFileSync(new URL(".env.example", root), "utf8");
  const entries = names(example, /^(?:# )?([A-Za-z_][A-Za-z_0-9]*)=/gm);
  const actual = new Set(entries);
  assert.equal(entries.length, actual.size, ".env.example contains duplicate keys");
  assert.deepEqual([...expected].filter((key) => !actual.has(key)).sort(), [], "missing runtime keys");
  assert.deepEqual([...actual].filter((key) => !expected.has(key)).sort(), [], "unused example keys");
});
