import test from "node:test";
import assert from "node:assert/strict";
import { inspectText } from "../scripts/publication-patterns.mjs";
import { reviewedFindings, lineHash, ALLOWLIST_PATH } from "../scripts/publication-allowlist.mjs";
import { scratch, stage, node, fails } from "./helpers.mjs";

const literal = '"synthetic-value"';
const has = (text, path) => inspectText(text, path).some((f) => f.category === "literal-secret");
const pair = (name, path, prefix, good, bad = literal) => test(name, () => {
  assert.equal(has(prefix + good, path), false, "reference or message must pass");
  assert.equal(has(prefix + bad, path), true, "literal must fail");
});
for (const value of ['$DB_PASSWORD', '"$DB_PASSWORD"', '$(cat /run/secrets/pw)', '"$(cat /run/secrets/pw)"', '`cat /run/secrets/pw`'])
  pair(`shell reference ${value}`, "example.sh", "PASSWORD=", value);
for (const value of ['$TOKEN', '"$TOKEN"', '$(read_token)', '`read_token`'])
  pair(`shell flag reference ${value}`, "example.sh", "cmd --token ", value);
pair("compose environment reference", "compose.yml", "environment:\n  - DB_PASSWORD=", "$DB_PASSWORD");
for (const [path, prefix] of [["example.sh", "POSTGRES_PASSWORD_FILE="], ["example.sh", "cmd --password-file "],
  ["compose.yml", "private_key_path: "], ["compose.yml", ["secrets", ":\n  db_password", ":\n    file: "].join("")]])
  pair(`secret file ${prefix}`, path, prefix, "/run/secrets/pw", prefix.endsWith("file: ") ? '"example"\npassword: ' + literal : literal);
for (const path of ["./pw", "../pw", "C:/secrets/pw"])
  pair(`relative secret file ${path}`, "example.sh", "TOKEN_FILE=", path);
pair("workflow permission", "example.yml", "permissions:\n  id-token: ", "write");
pair("workflow inherited secrets", "example.yml", "secrets: ", "inherit");
for (const [path, prefix, suffix] of [["auth.go", 'fmt.Errorf("invalid credentials: ', '", err)'],
  ["auth.go", 'log.Printf("invalid password: ', '")'], ["auth.ts", 'console.error("failed credentials: ', '")']]) {
  for (const message of ["%w", "denied", "", "retry later"])
    pair(`message ${prefix}${message}`, path, prefix, message + suffix, ['password', '=synthetic-value'].join('') + suffix);
}
for (const key of ["credentials", "password"])
  pair(`assignment-like string ${key}`, "auth.go", 'fmt.Println("' + key + ': ', '%w")', 'synthetic-value")');
for (const key of ["OLDPWD", "credentials_file"])
  test(`anchored secret names leave ${key} clean`, () => assert.equal(has(key + "=synthetic-value", "example.sh"), false));
for (const prefix of ["b", "r", "br", "rb", "u", "f"])
  pair(`Python literal prefix ${prefix}`, "example.py", "API_KEY = ", prefix + '"example"', prefix + literal);
for (const type of ["&str", "&'static str", "&[u8]"])
  pair(`Rust reference type ${type}`, "example.rs", "const API_KEY: " + type + " = ", "input", literal);
for (const path of ["auth.ts", "auth.go", "auth.rs", "auth.py"])
  pair(`multiline quoted value ${path}`, path, ["password", ":\n    "].join(""), '"example",', literal + ",");
for (const [call, key] of [["headers.Set", "X-Api-Key"], ["localStorage.setItem", "token"], ["request.Header.Set", "X-Api-Key"], ["storage.setItem", "token"]])
  pair(`literal setter ${call}`, "auth.go", call + '("' + key + '", ', "input)", literal + ")");
for (const prefix of [['Key: ', '"DB_PASSWORD", ', 'Value: '].join(''), ['"name": ', '"API_KEY", ', '"value": '].join(''), ['name: ', '"TOKEN"\n', 'value: '].join('')])
  pair(`paired key/value ${prefix}`, "auth.go", prefix, "input", literal);
