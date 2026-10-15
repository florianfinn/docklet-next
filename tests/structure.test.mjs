import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { provenanceProblems, sha256 } from "../scripts/check-structure.mjs";
import { scratch, stage, write, git, node, ok, fails, readJSON, writeJSON, rawCommit } from "./helpers.mjs";

const header =
  "// Source: example/core/file.go\n// Revision: " +
  "a".repeat(40) +
  "\n// Retrieved: 2026-10-13\n// License: BSD-3-Clause\n// Deviations: None\n";
const check = (root, name, ...args) => node(root, "scripts/check-structure.mjs", "--only", name, ...args);
test("own file size boundary is 1000 lines", (t) => {
  const root = scratch(t);
  stage(root, "sample.txt", "line\n".repeat(1001));
  fails(check(root, "size"), "file-size");
  stage(root, "sample.txt", "line\n".repeat(1000));
  ok(check(root, "size"));
});
test("adopted file may shrink but cannot grow past its ceiling", (t) => {
  const root = scratch(t);
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/example.go": 1002 }, generated: [] });
  stage(root, "backend/example.go", header + "line\n".repeat(997));
  ok(check(root, "size"));
  git(root, "add", ".");
  rawCommit(root);
  stage(root, "backend/example.go", header + "line\n".repeat(998));
  fails(check(root, "size"), "file-size");
  stage(root, "backend/example.go", header + "line\n".repeat(996));
  ok(check(root, "size"));
});
test("raising a recorded ceiling fails against the target base", (t) => {
  const root = scratch(t);
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/example.go": 1002 }, generated: [] });
  stage(root, "backend/example.go", header + "line\n".repeat(997));
  git(root, "add", ".");
  const base = rawCommit(root);
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/example.go": 1003 }, generated: [] });
  fails(check(root, "size", "--base", base), "size-baseline-growth");
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/example.go": 1002 }, generated: [] });
  ok(check(root, "size", "--base", base));
});
test("an adopted file cannot regrow beneath an old ceiling", (t) => {
  const root = scratch(t);
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/example.go": 1010 }, generated: [] });
  stage(root, "backend/example.go", header + "line\n".repeat(997));
  git(root, "add", ".");
  const base = rawCommit(root);
  stage(root, "backend/example.go", header + "line\n".repeat(998));
  fails(check(root, "size", "--base", base), "file-size");
  stage(root, "backend/example.go", header + "line\n".repeat(996));
  ok(check(root, "size", "--base", base));
});
test("generated exemptions are exact paths", (t) => {
  const root = scratch(t);
  writeJSON(root, "scripts/size-baseline.json", { adopted: {}, generated: ["api/generated/example.ts"] });
  stage(root, "api/generated/example.ts", "// Generated from the API schema\n" + "line\n".repeat(1002));
  ok(check(root, "size"));
  stage(root, "other.txt", "line\n".repeat(1002));
  fails(check(root, "size"), "file-size");
});
test("every adopted path needs a complete provenance header", (t) => {
  const root = scratch(t);
  stage(root, "backend/example.go", "package example\n");
  fails(check(root, "provenance"), "provenance-header");
  stage(root, "backend/example.go", header + "package example\n");
  ok(check(root, "provenance"));
  stage(root, "backend/example.go", header.replace("a".repeat(40), "abc") + "package example\n");
  fails(check(root, "provenance"), "provenance-header");
});
test("JSON provenance sidecars bind exact file bytes", (t) => {
  const root = scratch(t);
  const content = readFileSync(`${root}/.claude/settings.json`);
  write(root, ".claude/settings.json", Buffer.concat([content, Buffer.from("\n")]));
  fails(check(root, "provenance"), "provenance-hash");
  write(root, ".claude/settings.json", content);
  ok(check(root, "provenance"));
});
test("migration headers agree with sidecars for every authoritative field", () => {
  const path = "backend/resources/migrations/postgres/008_example.sql";
  const record = {
    source: "example/migration.sql", revision: "a".repeat(40), retrieved: "2026-10-15",
    license: "Apache-2.0", deviations: "Synthetic forward migration.",
  };
  const content = () => Buffer.from(Object.entries(record)
    .map(([key, value]) => `-- ${key[0].toUpperCase()}${key.slice(1)}: ${value}\n`).join("") + "-- +goose Up\n");
  const policy = (bytes, sidecar = record) => ({
    prefixes: [], files: [], sidecars: { [path]: { ...sidecar, sha256: sha256(bytes) } },
    migrationHeaderOverrides: {},
  });
  const check = (bytes, sidecar = record) => provenanceProblems([{ path, content: bytes }], policy(bytes, sidecar));
  assert.deepEqual(check(content()), []);
  assert.ok(provenanceProblems([{ path, content: content() }],
    { prefixes: [], files: [], sidecars: {}, migrationHeaderOverrides: {} })
    .some((finding) => finding.category === "migration-provenance-sidecar"));
  for (const key of Object.keys(record)) {
    const changed = { ...record, [key]: key === "revision" ? "b".repeat(40) : `${record[key]} changed` };
    const header = content().toString().replace(record[key], changed[key]);
    assert.ok(check(Buffer.from(header)).some((finding) => finding.category === "migration-provenance-mismatch"), key);
    assert.ok(check(content(), changed).some((finding) => finding.category === "migration-provenance-mismatch"), key);
  }
});
test("unlisted binary and changed asset hash fail", (t) => {
  const root = scratch(t);
  const data = Buffer.from([0, 1, 2, 3]);
  stage(root, "assets/example.bin", data);
  fails(check(root, "assets"), "asset-allowlist");
  writeJSON(root, "scripts/assets.json", [
    {
      path: "assets/example.bin",
      sha256: sha256(data),
      source: "synthetic",
      license: "Apache-2.0",
      purpose: "Synthetic guard fixture",
    },
  ]);
  ok(check(root, "assets"));
  stage(root, "assets/example.bin", Buffer.from([0, 1, 2, 4]));
  fails(check(root, "assets"), "asset-allowlist");
});
test("text media is allowlisted and excluded audio stays excluded even if listed", (t) => {
  const root = scratch(t);
  const data = Buffer.from("<svg />\n");
  stage(root, "example.svg", data);
  fails(check(root, "assets"), "asset-allowlist");
  writeJSON(root, "scripts/assets.json", [
    {
      path: "example.svg",
      sha256: sha256(data),
      source: "synthetic",
      license: "Apache-2.0",
      purpose: "Synthetic fixture",
    },
  ]);
  ok(check(root, "assets"));
  stage(root, "example.mp3", data);
  fails(check(root, "assets"), "excluded-media");
});
test("BSD licence must exist and match its immutable hash", (t) => {
  const root = scratch(t);
  const path = "LICENSES/BSD-3-Clause-Arcane.txt";
  const text = readFileSync(`${root}/${path}`);
  stage(root, path, "changed\n");
  fails(check(root, "license"), "licence-integrity");
  stage(root, path, text);
  ok(check(root, "license"));
  git(root, "rm", "-f", path);
  fails(check(root, "license"), "licence-integrity");
});
test("configured build artefact copies are mandatory and byte identical", (t) => {
  const root = scratch(t);
  const policy = readJSON(root, "scripts/license-policy.json");
  policy.artifactCopies = ["dist/licenses/arcane.txt"];
  writeJSON(root, "scripts/license-policy.json", policy);
  fails(check(root, "license"), "licence-artifact-copy");
  write(root, policy.artifactCopies[0], readFileSync(`${root}/${policy.source}`));
  ok(check(root, "license"));
  write(root, policy.artifactCopies[0], "altered\n");
  fails(check(root, "license"), "licence-artifact-copy");
});
test("malformed guard configuration fails closed", (t) => {
  const root = scratch(t);
  write(root, "scripts/assets.json", "not-json");
  fails(check(root, "assets"), "check-error");
  writeJSON(root, "scripts/assets.json", []);
  ok(check(root, "assets"));
});

