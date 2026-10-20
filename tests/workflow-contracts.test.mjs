import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { assertWorkflow } from "./workflow-contracts.mjs";

for (const file of ["checks", "pr-text", "review-status"]) {
  const original = readFileSync(new URL(`../.github/workflows/${file}.yml`, import.meta.url), "utf8");
  test(`workflow contract ${file} accepts the tracked workflow`, () => assertWorkflow(file, original));
  const mutations = [
    ["timeout removed", (text) => text.replace("    timeout-minutes: 15\n", "")],
    ["timeout raised", (text) => text.replace("timeout-minutes: 15", "timeout-minutes: 150")],
    ["job key", (text) => text.replace(/^  (?:checks|pr-text|review):$/m, "  renamed:")],
    ["job name", (text) => text.replace(/^    name: .+$/m, "    name: renamed")],
    ["contents write", (text) => text.replace("contents: read", "contents: write")],
    ["extra permissions", (text) => text.replace("contents: read", "contents: read\n  actions: write")],
    ["job permissions", (text) => text.replace("    steps:", "    permissions: write-all\n    steps:")],
    ["floating action", (text) => text.replace(/checkout@[a-f0-9]{40}/, "checkout@v4")],
    ["missing action", (text) => text.replace(/^      - uses: actions\/setup-node.*\n/m, "")],
    ["cache", (text) => text.replace("actions/checkout@", "actions/cache@")],
    ["secret", (text) => text + "      - run: echo ${{ secrets.EXAMPLE }}\n"],
    [
      "checkout credentials",
      (text) => text.replace("persist-credentials: false", "persist-credentials: true"),
    ],
    ["main trigger", (text) => text.replace("branches: [main]", "branches: [other]")],
  ];
  for (const property of [
    "comment.body",
    "pull_request.title",
    "pull_request.body",
    "pull_request.head.ref",
  ]) {
    for (const multiline of [false, true]) {
      mutations.push([
        `shell interpolation ${property} ${multiline}`,
        (text) =>
          text +
          (multiline ? "      - run: |\n          echo " : "      - run: echo ") +
          "${{ github.event." +
          property +
          " }}\n",
      ]);
    }
  }
  if (file === "review-status") {
    mutations.push(
      ["review trigger", (text) => text.replace("pull_request_target:", "pull_request:")],
      [
        "checkout PR base",
        (text) =>
          text.replace("github.event.repository.default_branch", "github.event.pull_request.base.ref"),
      ],
      [
        "checkout PR head",
        (text) =>
          text.replace("github.event.repository.default_branch", "github.event.pull_request.head.ref"),
      ],
      [
        "owner gate",
        (text) => text.replace("author_association == 'OWNER'", "author_association != 'OWNER'"),
      ],
      ["review execution", (text) => text.replace("run: node scripts/review-status.mjs", "run: true")],
      ["PR permission", (text) => text.replace("pull-requests: read", "pull-requests: write")],
      ["status permission", (text) => text.replace("statuses: write", "statuses: read")],
    );
  } else {
    mutations.push(["PR trigger", (text) => text.replace("pull_request:", "pull_request_target:")]);
  }
  if (file === "checks") {
    mutations.push(
      ["missing lint stage", (text) => text.replace("[lint, test, build]", "[test, build]")],
      ["missing test stage", (text) => text.replace("[lint, test, build]", "[lint, build]")],
      ["missing build stage", (text) => text.replace("[lint, test, build]", "[lint, test]")],
      ["full stage timeout removed", (text) => text.replace(/(  full-checks:[\s\S]*?)    timeout-minutes: 15/, "$1")],
      ["full stage timeout raised", (text) => text.replace(/(  full-checks:[\s\S]*?)    timeout-minutes: 15/, "$1    timeout-minutes: 30")],
      ["matrix stage command omitted", (text) => text.replace("run: pnpm run ${{ matrix.stage }}", "run: pnpm run lint")],
      ["matrix skipped for full scope", (text) => text.replace("if: needs.scope.outputs.value == 'full'", "if: needs.scope.outputs.value == 'text'")],
      ["verify skipped after matrix failure", (text) => text.replace("    if: always()", "    if: success()")],
      ["checks skipped after verify failure", (text) => text.replace(/(  checks:[\s\S]*?)    if: always\(\)/, "$1    if: success()")],
      ["verify accepts failed scope", (text) => text.replace('test "$SCOPE_RESULT" = success', 'test "$SCOPE_RESULT" != cancelled')],
      ["verify accepts skipped full stage", (text) => text.replace('full) test "$FULL_RESULT" = success', 'full) test "$FULL_RESULT" != cancelled')],
      ["verify accepts running full stage in text scope", (text) => text.replace('text) test "$FULL_RESULT" = skipped', 'text) true')],
      ["checks accepts failed full stage", (text) => text.replace('full) test "$FULL_RESULT" = success &&', 'full) test "$FULL_RESULT" != cancelled &&')],
      ["checks accepts failed image stage", (text) => text.replace('test "$IMAGE_RESULT" = success', 'test "$IMAGE_RESULT" != cancelled')],
      ["checks accepts skipped verify", (text) => text.replace('test "$VERIFY_RESULT" = success', 'test "$VERIFY_RESULT" != cancelled')],
      ["checks accepts image on text scope", (text) => text.replace('test "$IMAGE_RESULT" = skipped', 'test "$IMAGE_RESULT" != cancelled')],
      ["full stage omitted from verify needs", (text) => text.replace("needs: [scope, full-checks]", "needs: [scope]")],
      ["full stage omitted from checks needs", (text) => text.replace("needs: [scope, verify, full-checks, package-image]", "needs: [scope, verify, package-image]")],
      ["Docker isolation removed", (text) => text.replace("sudo systemctl stop docker.service docker.socket containerd.service", "true")],
      ["Docker socket check removed", (text) => text.replace("test ! -S /run/docker.sock", "true")],
      ["frozen install removed", (text) => text.replace("pnpm --dir web install --frozen-lockfile", "pnpm --dir web install")],
      ["base SHA removed", (text) => text.replace("CHECK_BASE_SHA: ${{ github.event.pull_request.base.sha }}", "CHECK_BASE_SHA: ''")],
      ["continue on error", (text) => text.replace("    strategy:", "    continue-on-error: true\n    strategy:")],
      ["publication history removed", (text) => text.replace("node scripts/check-publication.mjs --history", "true")],
      ["reduced text checks removed", (text) => text.replace("node scripts/change-scope.mjs --run-text-tests", "true")],
    );
  }
  for (const [label, mutate] of mutations) {
    test(`workflow contract ${file} rejects ${label}`, () => {
      const changed = mutate(original);
      assert.notEqual(changed, original);
      assert.throws(() => assertWorkflow(file, changed));
    });
  }
}
