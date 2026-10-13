import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { assertWorkflow } from "./workflow-contracts.mjs";

for (const file of ["checks", "pr-text", "review-status"]) {
  const original = readFileSync(new URL(`../.github/workflows/${file}.yml`, import.meta.url), "utf8");
  test(`workflow contract ${file} accepts the tracked workflow`, () => assertWorkflow(file, original));
  const mutations = [
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
  for (const [label, mutate] of mutations) {
    test(`workflow contract ${file} rejects ${label}`, () => {
      const changed = mutate(original);
      assert.notEqual(changed, original);
      assert.throws(() => assertWorkflow(file, changed));
    });
  }
}