test("unrelated records and ordinary calls are not paired", () => {
  for (const text of ['Key: "DB_PASSWORD", Other: "value", Value: ', 'Key: "label", Value: ', 'call("token", '])
    assert.equal(has(text + literal + ")", "auth.go"), false);
});
const entry = { path: "notes.md", category: "literal-secret", lineSha256: lineHash("synthetic"), reason: "Reviewed public synthetic fixture." };
for (const category of ["ai-co-author", "session-link", "corrupted-text", "private-commit-identity", "invalid-utf8",
  "runtime-env-file", "private-artifact", "file-name", "operator-denylist", "unsupported-git-entry", "file-size", "provenance-header"])
  test(`policy category ${category} cannot be exempted`, () => {
    assert.throws(() => reviewedFindings([{ ...entry, category }], [{ ...entry, category, sha256: entry.lineSha256 }]));
  });
for (const category of ["literal-secret", "credential-pattern", "non-example-email", "network-address", "workstation-path", "private-hostname"])
  test(`data category ${category} permits an exact reviewed finding`, () => {
    assert.deepEqual(reviewedFindings([{ ...entry, category }], [{ path: entry.path, category, sha256: entry.lineSha256 }]), []);
    assert.deepEqual(reviewedFindings([], [{ path: entry.path, category, sha256: entry.lineSha256 }]), [{ path: entry.path, category }]);
  });
for (const reason of ["todo", "TODO: fill this later", "TBD: awaiting review", "FIXME: explain later", "reason", "<reviewed public reason>", "abcdefghijk"])
  test(`placeholder reason ${reason} fails`, () => assert.throws(() => reviewedFindings([{ ...entry, reason }], [])));
for (const field of ["count", "reason", "path", "category", "lineSha256"])
  test(`duplicate JSON field ${field} fails the real CLI`, (t) => {
    const root = scratch(t);
    const text = ["const token", " = ", literal].join("");
    const record = { ...entry, path: "notes.ts", lineSha256: lineHash(text), count: 1 };
    stage(root, record.path, text);
    const json = JSON.stringify(record);
    stage(root, ALLOWLIST_PATH, "[" + json.slice(0, -1) + ',' + JSON.stringify(field) + ':' + JSON.stringify(record[field]) + "}]");
    fails(node(root, "scripts/check-publication.mjs", "--staged"), "check-error");
  });

test("reason length uses a trimmed twelve-character boundary", () => {
  const finding = { path: entry.path, category: entry.category, sha256: entry.lineSha256 };
  assert.deepEqual(reviewedFindings([{ ...entry, reason: "  Public label  " }], [finding]), []);
  assert.throws(() => reviewedFindings([{ ...entry, reason: " abcdefghijk " }], [finding]));
});
test("escaped duplicate JSON field names fail the real CLI", (t) => {
  const root = scratch(t);
  const text = ["const token", " = ", literal].join("");
  const record = { ...entry, path: "notes.ts", lineSha256: lineHash(text), count: 1 };
  stage(root, record.path, text);
  stage(root, ALLOWLIST_PATH, "[" + JSON.stringify(record).slice(0, -1) + ',"co' + '\\u0075' + 'nt":1}]');
  fails(node(root, "scripts/check-publication.mjs", "--staged"), "check-error");
});
for (const [category, text] of [["ai-co-author", ["Co-authored-by:", " Codex"].join("")],
  ["session-link", ["https://claude.ai", "/code/session_synthetic"].join("")],
  ["corrupted-text", "ordinary" + String.fromCodePoint(0x200b)]])
  test(`real CLI and proposal keep ${category} non-exemptable`, (t) => {
    const root = scratch(t);
    stage(root, "notes.md", text);
    fails(node(root, "scripts/check-publication.mjs", "--staged"), category);
    const result = node(root, "scripts/publication-allowlist.mjs", "--propose", "notes.md");
    assert.equal(result.status, 0);
    assert.deepEqual(JSON.parse(result.stdout), []);
    stage(root, ALLOWLIST_PATH, JSON.stringify([{ ...entry, category, lineSha256: lineHash(text) }]));
    fails(node(root, "scripts/check-publication.mjs", "--staged"), "check-error");
  });
for (const [path, prefix, suffix] of [["auth.go", 'fmt.Println("  password' + ': ', '")'],
  ["auth.ts", 'const text = `\n    password' + ': ', '\n`']])
  pair(`indented string configuration ${path}`, path, prefix, "example" + suffix, "synthetic-value" + suffix);
test("multiline scalar resumes at the next peer key with exact line hashes", () => {
  const text = ["password" + ": |", "  synthetic-value", "  password" + ": |", "    synthetic-value", "token" + ": synthetic-value"].join("\r\n");
  const found = inspectText(text, "example.yml", [], { detailed: true }).filter((f) => f.category === "literal-secret");
  assert.deepEqual(found.map((f) => f.sha256), text.split("\r\n").slice(1).map(lineHash));
});
