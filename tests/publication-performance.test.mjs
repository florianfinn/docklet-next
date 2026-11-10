import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

// Load-independent guard: each case scans the full input and its first eighth in one process.
// A linear scanner takes about 8x as long for the full input (measured up to 13x), a quadratic one about 64x.
// Load slows both measurements alike; cheap scans below FLOOR_MS never fail.
const GROWTH_LIMIT = 32;
const FLOOR_MS = 250;
const HANG_TIMEOUT_MS = 30000;
test("scan guard keeps its growth limit, floor and hang timeout", () => {
  assert.deepEqual([GROWTH_LIMIT, FLOOR_MS, HANG_TIMEOUT_MS], [32, 250, 30 * 1000]);
  const source = readFileSync(new URL(import.meta.url), "utf8");
  assert.deepEqual(source.split("\n").filter((line) => /^\s*assert[.]equal\(result[.]ok, true/.test(line)).map((line) => line.trim()),
    ["assert.equal(result.ok, true, `${label}: ${describe(result.timings)}`);"]);
  assert.equal(ATTEMPTS, 3);
  assert.deepEqual(source.match(/^\s*timeout: \S+,$/gm)?.map((line) => line.trim()), ["timeout: HANG_TIMEOUT_MS,"]);
});
const withinGrowth = (timing) => timing.full < FLOOR_MS || timing.full < GROWTH_LIMIT * timing.part;
// Shared-load spikes slow one sample, not every one: a failing case is measured again, at most ATTEMPTS times in total.
// A quadratic scan exceeds the limit on every attempt.
const ATTEMPTS = 3;
function growthAttempts(measure) {
  const timings = [];
  for (let attempt = 0; attempt < ATTEMPTS; attempt++) {
    const timing = measure();
    timings.push(timing);
    if (withinGrowth(timing)) return { ok: true, timings };
  }
  return { ok: false, timings };
}
const describe = (timings) => timings.map((timing) => `${timing.full.toFixed(1)} ms full, ${timing.part.toFixed(1)} ms eighth`).join("; ");
test("growth rule separates linear from quadratic scans", () => {
  assert.equal(withinGrowth({ full: 800, part: 100 }), true);
  assert.equal(withinGrowth({ full: 6400, part: 100 }), false);
  assert.equal(withinGrowth({ full: 120, part: 1 }), true);
});
test("a failing growth sample is measured again at most twice", () => {
  const sequence = (values) => { const queue = [...values]; return () => queue.shift(); };
  const slow = { full: 6400, part: 100 }, fast = { full: 800, part: 100 };
  assert.deepEqual(growthAttempts(sequence([slow, fast, slow])), { ok: true, timings: [slow, fast] });
  assert.deepEqual(growthAttempts(sequence([fast, slow])), { ok: true, timings: [fast] });
  const failing = growthAttempts(sequence([slow, slow, slow, fast]));
  assert.deepEqual(failing, { ok: false, timings: [slow, slow, slow] });
});
// One subprocess per sample; a subprocess timeout also stops a synchronous scanner that never returns.
function sample(setup, expression, call) {
  const source = `
    ${setup}
    const input = (${expression});
    const time = (value) => { const start = performance.now(); ${call}; return performance.now() - start; };
    const part = input.slice(0, Math.floor(input.length / 8));
    // The eighth runs first (it also warms the JIT); its fastest of three is the reference.
    const reference = Math.min(time(part), time(part), time(part));
    const full = time(input);
    console.log(JSON.stringify({ bytes: Buffer.byteLength(input), full, part: reference }));
  `;
  const result = spawnSync(process.execPath, ["--input-type=module", "-e", source], {
    cwd: new URL("../", import.meta.url),
    encoding: "utf8",
    timeout: HANG_TIMEOUT_MS,
  });
  assert.equal(result.status, 0, result.error?.code ?? result.stderr);
  return JSON.parse(result.stdout);
}
test("a deliberately quadratic scanner fails every attempt", () => {
  const quadratic = "const scan = (value) => { let n = 0; for (let i = 0; i < value.length; i++) for (let j = 0; j < i; j++) n += value.charCodeAt(j) & 1; return n; };";
  const result = growthAttempts(() => sample(quadratic, '"ab".repeat(9000)', "globalThis.sink = scan(value)"));
  assert.equal(result.ok, false, `quadratic scanner passed: ${describe(result.timings)}`);
  assert.equal(result.timings.length, ATTEMPTS);
});
const cases = [
  ["78 KB adjacent name/value spaces", '\'{ name: "x"\' + " ".repeat(80000)', "inspectText", "example.ts", 80000],
  ["78 KB adjacent name/value newlines", '\'{ name: "x"\' + "\\n".repeat(80000)', "inspectText", "example.ts", 80000],
  ["4 MB nested block scalars", 'Array.from({length: 2890}, (_, i) => " ".repeat(i) + ["password", ": |\\n"].join("")).join("")', "inspectText", "example.yml"],
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
// Exercise the source lexer and detailed line hashes as well as plain text.
cases.push(
  ["source lexer strings", '("const config = `" + ["password", "=example"].join("") + "`;\\n").repeat(40000)', "inspectText", "example.ts"],
  ["source typed candidates", '["token", ": string | "].join("").repeat(70000)', "inspectText", "example.ts"],
  ["source raw lines", '["const token", " = `\\n"].join("") + "example\\n".repeat(125000) + "`"', "inspectText", "example.go"],
);
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
for (const [label, expression, scanner = "inspectText", path = "synthetic", minimumBytes = 1000000] of cases) {
  test(`adversarial scan: ${label}`, (t) => {
    const call = scanner === "inspectPath" ? "inspectPath(value)" : `inspectText(value, ${JSON.stringify(path)}, ["absent-term"], { detailed: ${path !== "synthetic"} })`;
    const setup = 'import { inspectText, inspectPath } from "./scripts/publication-patterns.mjs";';
    const result = growthAttempts(() => {
      const timing = sample(setup, expression, call);
      assert.equal(timing.bytes >= minimumBytes, true);
      return timing;
    });
    assert.equal(result.ok, true, `${label}: ${describe(result.timings)}`);
    const timing = result.timings.at(-1);
    t.diagnostic(`${label}: ${timing.bytes} bytes, ${describe(result.timings)}`);
  });
}
