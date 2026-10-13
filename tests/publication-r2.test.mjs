import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { inspectText, inspectPath, safePath } from "../scripts/publication-patterns.mjs";
import { messageProblems } from "../scripts/commit-subject.mjs";
import { FIXTURES } from "../scripts/publication-fixtures.mjs";

const has = (text, category) => inspectText(text).some((f) => f.category === category);
const shapes = (key, value) => [
  `${key}=${value}`,
  `${key}: ${value}`,
  `"${key}": "${value}"`,
  `'${key}': '${value}'`,
  `--${key}=${value}`,
  `--${key} ${value}`,
  `--${key}='${value}'`,
  `--${key} "${value}"`,
];
for (const name of [
  "secret",
  "password",
  "passwd",
  "token",
  "api_key",
  "apikey",
  "private_key",
  "client_secret",
  "api-key",
  "private-key",
]) {
  test(`sensitive name class ${name} accepts all affixes and assignment forms`, () => {
    for (const key of [
      name,
      `prefix_${name}`,
      `${name}_suffix`,
      `prefix${name}suffix`,
      `prefix.${name}`,
      name.toUpperCase(),
    ]) {
      for (const value of [
        "x",
        "abc12345",
        "abcdefghijklmnop",
        "${VAR}suffix",
        "<example>suffix",
        "***suffix",
        "aaaaaaa",
      ])
        for (const input of shapes(key, value)) assert.equal(has(input, "literal-secret"), true);
      for (const value of ["<your password>", "${VAR}", "changeme", "example", "", "test-only", "***"])
        for (const input of shapes(key, value)) assert.equal(has(input, "literal-secret"), false);
    }
  });
}
test("common credential names and nested URL assignments are detected", () => {
  for (const key of [
    "JWT_SECRET",
    "SECRET_KEY",
    "SESSION_SECRET",
    "refresh_token",
    "SLACK_BOT_TOKEN",
    "AUTH_TOKEN",
    "private_key",
  ]) {
    for (const input of shapes(key, "abcdefgh12345678")) assert.equal(has(input, "literal-secret"), true);
  }
  const input = "https://example.com/?value=" + shapes("access_token", "abcdefgh12345678")[0];
  assert.equal(has(input, "literal-secret"), true);
  assert.deepEqual(inspectText(shapes("GITHUB_TOKEN", "${{ github.token }}")[0]), []);
});
test("leading punctuation cannot hide a private hostname", () => {
  const host = FIXTURES["private-hostname"][0];
  for (const prefix of [".", "...", "wait...", "see..", "x:.", "../"])
    assert.equal(has(prefix + host, "private-hostname"), true);
  for (const input of [".env.local", "web/.env.local", ".env.example", "host.internal.js", "example.com"])
    assert.equal(has(input, "private-hostname"), false);
});
test("wildcard listeners and ambiguous scope names stay clean", () => {
  for (const input of ["0.0.0.0:80", "[::]:8080", "::", "a::b", "std::abc"])
    assert.deepEqual(inspectText(input), []);
  assert.equal(has("[" + "a::b" + "]", "network-address"), true);
  const address = FIXTURES["network-address"][0];
  for (const input of [address + "a", "x" + address, address + "."])
    assert.equal(has(input, "network-address"), true);
});
for (const code of [
  0x200b, 0x200c, 0x200d, 0x200e, 0x200f, 0x202a, 0x202b, 0x202c, 0x202d, 0x202e, 0x2060, 0x2061, 0x2062,
  0x2063, 0x2064, 0x2065, 0x2066, 0x2067, 0x2068, 0x2069, 0xfeff,
]) {
  test(`invisible control U+${code.toString(16)} fails in content, name and commit`, () => {
    const value = String.fromCodePoint(code);
    assert.equal(has("text" + value, "corrupted-text"), true);
    assert.equal(inspectPath("file" + value + ".txt")[0].category, "corrupted-text");
    assert.equal(safePath("file" + value + ".txt"), "file-name");
    assert.equal(
      messageProblems("feat: " + value + "x").some((f) => f.category === "corrupted-text"),
      true,
    );
    assert.deepEqual(inspectText("Ordinary text"), []);
  });
}
test("AI qualifiers and GitHub branding cannot hide attribution", () => {
  for (const name of ["Claude AI Assistant", "Codex (GPT-5)", "GitHub Copilot"])
    for (const email of ["", " <12345+bot@users.noreply.github.com>"])
      assert.equal(has("Co-authored-by: " + name + email, "ai-co-author"), true);
  assert.deepEqual(inspectText("Co-authored-by: Claude Martin <123+human@users.noreply.github.com>"), []);
});
test("runtime env names are case insensitive except the exact template spelling", () => {
  for (const path of [
    ".ENV",
    ".Env.local",
    "nested/.ENV",
    ".ENV.example",
    ".env.EXAMPLE",
    ".env.private.env.example",
  ])
    assert.equal(
      inspectPath(path).some((f) => f.category === "runtime-env-file"),
      true,
    );
  assert.deepEqual(inspectPath("nested/.env.example"), []);
});
test("all governance paths require maintainer ownership", () => {
  const owners = readFileSync(new URL("../.github/CODEOWNERS", import.meta.url), "utf8");
  for (const path of [
    "scripts/",
    "tests/",
    "LICENSE",
    "LICENSES/",
    "NOTICE",
    "AGENTS.md",
    ".claude/",
    ".github/",
    ".githooks/",
    "docs/design/",
  ])
    assert.equal(owners.split("\n").includes(`/${path} @florianfinn`), true);
});
