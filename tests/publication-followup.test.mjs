import test from "node:test";
import assert from "node:assert/strict";
import { inspectText, inspectPath, aiIdentity, publicIdentity } from "../scripts/publication-patterns.mjs";
import { messageProblems } from "../scripts/commit-subject.mjs";

const secret = (text, path) => inspectText(text, path).some((f) => f.category === "literal-secret");
const assign = (key, separator, value) => key + separator + value;
const clean = [
  ["Go header", "auth.go", assign("token", " := ", 'r.Header.Get("X")')],
  ["Go error", "auth.go", assign("var ErrInvalidToken", " = ", 'errors.New("invalid")')],
  ["Go comparison", "auth.go", assign("if apiKey", " == ", '"" { return }')],
  ["Go struct tag", "auth.go", 'Password string `json:"password"`'],
  ["Go identifier", "auth.go", assign("password", " = ", "request.Password")],
  ["TS annotation", "auth.ts", assign("token", ": ", "string;")],
  ["TS schema", "auth.ts", assign('"password"', ": ", "z.string().min(8)")],
  ["TS form", "auth.ts", assign("const password", " = ", 'form.get("password")')],
  ["TS call", "auth.ts", assign("const token", " = ", "getToken();")],
  ["TS member", "auth.ts", assign("password", " = ", 'request.form["password"]')],
  ["TS identifier", "auth.ts", assign("password", " = ", "input")],
  ["TS null", "auth.ts", assign("refreshToken", ": ", "null,")],
  ["TS destructuring", "auth.ts", 'const { password } = form; checkToken(token);'],
  ["TS comparison", "auth.ts", assign("if (token", " === ", '"expired") {}')],
  ["env empty default", ".env.example", assign("token", "=", "${TOKEN:-}")],
  ["shell default", "example.sh", assign("token", "=", '"${TOKEN:-example}"')],
  ["YAML quoted placeholder", "example.yml", assign("password", ":\n", '  "example"')],
  ["YAML empty placeholder", "example.yml", assign("password", ":\n", '  ""')],
  ["YAML placeholder", "example.yml", assign("password", ":\n", "  <your password>")],
];
for (const [label, path, text] of clean) {
  test(`source and placeholder accepted: ${label}`, () => assert.equal(secret(text, path), false));
}
for (const key of ["password", "passphrase", "access_key", "accessKey", "pwd", "credentials"]) {
  for (const [label, path, separator, value] of [
    ["Go literal", "auth.go", " := ", '"synthetic-value"'],
    ["TS literal", "auth.ts", " = ", '"synthetic-value"'],
    ["TS template", "auth.ts", " = ", "`synthetic-value`"],
    ["YAML scalar", "example.yml", ": ", "synthetic-value"],
    ["YAML quoted continuation", "example.yml", ":\n", '  "synthetic-value"'],
    ["YAML continuation", "example.yml", ":\n", "  synthetic-value"],
    ["YAML block", "example.yml", ": |-\n", "  synthetic-value"],
    ["env", ".env.example", "=", "synthetic-value"],
    ["shell", "example.sh", "=", "synthetic-value"],
    ["flag", "example.sh", " ", "synthetic-value"],
  ]) {
    test(`literal rejected: ${key} ${label}`, () => {
      const name = label === "flag" ? "--" + key : key;
      assert.equal(secret(assign(name, separator, value), path), true);
      assert.equal(secret(assign(name, separator, value.replace("synthetic-value", "example")), path), false);
    });
  }
}
for (const value of ["${TOKEN:-}suffix", "${TOKEN:-example}suffix", "${TOKEN}${OTHER}"])
  test(`shell substitution suffix rejected: ${value}`, () => {
    assert.equal(secret(assign("token", "=", value), ".env.example"), true);
  });
for (const code of [0x00ad, 0x034f, 0x061c, 0x115f, 0x1160, 0x180e, 0x2028, 0x2029, 0x3164, 0xffa0]) {
  test(`additional invisible U+${code.toString(16)}`, () => {
    const char = String.fromCodePoint(code);
    for (const findings of [inspectText("x" + char), inspectPath("x" + char), messageProblems("fix: x" + char)])
      assert.equal(findings.some((f) => f.category === "corrupted-text"), true);
  });
}
for (const name of ["Windsurf", "Aider", "Amazon Q Developer", "Junie", "claude-code[bot]"]) {
  test(`additional AI identity ${name}`, () => {
    assert.equal(aiIdentity(name, "123+example@users.noreply.github.com"), true);
    assert.equal(publicIdentity(name, "123+example@users.noreply.github.com"), false);
    assert.equal(inspectText("Co-authored-by: " + name).some((f) => f.category === "ai-co-author"), true);
    assert.equal(aiIdentity("Claude Martin"), false);
  });
}
for (const host of ["codeium.com", "windsurf.com", "aider.chat"]) {
  test(`additional AI domain ${host}`, () => {
    assert.equal(aiIdentity("Example", "synthetic@" + host), true);
    assert.equal(aiIdentity("Example", "synthetic@sub." + host), true);
    assert.equal(aiIdentity("Example", "synthetic@" + host + ".example.com"), false);
  });
}