test("a same-change oversized file and baseline fail even with valid provenance", (t) => {
  const root = scratch(t);
  const base = rawCommit(root);
  stage(root, "backend/new.go", header + "line\n".repeat(4995));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/new.go": 6000 }, generated: [] });
  fails(check(root, "size", "--base", base), "size-baseline-new-entry");
  writeJSON(root, "scripts/size-baseline.json", { adopted: {}, generated: [] });
  stage(root, "backend/new.go", header + "line\n");
  ok(check(root, "size", "--base", base));
});
test("an existing own file cannot gain an adoption ceiling", (t) => {
  const root = scratch(t);
  stage(root, "backend/existing.go", header + "line\n".repeat(995));
  const base = rawCommit(root);
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/existing.go": 1001 }, generated: [] });
  fails(check(root, "size", "--base", base), "size-baseline-new-entry");
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/existing.go": 1000 }, generated: [] });
  fails(check(root, "size", "--base", base), "size-baseline-new-entry");
});
test("existing oversized files cannot acquire a new exemption", (t) => {
  const root = scratch(t);
  stage(root, "backend/existing.go", header + "line\n".repeat(1195));
  const base = rawCommit(root);
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/existing.go": 1200 }, generated: [] });
  fails(check(root, "size", "--base", base), "size-baseline-new-entry");
  writeJSON(root, "scripts/size-baseline.json", { adopted: { "backend/existing.go": 1201 }, generated: [] });
  fails(check(root, "size", "--base", base), "size-baseline-new-entry");
});
test("baseline exemptions require configured adoption and a valid header", (t) => {
  const root = scratch(t);
  const path = "sample.txt";
  stage(root, path, header + "line\n".repeat(1195));
  writeJSON(root, "scripts/size-baseline.json", { adopted: { [path]: 1200 }, generated: [] });
  fails(check(root, "size"), "size-baseline-provenance");
  const policy = readJSON(root, "scripts/provenance.json");
  policy.files.push(path);
  writeJSON(root, "scripts/provenance.json", policy);
  fails(check(root, "size"), "size-baseline-provenance");
  stage(root, path, "line\n".repeat(1200));
  fails(check(root, "size"), "size-baseline-provenance");
});
for (const prefix of ["backend", "types", "third_party/arcane", "web", "cli", "deploy", "api"]) {
  test(`adopted prefix ${prefix} requires provenance`, (t) => {
    const root = scratch(t);
    const path = `${prefix}/example.txt`;
    stage(root, path, "synthetic\n");
    fails(check(root, "provenance"), "provenance-header");
    stage(root, path, header + "synthetic\n");
    ok(check(root, "provenance"));
  });
}
for (const extension of [
  "mp3",
  "mp4",
  "wav",
  "ogg",
  "webm",
  "mov",
  "avi",
  "flac",
  "m4a",
  "mkv",
  "m4v",
  "flv",
  "aac",
  "opus",
  "wma",
  "aiff",
]) {
  test(`excluded media ${extension} fails even when hash allowlisted`, (t) => {
    const root = scratch(t);
    const path = `clip.${extension}`;
    const data = Buffer.from("synthetic\n");
    stage(root, path, data);
    const asset = {
      path,
      sha256: sha256(data),
      source: "synthetic",
      license: "Apache-2.0",
      purpose: "Fixture",
    };
    writeJSON(root, "scripts/assets.json", [asset]);
    fails(check(root, "assets"), "excluded-media");
    git(root, "rm", "-f", path);
    writeJSON(root, "scripts/assets.json", []);
    ok(check(root, "assets"));
  });
}
