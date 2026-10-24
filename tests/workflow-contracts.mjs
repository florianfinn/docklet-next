import assert from "node:assert/strict";

const JOBS = { checks: "checks", "pr-text": "pr-text", "review-status": "review" };
const PERMISSIONS = {
  checks: "  contents: read\n",
  "pr-text": "  contents: read\n",
  "review-status": "  contents: read\n  pull-requests: read\n  statuses: write\n",
};
const jobBlock = (text, name) => {
  const match = text.match(new RegExp(`^  ${name}:\\n([\\s\\S]*?)(?=^  [a-z][a-z-]*:\\n|(?![\\s\\S]))`, "m"));
  assert.ok(match, `missing ${name} job`);
  return match[1];
};

export function assertWorkflow(file, text) {
  const job = JOBS[file];
  if (file === "checks") {
    const scope = jobBlock(text, "scope");
    const verify = jobBlock(text, "verify");
    const full = jobBlock(text, "full-checks");
    const image = jobBlock(text, "package-image");
    const checks = jobBlock(text, "checks");
    assert.match(scope, /outputs:\n      value: \$\{\{ steps\.detect\.outputs\.scope \}\}/);
    assert.match(scope, /node scripts\/change-scope\.mjs --ci/);
    assert.match(verify, /^    name: verify$/m);
    assert.match(verify, /^    needs: \[scope, full-checks\]$/m);
    assert.match(verify, /^    if: always\(\)$/m);
    assert.match(verify, /^    timeout-minutes: 15$/m);
    assert.match(verify, /run: node scripts\/check-publication\.mjs --history/);
    assert.match(verify, /if: needs\.scope\.outputs\.value == 'text'\n        run: node scripts\/change-scope\.mjs --run-text-tests/);
    assert.match(full, /^    name: full-\$\{\{ matrix\.stage \}\}$/m);
    assert.match(full, /^    needs: scope$/m);
    assert.match(full, /^    if: needs\.scope\.outputs\.value == 'full'$/m);
    assert.match(full, /^    timeout-minutes: \$\{\{ matrix\.stage == 'test' && 30 \|\| 25 \}\}$/m);
    assert.match(full, /strategy:\n      fail-fast: false\n      matrix:\n        stage: \[lint, test, build\]/);
    for (const entry of [
      'CGO_ENABLED: "0"',
      "CHECK_BASE_SHA: ${{ github.event.pull_request.base.sha }}",
      "GOFLAGS: -p=2",
      "VITEST_MAX_WORKERS: 2",
      "fetch-depth: 0",
      "persist-credentials: false",
      "node-version: 24",
      "run: node scripts/setup-git-hooks.mjs",
      "corepack prepare pnpm@11.21.0 --activate",
      "run: pnpm --dir web install --frozen-lockfile",
      "go-version-file: backend/go.mod",
      "cache-dependency-path:",
      "sudo systemctl stop docker.service docker.socket containerd.service",
      "sudo rm -f /var/run/docker.sock /run/docker.sock",
      "unset DOCKER_HOST",
      "test ! -S /var/run/docker.sock",
      "test ! -S /run/docker.sock",
      "run: pnpm run ${{ matrix.stage }}",
    ]) assert.ok(full.includes(entry), `full checks missing ${entry}`);
    assert.match(image, /^    timeout-minutes: 30$/m);
    assert.match(checks, /^    needs: \[scope, verify, full-checks, package-image\]$/m);
    assert.match(checks, /^    if: always\(\)$/m);
    for (const gate of [verify, checks]) {
      assert.match(gate, /SCOPE: \$\{\{ needs\.scope\.outputs\.value \}\}/);
      assert.match(gate, /SCOPE_RESULT: \$\{\{ needs\.scope\.result \}\}/);
      assert.match(gate, /FULL_RESULT: \$\{\{ needs\.full-checks\.result \}\}/);
      assert.match(gate, /test "\$SCOPE_RESULT" = success/);
      assert.match(gate, /text\) test "\$FULL_RESULT" = skipped/);
      assert.match(gate, /full\) test "\$FULL_RESULT" = success/);
      assert.match(gate, /\*\) exit 1/);
    }
    assert.match(checks, /IMAGE_RESULT: \$\{\{ needs\.package-image\.result \}\}/);
    assert.match(checks, /test "\$VERIFY_RESULT" = success/);
    assert.match(checks, /text\) test "\$FULL_RESULT" = skipped && test "\$IMAGE_RESULT" = skipped/);
    assert.match(checks, /full\) test "\$FULL_RESULT" = success && test "\$IMAGE_RESULT" = success/);
    assert.equal(text.includes("continue-on-error"), false);
  }
  assert.match(text, /^    timeout-minutes: 15$/m);
  assert.match(text, new RegExp(`^  ${job}:$`, "m"));
  assert.match(text, new RegExp(`^    name: ${job}$`, "m"));
  const permissions = text.match(/^permissions:\n((?:  [^\n]*\n)+)/m)?.[1];
  assert.equal(permissions, PERMISSIONS[file]);
  assert.equal(/^ +permissions:/m.test(text), false);
  const actions = [...text.matchAll(/^\s+- uses: ([^\s]+)/gm)];
  assert.equal(actions.length, file === "checks" ? 9 : 2);
  for (const match of actions) assert.match(match[1], /@[a-f0-9]{40}$/);
  assert.equal(/actions\/cache\b/.test(text), false);
  assert.equal(text.includes("secrets."), false);
  assert.equal((text.match(/persist-credentials: false/g) ?? []).length, file === "checks" ? 4 : 1);
  assert.match(text, /branches: \[main\]/);
  // Expressions in run blocks must not interpolate attacker-controlled event text.
  const runBlocks = [...text.matchAll(/^\s*(?:- )?run:([^\n]*(?:\n {10,}[^\n]*)*)/gm)];
  for (const block of runBlocks) {
    assert.equal(
      /\$\{\{[^}]*github\.event\.(?:comment|pull_request\.(?:title|body|head\.ref))/.test(block[1]),
      false,
    );
  }
  if (job === "review") {
    assert.match(text, /pull_request_target:/);
    assert.match(text, /ref: \$\{\{ github.event.repository.default_branch \}\}/);
    assert.equal(/ref:.*github\.event\.pull_request/.test(text), false);
    assert.equal(text.includes("github.event.pull_request.head"), false);
    assert.match(text, /github.event.comment.author_association == 'OWNER'/);
    assert.match(text, /run: node scripts\/review-status.mjs/);
  } else {
    assert.match(text, /\n  pull_request:/);
    assert.equal(text.includes("pull_request_target:"), false);
    assert.equal(text.includes("write"), false);
  }
  if (file === "checks") {
    assert.match(text, /^  package-image:$/m);
    assert.match(text, /scripts\/smoke-hub-image.mjs/);
    assert.equal(text.includes("postgres-integration"), false);
  }
}
