import test from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { resolve } from "node:path";
import { cleanEnvironment, scratch, rawCommit, invoke, write, ok, ROOT } from "./helpers.mjs";

test("child CLI discards foreign runner event and runtime state", (t) => {
  const root = scratch(t);
  rawCommit(root);
  const event = resolve(root, "foreign-event.json");
  write(root, "foreign-event.json", JSON.stringify({ pull_request: { head: { sha: "f".repeat(40) } } }));
  const foreign = { ...process.env, GITHUB_EVENT_NAME: "pull_request", GITHUB_EVENT_PATH: event,
    GITHUB_ACTIONS: "true", CI: "true", CI_EVENT: "foreign", RUNNER_TEMP: "foreign",
    ACTIONS_RUNTIME_TOKEN: "example", NODE_OPTIONS: "--invalid-option", CHECK_BASE_SHA: "foreign",
    npm_config_user_agent: "foreign", PNPM_HOME: "foreign", GIT_DIR: "foreign" };
  const env = cleanEnvironment(foreign);
  for (const key of Object.keys(foreign).filter((key) => /^(?:GIT_|GITHUB_|CI|RUNNER_|ACTIONS_|NODE_|CHECK_BASE|npm_|PNPM_)/.test(key)))
    assert.equal(Object.hasOwn(env, key), false, key);
  ok(invoke(root, process.execPath, ["scripts/check-publication.mjs", "--history"], { env }));
  const child = `
    const { node, ok } = await import(${JSON.stringify(pathToFileURL(resolve(ROOT, "tests/helpers.mjs")).href)});
    ok(node(process.cwd(), "scripts/check-publication.mjs", "--history"));
  `;
  ok(invoke(root, process.execPath, ["--input-type=module", "-e", child], {
    env: { ...foreign, NODE_OPTIONS: "" },
  }));
});
test("scratch helper uses a temporary directory outside the repository", (t) => {
  const before = existsSync(resolve(ROOT, ".scratch"));
  const root = scratch(t);
  assert.equal(root.startsWith(ROOT), false);
  assert.equal(existsSync(resolve(ROOT, ".scratch")), before);
});
