import test from "node:test";
import assert from "node:assert/strict";
import { inspectText } from "../scripts/publication-patterns.mjs";

// These row IDs match the policy decision table. Inputs are assembled synthetic data.
const assignment = (key, separator, value) => key + separator + value;
const literal = '"synthetic-value"';
const rows = [
  ["D1 untyped", "auth.ts", "const token", " = ", literal, "input"],
  ["D2 typed TS", "auth.ts", "export const JWT_SECRET", ": string = ", literal, "process.env.X"],
  ["D2 readonly TS", "auth.ts", "private readonly apiKey", ": string = ", literal, "input"],
  ["D2 typed Python", "auth.py", "secret", ": str = ", literal, "input"],
  ["D3 Go short", "auth.go", "token", " := ", literal, 'r.Header.Get("X")'],
  ["D3 Go var", "auth.go", "var apiKey", " = ", literal, "input"],
  ["D3 Go typed", "auth.go", "var apiKey", " string = ", literal, "input"],
  ["D4 env", ".env.example", "TOKEN", "=", "synthetic-value", "${TOKEN}"],
  ["D4 shell", "example.sh", "export API_KEY", "=", "synthetic-value", "example"],
  ["D4 env keyword", ".env.example", "if token", "=", "synthetic-value", "example"],
  ["D5 flag", "example.sh", "--password", " ", "synthetic-value", "example"],
  ["D6 YAML", "example.yml", "password", ": ", literal, '"example"'],
  ["D6 JSON", "example.json", '"password"', ": ", literal, '"example"'],
  ["D7 upper embedded", "auth.ts", 'env: ["API_KEY', "=", 'synthetic-value"]', 'example"]'],
  ["D7 lower embedded", "auth.go", 'fmt.Sprintf("user=a password', "=", 'synthetic-value")', 'example")'],
  ["D8 URL", "auth.go", 'dsn := "https://example.com/?api_key', "=", 'synthetic-value"', 'example"'],
  ["D9 comment", "auth.ts", "// token", ": ", "synthetic-value", "example"],
  ["D9 Python comment", "auth.py", "# TOKEN", "=", "synthetic-value", "example"],
  ["D10 template", "auth.ts", "const config = `\nAPI_KEY", "=", "synthetic-value\n`", "example\n`"],
  ["D10 Go raw", "auth.go", "config := `\npassword", ": ", "synthetic-value\n`", "example\n`"],
  ["D10 direct multiline", "auth.go", "token", " := ", "`\nsynthetic-value\n`", "`\nexample\n`"],
  ["D11 colon default", ".env.example", "PASSWORD", "=", "${VAR:-synthetic-value}", "${VAR:-example}"],
  ["D11 default", "example.sh", "PASSWORD", "=", "${VAR-synthetic-value}", "${VAR-}"],
  ["D12 YAML scalar", "example.yml", "password", ":\n", "  synthetic-value", "  example"],
  ["D12 YAML block", "example.yml", "password", ": |-\n", "  synthetic-value", "  example"],
  ["D13 mapping", "example.yml", "secrets", ":\n", "  synthetic-value", "  db_password" + ":\n    file: ./db_password.txt"],
  ["D13 list", "example.yml", "tokens", ":\n", "  synthetic-value", "  - name: example"],
  ["D14 mode", "auth.ts", "credentials", ": ", literal, '"include"'],
  ["D15 path", "example.sh", "PWD", "=", "synthetic-value", "/tmp/work"],
  ["D15 credentials path", "example.sh", "GOOGLE_APPLICATION_CREDENTIALS", "=", "synthetic-value", "/run/secrets/g.json"],
  ["D16 identifier", "auth.ts", "token", " = ", literal, "input"],
  ["D16 call", "auth.ts", "const password", " = ", literal, 'form.get("password")'],
  ["D16 member", "auth.go", "password", " = ", literal, "request.Password"],
  ["D17 annotation", "auth.ts", "token", ": ", "string = " + literal, "string;"],
  ["D18 comparison", "auth.go", "if apiKey", " ", "= " + literal, '== "expired" {}'],
];
for (const [id, path, key, separator, bad, good] of rows) {
  test(`decision ${id}: literal rejected and clean twin accepted`, () => {
    const has = (value) => inspectText(assignment(key, separator, value), path).some((f) => f.category === "literal-secret");
    assert.equal(has(bad), true);
    assert.equal(has(good), false);
  });
}
for (const mode of ["include", "same-origin", "omit"])
  test(`fetch credentials mode ${mode}`, () => assert.deepEqual(inspectText(assignment("credentials", ": ", JSON.stringify(mode)), "auth.ts"), []));
for (const value of ["changeme", "example", "test-only", "***", "<value>", ""])
  test(`shell default placeholder ${value}`, () => {
    for (const operator of ["-", ":-", "+", ":+", "?", ":?", "=", ":="])
      assert.deepEqual(inspectText(assignment("PASSWORD", "=", "${VAR" + operator + value + "}"), "example.sh"), []);
  });
test("inline YAML mappings and lists retain nested scalar checks", () => {
  for (const nested of ['{ name: example }', '[example]'])
    assert.deepEqual(inspectText(assignment("secrets", ": ", nested), "example.yml"), []);
  const nested = "{ " + assignment("password", ": ", "synthetic-value") + " }";
  assert.equal(inspectText(assignment("secrets", ": ", nested), "example.yml").some((f) => f.category === "literal-secret"), true);
});
test("structural workflow and Kubernetes keys are not scalar secrets", () => {
  const yaml = ['container:', '  credentials:', '    username: example', assignment('    password', ': ', '${{ secrets.X }}'),
    'secretKeyRef:', '  name: example', '  key: password', 'secrets:', '  database:', '    external: true'].join("\n");
  assert.deepEqual(inspectText(yaml, "example.yml"), []);
});
