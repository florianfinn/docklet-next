import test, { after } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawn } from "node:child_process";

// V8 precise coverage counts executed scanner blocks, including inner loop iterations.
// Full/eighth counts keep the original growth limit without a timing floor or retries.
const GROWTH_LIMIT = 32;
const FLOOR_MS = 250;
const HANG_TIMEOUT_MS = 30000;
const ATTEMPTS = 3;
// Regex backtracking is native work absent from coverage. A 512x size ratio gives
// linear work 64x noise headroom at the same growth exponent (8 -> 32, 512 -> 32768).
const REGEX_SIZE_RATIO = 512;
const REGEX_GROWTH_LIMIT = GROWTH_LIMIT ** 3;
const withinGrowth = (timing) => timing.full < GROWTH_LIMIT * timing.part;
const withinRegexGrowth = (timing) => timing.full < FLOOR_MS || timing.full < REGEX_GROWTH_LIMIT * timing.part;
const describe = (samples) => samples.map((sample) => `${sample.full} blocks full, ${sample.part} blocks eighth`).join("; ");
test("scan guard keeps its growth limit, floor and hang timeout", () => {
  assert.deepEqual([GROWTH_LIMIT, FLOOR_MS, HANG_TIMEOUT_MS], [32, 250, 30 * 1000]);
  assert.deepEqual([REGEX_SIZE_RATIO, REGEX_GROWTH_LIMIT, ATTEMPTS], [512, 32768, 3]);
  const source = readFileSync(new URL(import.meta.url), "utf8");
  assert.deepEqual(source.split("\n").filter((line) => /^\s*assert[.]equal\(result[.]ok, true/.test(line)).map((line) => line.trim()),
    ["assert.equal(result.ok, true, `${label}: ${describe(result.samples)}`);"]);
  assert.match(source, /setTimeout\(.*HANG_TIMEOUT_MS\);/);
});
test("growth rule separates linear from quadratic scans", () => {
  assert.equal(withinGrowth({ full: 800, part: 100 }), true);
  assert.equal(withinGrowth({ full: 6400, part: 100 }), false);
  assert.equal(withinGrowth({ full: 120, part: 1 }), false);
  assert.equal(withinRegexGrowth({ full: 512, part: 1 }), true);
  assert.equal(withinRegexGrowth({ full: 262144, part: 1 }), false);
});
async function growthAttempts(measure) {
  const samples = [];
  for (let attempt = 0; attempt < ATTEMPTS; attempt++) samples.push(await measure());
  return { ok: samples.every(withinGrowth), samples };
}
test("every count sample must satisfy the growth rule", async () => {
  const slow = { full: 6400, part: 100 }, fast = { full: 800, part: 100 };
  for (const values of [[slow, fast, fast], [fast, slow, fast], [fast, fast, slow]]) {
    const queue = [...values];
    assert.deepEqual(await growthAttempts(() => queue.shift()), { ok: false, samples: values });
  }
});
// One isolated process counts all cases; a per-request deadline also stops native hangs.
// Start coverage before loading scanners, so V8 instruments every executed loop.
const workerSource = `
  import { Session } from "node:inspector";
  import { createInterface } from "node:readline";
  const session = new Session();
  session.connect();
  const post = (method, args = {}) => new Promise((resolve, reject) =>
    session.post(method, args, (error, result) => error ? reject(error) : resolve(result)));
  await post("Profiler.enable");
  await post("Profiler.startPreciseCoverage", { callCount: true, detailed: true });
  const { inspectText, inspectPath } = await import("./scripts/publication-patterns.mjs");
  for await (const request of createInterface({ input: process.stdin })) {
    const { expression, call, setup } = JSON.parse(request);
    const input = eval(expression);
    const scan = eval("(value) => { " + setup + "; " + call + "; }");
    const count = async (value) => {
      await post("Profiler.takePreciseCoverage");
      const start = performance.now();
      scan(value);
      const elapsed = performance.now() - start;
      const { result } = await post("Profiler.takePreciseCoverage");
      const blocks = result.filter((script) =>
        (script.url.includes("/scripts/publication-") && script.url.endsWith(".mjs")) || script.url === "")
        .flatMap((script) => script.functions).flatMap((fn) => fn.ranges).reduce((sum, range) => sum + range.count, 0);
      return { blocks, elapsed };
    };
    const tiny = input.slice(0, Math.floor(input.length / ${REGEX_SIZE_RATIO}));
    const time = (value) => { const start = performance.now(); scan(value); return performance.now() - start; };
    const reference = Math.min(time(tiny), time(tiny), time(tiny));
    const part = await count(input.slice(0, Math.floor(input.length / 8)));
    const full = await count(input);
    console.log(JSON.stringify({ bytes: Buffer.byteLength(input), full: full.blocks, part: part.blocks,
      regex: { full: full.elapsed, part: reference } }));
  }
  session.disconnect();
`;
let worker;
let pending;
let stderr = "";
after(() => worker?.kill());
async function sample(setup, expression, call) {
  if (!worker) {
    worker = spawn(process.execPath, ["--input-type=module", "-e", workerSource], {
      cwd: new URL("../", import.meta.url), stdio: ["pipe", "pipe", "pipe"],
    });
    worker.stderr.on("data", (data) => { stderr += data; });
    let output = "";
    worker.stdout.on("data", (data) => {
      output += data;
      const newline = output.indexOf("\n");
      if (newline < 0) return;
      const line = output.slice(0, newline);
      output = output.slice(newline + 1);
      pending?.resolve(JSON.parse(line));
    });
    worker.on("error", (error) => pending?.reject(error));
    worker.on("exit", (code, signal) => pending?.reject(new Error(`scanner worker exited: ${code ?? signal}: ${stderr}`)));
  }
  const measured = await new Promise((resolve, reject) => {
    const deadline = setTimeout(() => { worker.kill(); reject(new Error("scanner hang timeout")); }, HANG_TIMEOUT_MS);
    pending = {
      resolve: (value) => { clearTimeout(deadline); pending = undefined; resolve(value); },
      reject: (error) => { clearTimeout(deadline); pending = undefined; reject(error); },
    };
    worker.stdin.write(JSON.stringify({ setup, expression, call }) + "\n");
  });
  assert.equal(measured.part > 0, true, "scanner coverage must be collected");
  return measured;
}
test("a deliberately quadratic scanner fails every attempt", async (t) => {
  const quadratic = "const quadratic = (value) => { let n = 0; for (let i = 0; i < value.length; i++) for (let j = 0; j < i; j++) n += value.charCodeAt(j) & 1; return n; };";
  const result = await growthAttempts(() => sample(quadratic, '"ab".repeat(100)', "globalThis.sink = quadratic(value)"));
  assert.equal(result.ok, false, `quadratic scanner passed: ${describe(result.samples)}`);
  assert.equal(result.samples.every((entry) => !withinGrowth(entry)), true);
  t.diagnostic(describe(result.samples));
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
// Case variants can produce identical input; retain all labels and count each exact input once, without retries.
const caseMeasurements = new Map();
for (const [label, expression, scanner = "inspectText", path = "synthetic", minimumBytes = 1000000] of cases) {
  test(`adversarial scan: ${label}`, async (t) => {
    const call = scanner === "inspectPath" ? "inspectPath(value)" : `inspectText(value, ${JSON.stringify(path)}, ["absent-term"], { detailed: ${path !== "synthetic"} })`;
    const key = JSON.stringify([expression, call]);
    if (!caseMeasurements.has(key)) caseMeasurements.set(key, (async () => {
      const measured = await sample("", expression, call);
      assert.equal(measured.bytes >= minimumBytes, true);
      assert.equal(withinRegexGrowth(measured.regex), true,
        `${label}: native regex growth: ${JSON.stringify(measured.regex)}`);
      return measured;
    })());
    const measured = await caseMeasurements.get(key);
    const result = { ok: withinGrowth(measured), samples: [measured] };
    assert.equal(result.ok, true, `${label}: ${describe(result.samples)}`);
    t.diagnostic(`${label}: ${result.samples[0].bytes} bytes, ${describe(result.samples)}`);
  });
}
