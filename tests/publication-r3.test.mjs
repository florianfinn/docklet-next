import test from "node:test";
import assert from "node:assert/strict";
import { inspectText } from "../scripts/publication-patterns.mjs";

const has = (text, path) => inspectText(text, path).some((f) => f.category === "literal-secret");
const value = "abcdef123456";
for (const [path, key, separator, close] of [
  ["README.md", "Default password", ": ", "`"],
  ["README.md", "TOKEN", "=", "`"],
  [".env.example", "SECRET_KEY", "=", "`"],
  ["example.yml", "password", ": ", "`"],
  ["example.yml", "password", ": ", ""],
]) test(`B1 backtick literal rejected: ${path} ${key} ${close ? "closed" : "open"}`, () => {
  assert.equal(has(key + separator + "`" + value + close, path), true);
  assert.equal(has(key + separator + "`example" + close, path), false);
});
for (const path of ["example.sh", "example.bash", "example.zsh", "Dockerfile", "deploy/Dockerfile.test"])
  test(`B1 backtick command accepted only in shell context: ${path}`, () => {
    assert.equal(has("TOKEN=" + "`cat /run/secrets/pw`", path), false);
    assert.equal(has("TOKEN=" + value, path), true);
  });
for (const path of ["example.md", "example.yaml", ".env.example", "example.toml", "example.txt"])
  test(`B1 non-shell backticks remain literal: ${path}`, () => {
    assert.equal(has("TOKEN=" + "`" + value + "`", path), true);
  });

for (const path of ["auth.go", "auth.ts", "auth.py"]) {
  for (const prefix of ["Default ", "default\t", "  Default   "])
    test(`S2 default credential prose: ${path} ${JSON.stringify(prefix)}`, () => {
      const text = (scalar) => 'print("' + prefix + "Password" + ": " + scalar + '")';
      assert.equal(has(text(value), path), true);
      for (const clean of ["example", "%s", ""]) assert.equal(has(text(clean), path), false);
    });
  for (const newline of ["\\n", "\\r\\n"])
    for (const key of ["api_key", "pwd"])
      test(`S2 escaped configuration line: ${path} ${JSON.stringify(newline)} ${key}`, () => {
        const text = (scalar) => 'print("host: example' + newline + key + ": " + scalar + newline + '")';
        assert.equal(has(text(value), path), true);
        for (const clean of ["example", "%s", ""]) assert.equal(has(text(clean), path), false);
      });
  test(`S2 escaped backslash is not a configuration newline: ${path}`, () => {
    assert.equal(has('print("host: example' + "\\\\n" + "api_key: " + value + '")', path), false);
  });
}
