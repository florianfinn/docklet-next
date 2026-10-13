import assert from "node:assert/strict";

const JOBS = { checks: "checks", "pr-text": "pr-text", "review-status": "review" };
const PERMISSIONS = {
  checks: "  contents: read\n",
  "pr-text": "  contents: read\n",
  "review-status": "  contents: read\n  pull-requests: read\n  statuses: write\n",
};
export function assertWorkflow(file, text) {
  const job = JOBS[file];
  assert.match(text, /^    timeout-minutes: 15$/m);
  assert.match(text, new RegExp(`^  ${job}:$`, "m"));
  assert.match(text, new RegExp(`^    name: ${job}$`, "m"));
  const permissions = text.match(/^permissions:\n((?:  [^\n]*\n)+)/m)?.[1];
  assert.equal(permissions, PERMISSIONS[file]);
  assert.equal(/^ +permissions:/m.test(text), false);
  const actions = [...text.matchAll(/^\s+- uses: ([^\s]+)/gm)];
  assert.equal(actions.length, 2);
  for (const match of actions) assert.match(match[1], /@[a-f0-9]{40}$/);
  assert.equal(/actions\/cache\b/.test(text), false);
  assert.equal(text.includes("secrets."), false);
  assert.match(text, /persist-credentials: false/);
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
}
