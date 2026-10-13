import test from "node:test";
import assert from "node:assert/strict";
import { inspectText, inspectPath } from "../scripts/publication-patterns.mjs";
import { FIXTURES } from "../scripts/publication-fixtures.mjs";

const samples = [];
const add = (category, label, value) => samples.push({ category, label, value });
for (const prefix of ["ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "sk-"]) {
  add("credential-pattern", prefix, prefix + "A".repeat(36));
}
for (const kind of ["a", "b", "p", "r", "s"]) {
  add("credential-pattern", `Slack ${kind}`, `xox${kind}-` + "1".repeat(24));
}
for (const kind of ["", "RSA ", "EC ", "OPENSSH "]) {
  add("credential-pattern", `private key ${kind}`, "-----BEGIN " + kind + "PRIVATE KEY-----");
}
add("credential-pattern", "AWS", "AKIA" + "A".repeat(16));
add("credential-pattern", "Bearer", "Bearer " + "A".repeat(24));
add("credential-pattern", "JWT", "eyJ" + "A".repeat(8) + "." + "B".repeat(12) + "." + "C".repeat(12));
for (const key of [
  "password",
  "passwd",
  "secret",
  "token",
  "apikey",
  "api_key",
  "api-key",
  "accesstoken",
  "access_token",
  "access-token",
  "clientsecret",
  "client_secret",
  "client-secret",
  "DB_PASSWORD",
  "DB_PASSWD",
  "SERVICE_API_KEY",
]) {
  for (const [shape, assignment] of [
    ["env", `${key}=abcdefgh12345678`],
    ["single quotes", `${key} = 'abcdefgh12345678'`],
    ["double quotes", `"${key}": "abcdefgh12345678"`],
  ]) {
    add("literal-secret", `${key} ${shape}`, assignment);
  }
}
for (const [category, values] of Object.entries(FIXTURES)) {
  values.forEach((value, index) => add(category, `fixture ${index + 1}`, value));
}
for (const root of [
  "root",
  "u/synthetic",
  ["export", "home", "synthetic"].join("/"),
  ["var", "home", "synthetic"].join("/"),
]) {
  add("workstation-path", root, "/" + root + "/project");
}
for (const home of ["~", "~synthetic"]) add("workstation-path", home, home + "/projects/x");
for (const drive of ["C", "D"]) {
  for (const home of ["Users", "Documents and Settings"]) {
    for (const separator of ["\\", "/"]) {
      add(
        "workstation-path",
        `${drive} ${home} ${separator}`,
        `${drive}:${separator}${home}${separator}synthetic`,
      );
    }
  }
}
for (const [label, value] of [
  ["replacement", "\uFFFD"],
  ["C3", "\u00C3\u00BC"],
  ["C2", "\u00C2\u0080"],
  ["E2", "\u00E2\u20AC\u201D"],
  ["low control", "\x01"],
  ["vertical tab", "\x0B"],
  ["form feed", "\x0C"],
  ["high control", "\x1F"],
  ["delete", "\x7F"],
])
  add("corrupted-text", label, value);
for (const [host, path] of [
  ["claude.ai", "code/session_"],
  ["claude.ai", "code/session/"],
  ["chatgpt.com", "s/"],
  ["chatgpt.com", "share/"],
  ["chatgpt.com", "codex/tasks/"],
  ["chat.openai.com", "share/"],
])
  add("session-link", `${host} ${path}`, `https://${host}/${path}example`);

const contexts = [
  ["end", (value) => value],
  ["period", (value) => `${value}.`],
  ["comma", (value) => `${value},`],
  ["parenthesis", (value) => `(${value})`],
  ["double quote", (value) => `"${value}"`],
  ["single quote", (value) => `'${value}'`],
  ["question", (value) => `${value}?`],
  ["line", (value) => `Read ${value}\nNext line`],
  ["URL", (value) => `https://example.com/?value=${value}&end=1`],
];
for (const { category, label, value } of samples) {
  test(`pattern alternatives: ${category} ${label}`, () => {
    for (const [context, wrap] of contexts) {
      assert.equal(
        inspectText(wrap(value)).some((finding) => finding.category === category),
        true,
        context,
      );
    }
    assert.deepEqual(inspectText("Read the token documentation. Use a password manager at example.com."), []);
  });
}
for (const tool of [
  "claude",
  "copilot",
  "codex",
  "chatgpt",
  "openai",
  "gemini",
  "cursor",
  "devin",
  "opus",
  "sonnet",
  "haiku",
  "fable",
  "grok",
  "llama",
  "deepseek",
]) {
  test(`AI attribution rejects ${tool} with and without email`, () => {
    for (const suffix of ["", " <123+example@users.noreply.github.com>"]) {
      const value = "Co-authored-by: " + tool + " 5.5" + suffix;
      assert.equal(
        inspectText(value).some((finding) => finding.category === "ai-co-author"),
        true,
      );
    }
    assert.deepEqual(inspectText("Assisted-by: " + tool), []);
  });
}
for (const extension of ["pem", "key", "p12", "dump", "sqlite", "sqlite3", "log", "zip", "tgz"]) {
  test(`private artifact alternative ${extension}`, () => {
    assert.equal(
      inspectPath(`fixture.${extension}`).some((finding) => finding.category === "private-artifact"),
      true,
    );
    assert.deepEqual(inspectPath("fixture.txt"), []);
  });
}
test("pattern boundaries preserve public hosts, examples and ordinary prose", () => {
  for (const value of [
    "fixture.lan.example.com",
    "fixture.localhost",
    "fixture.internal-service.example.com",
    "fixture.home.arpa.example.com",
    "secret: example",
    "token: test-only",
    "token = changeme",
    ["password", "="].join("") + '""',
    "/rooted/project",
    "person@example.com.",
    "https://example.com/",
    "192.0.2.1.",
    "[2001:db8::1]",
    "::1.",
  ])
    assert.deepEqual(inspectText(value), [], value);
});
test("long nonmatching candidates finish within a bounded scan", { timeout: 3000 }, () => {
  for (const value of ["a".repeat(100000), "a.".repeat(50000), "a_".repeat(50000)]) {
    assert.deepEqual(inspectText(value), []);
  }
});

for (const host of ["anthropic.com", "openai.com", "cursor.com", "cursor.sh", "devin.ai", "cognition.ai"]) {
  test(`AI attribution rejects provider email ${host}`, () => {
    const value = "Co-authored-by: Example Tool <synthetic@" + host + ">";
    assert.equal(
      inspectText(value).some((finding) => finding.category === "ai-co-author"),
      true,
    );
    assert.deepEqual(
      inspectText("Co-authored-by: Example Contributor <123+human@users.noreply.github.com>"),
      [],
    );
  });
}
for (const qualifier of [
  "ai",
  "agent",
  "code",
  "coding",
  "cli",
  "assist",
  "bot",
  "[bot]",
  "opus",
  "sonnet",
  "haiku",
  "pro",
  "flash",
]) {
  test(`AI attribution rejects tool qualifier ${qualifier}`, () => {
    assert.equal(
      inspectText("Co-authored-by: " + "Claude " + qualifier).some(
        (finding) => finding.category === "ai-co-author",
      ),
      true,
    );
    assert.deepEqual(inspectText("Assisted-by: " + "Claude " + qualifier), []);
  });
}
for (const login of ["codex", "123+codex", "123+codex[bot]", "123+jules[bot]"]) {
  test(`AI attribution rejects tool login ${login}`, () => {
    const value = "Co-authored-by: Example Tool <" + login + "@users.noreply.github.com>";
    assert.equal(
      inspectText(value).some((finding) => finding.category === "ai-co-author"),
      true,
    );
  });
}
for (const domain of [
  "example.com",
  "example.net",
  "example.org",
  "sub.example.com",
  "sub.test",
  "sub.invalid",
]) {
  test(`reserved email domain ${domain} permits only its own suffix`, () => {
    assert.deepEqual(inspectText(`person@${domain}.`), []);
    assert.equal(
      inspectText(`person@${domain}.synthetic-mail.org`).some(
        (finding) => finding.category === "non-example-email",
      ),
      true,
    );
  });
}
for (const path of [
  ".env",
  ".env.local",
  "nested/.env",
  ".private/example.txt",
  "review-reports/example.txt",
]) {
  test(`private path ${path} is rejected`, () => {
    assert.equal(inspectPath(path).length > 0, true);
    assert.deepEqual(inspectPath("docs/example.md"), []);
  });
}
for (const address of ["192.0.2.1", "198.51.100.1", "203.0.113.1", "127.0.0.1", "::1", "2001:db8::1"]) {
  test(`reserved or loopback address ${address} is accepted`, () => {
    for (const [, wrap] of contexts) assert.deepEqual(inspectText(wrap(address)), []);
    assert.equal(
      inspectText(FIXTURES["network-address"][0]).some((finding) => finding.category === "network-address"),
      true,
    );
  });
}

test("long failed secret keys and address candidates remain bounded", { timeout: 3000 }, () => {
  for (const value of [
    "a_".repeat(50000) + "unknown=abcdefgh12345678",
    "a".repeat(100000) + ":" + "b".repeat(100000),
    "a.".repeat(50000) + "@example.com",
  ])
    assert.deepEqual(inspectText(value), []);
});
