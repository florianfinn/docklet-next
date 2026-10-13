import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

const SCAN_BUDGET_MS = 2000;
test("scan budget stays at two seconds", () => {
  assert.equal(SCAN_BUDGET_MS, 2 * 1000);
  const source = readFileSync(new URL(import.meta.url), "utf8");
  assert.equal(source.match(/assert[.]equal\((timing[.]ms < [A-Z_]+)/)?.[1], "timing.ms < SCAN_BUDGET_MS");
});
const cases = [
  ["coauthor spaces", '"Co-authored-by:" + " ".repeat(1000000 - 15)'],
  ["coauthor email domain", '"Co-authored-by: Example <person@" + "a.".repeat(500000) + "invalid>"'],
  ["repeated assignments", '["password", "="].join("").repeat(111112)'],
  ["quoted assignment", '["password", "="].join("") + "\\\"" + "x".repeat(1000000)'],
  ["long failed name", '"a_".repeat(500000) + "unknown=example"'],
  ["hostname labels", '"a.".repeat(500000)'],
  ["email local part", '"a".repeat(1000000) + "@example.com"'],
  ["email domain", '"person@" + "a.".repeat(500000) + "invalid"'],
  ["IPv6 separators", '"a:".repeat(500000)'],
  ["credential prefix", '"eyJ" + "a".repeat(1000000) + "."'],
  ["JWT upper-case overlap", '"eyJ".repeat(333334)'],
  ["JWT prefixed upper-case overlap", '"XeyJ".repeat(250000)'],
  ["YAML continuation", '["password", ":\\n  "].join("") + "x".repeat(1000000)'],
  ["source calls", '["const password", " = form.get(\\\"password\\\");\\n"].join("").repeat(40000)'],
  ["overlapping credential starts", '"eyJ-".repeat(250000)'],
  ["home expansion", '"~" + "a".repeat(1000000)'],
  ["corrupted text", '"\\u200b".repeat(333334)'],
  ["session candidate", '"https://".repeat(125000)'],
  ["runtime env filename", '"/.env.x".repeat(142858) + "\\nunsafe"', "inspectPath"],
  ["operator denylist", '"a".repeat(1000000)'],
];
// Every credential alternative gets mixed case and separator/boundary stress.
const prefixes = ["ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "AKIA", "sk-",
  "xoxa-", "xoxb-", "xoxp-", "xoxr-", "xoxs-", "Bearer ", "eyJ",
  ...["", "RSA ", "EC ", "OPENSSH "].map((kind) => "-----BEGIN " + kind + "PRIVATE KEY-----")];
for (const prefix of prefixes) {
  for (const [variant, token] of [["exact", prefix], ["lower", prefix.toLowerCase()], ["upper", prefix.toUpperCase()]]) {
    for (const separator of ["", "X", "x", "-", "_", ".", " ", "\n"]) {
      const unit = token + separator;
      cases.push([`${prefix} ${variant} separator ${JSON.stringify(separator)}`,
        `${JSON.stringify(unit)}.repeat(${Math.ceil(1000000 / unit.length)})`]);
    }
  }
}
for (const [label, expression, scanner = "inspectText"] of cases) {
  test(`1 MB adversarial scan: ${label}`, (t) => {
    const source = `
      import { inspectText, inspectPath } from "./scripts/publication-patterns.mjs";
      const input = (${expression});
      const start = performance.now();
      ${scanner === "inspectPath" ? "inspectPath(input)" : 'inspectText(input, "synthetic", ["absent-term"])'};
      console.log(JSON.stringify({ bytes: Buffer.byteLength(input), ms: performance.now() - start }));
    `;
    // A subprocess timeout also stops a synchronous scanner that never returns.
    const result = spawnSync(process.execPath, ["--input-type=module", "-e", source], {
      cwd: new URL("../", import.meta.url),
      encoding: "utf8",
      timeout: 5000,
    });
    assert.equal(result.status, 0, result.error?.code ?? result.stderr);
    const timing = JSON.parse(result.stdout);
    assert.equal(timing.bytes >= 1000000, true);
    assert.equal(timing.ms < SCAN_BUDGET_MS, true, `${label}: ${timing.ms.toFixed(1)} ms`);
    t.diagnostic(`${label}: ${timing.bytes} bytes, ${timing.ms.toFixed(1)} ms`);
  });
}
