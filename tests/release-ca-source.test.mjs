// Source: docklet-next/issues/446
// Revision: 18969d4007896659a766e4fb409c5cd74d695476
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original CA source asset and rendered pin guard coverage.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { createRequire } from "node:module";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import test from "node:test";
import { caSourceFiles, checkImageDefaults, interpolations, render } from "../deploy/release/render.mjs";
import { sha256 } from "../deploy/release/common.mjs";
const { parse } = createRequire(new URL("../web/package.json", import.meta.url))("yaml");

const assessed = JSON.parse(readFileSync("deploy/release/ca-bundle.json", "utf8"));
const v = "1.0.0-rc.1", repo = "registry.example.com/project/docklet";
const pin = (role) => `${repo}-${role}:${v}@sha256:${"b".repeat(64)}`;
const manifest = { version: v, releaseRepository: repo, images: Object.fromEntries(["hub", "agent"].map((role) => [role, { reference: pin(role), digest: `sha256:${"b".repeat(64)}` }])) };
const temporary = (fn) => { const p = mkdtempSync(join(tmpdir(), "docklet-release-ca-")); try { return fn(p); } finally { rmSync(p, { recursive: true, force: true }); } };
const sha512 = (bytes) => createHash("sha512").update(bytes).digest("hex");

test("the release package attaches the exact CA source archive with every licence it needs", () => temporary((out) => {
  const files = render(manifest, out);
  const archive = basename(assessed.source.file);
  const expected = [archive, ...[...assessed.notices, ...assessed.source.licenses].map((path) => basename(path))];
  const sums = readFileSync(join(out, "SHA256SUMS"), "utf8");
  for (const name of expected) {
    assert.ok(files[name], `missing package file ${name}`);
    assert.deepEqual(readFileSync(join(out, name)), name === archive ? readFileSync(assessed.source.file) : files[name]);
    assert.ok(sums.includes(`${sha256(files[name]).slice(7)}  ${name}\n`), `SHA256SUMS lacks ${name}`);
  }
  assert.equal(sha512(readFileSync(join(out, archive))), assessed.source.sha512);
}));
test("a missing or changed CA source archive stops the package", () => temporary((dir) => {
  assert.throws(() => caSourceFiles({ ...assessed, source: { ...assessed.source, file: join(dir, "absent.tar.bz2") } }), /CA source archive missing/);
  const changed = Buffer.from(readFileSync(assessed.source.file)); changed[changed.length - 1] ^= 1;
  const copy = join(dir, basename(assessed.source.file)); writeFileSync(copy, changed);
  assert.throws(() => caSourceFiles({ ...assessed, source: { ...assessed.source, file: copy } }), /SHA-512 mismatch/);
}));
test("the CA notice, bundle metadata and vendored archive name one source", () => {
  const notice = readFileSync("LICENSES/NOTICE-Mozilla-CA.txt", "utf8");
  const archive = basename(assessed.source.file);
  assert.equal(sha512(readFileSync(assessed.source.file)), assessed.source.sha512);
  for (const fact of [archive, assessed.source.file, assessed.source.sha512, assessed.source.upstream, assessed.sha256, ...assessed.source.licenses.map((path) => basename(path))])
    assert.ok(notice.includes(fact), `CA notice lacks ${fact}`);
  assert.doesNotMatch(notice, /GPL/);
  assert.equal(basename(assessed.source.upstream), archive);
});
test("a CA bundle version change without its source archive fails", () => {
  const stem = (value) => `ca-certificates-${value.replace(/-r\d+$/, "")}.tar.bz2`;
  assert.equal(basename(assessed.source.file), stem(assessed.version));
  assert.notEqual(basename(assessed.source.file), stem("20270101-r0"));
});

const template = readFileSync("deploy/hub/compose.yaml", "utf8");
const withExtra = (edit) => { const compose = parse(template); edit(compose); return "# Header\n" + JSON.stringify(compose); };
for (const [label, edit] of [
  ["a literal service image", (c) => { c.services.extra = { image: "example/extra:latest" }; }],
  ["an interpolated service image default", (c) => { c.services.extra = { image: "${EXTRA_IMAGE:-example/extra:1.0}" }; }],
  ["a service image without a usable default", (c) => { c.services.extra = { image: "${EXTRA_IMAGE}" }; }],
  ["an environment *_IMAGE fallback", (c) => { c.services.hub.environment.HELPER_IMAGE = "${HELPER_IMAGE:-example/helper:1}"; }],
  ["a fallback of the variable named _IMAGE", (c) => { c.services.hub.environment.EXTRA = "${_IMAGE:-example/helper:latest}"; }],
  ["a dash fallback of the variable named _IMAGE", (c) => { c.services.hub.environment.EXTRA = "${_IMAGE-example/helper:latest}"; }],
  ["an *_IMAGE fallback nested in another default", (c) => { c.services.hub.environment.EXTRA = "${OTHER:-${HELPER_IMAGE:-example/helper:latest}}"; }],
  ["a dash *_IMAGE fallback nested in another default", (c) => { c.services.hub.environment.EXTRA = "${OTHER:-${HELPER_IMAGE-example/helper:latest}}"; }],
  ["a nested fallback of the variable named _IMAGE", (c) => { c.services.hub.environment.EXTRA = "${OTHER-${_IMAGE:-example/helper:latest}}"; }],
  ["an *_IMAGE fallback whose default is another interpolation", (c) => { c.services.hub.environment.EXTRA = "${HELPER_IMAGE:-${OTHER:-x}}"; }],
  ["a service image whose default nests an interpolation", (c) => { c.services.extra = { image: "${EXTRA_IMAGE:-${OTHER:-x}}" }; }],
  ["an unbalanced interpolation", (c) => { c.services.hub.environment.EXTRA = "${HELPER_IMAGE:-x"; }],
  ["a list-form environment *_IMAGE fallback", (c) => { c.services.extra = { image: pin("hub"), environment: ["FOO_IMAGE=${FOO_IMAGE:-example/foo:1}"] }; }],
]) {
  test(`rendering refuses ${label} before writing any file`, () => temporary((dir) => {
    const out = join(dir, "package");
    assert.throws(() => render(manifest, out, { template: withExtra(edit) }), /image default|usable default|interpolation/);
    assert.equal(existsSync(out), false);
  }));
}
test("the pin guard accepts only the manifest's hub and agent pins", () => {
  assert.deepEqual(checkImageDefaults({ services: { hub: { image: `\${HUB_IMAGE:-${pin("hub")}}`, environment: { DOCKLET_AGENT_IMAGE: `\${DOCKLET_AGENT_IMAGE:-${pin("agent")}}` } } } }, manifest).sort(),
    [pin("agent"), pin("hub"), pin("hub")].sort());
  assert.deepEqual(checkImageDefaults({ services: { hub: { image: pin("hub"), environment: { EXTRA: `\${_IMAGE:-${pin("agent")}}`, PLAIN: "${OTHER:-x}" } } } }, manifest), [pin("hub"), pin("agent")]);
  const other = `${repo}-hub:1.0.0@sha256:${"c".repeat(64)}`;
  assert.throws(() => checkImageDefaults({ services: { hub: { image: `\${HUB_IMAGE:-${other}}` } } }, manifest), /pinned package reference/);
});
test("the env example keeps the operator overrides", () => temporary((out) => {
  const env = render(manifest, out)[`docklet-next-${v}.env.example`];
  assert.match(env, new RegExp(`^# HUB_IMAGE=${pin("hub").replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`, "m"));
  assert.match(env, /^DOCKLET_AGENT_IMAGE=$/m);
}));
test("the interpolation scan sees every expression, nested ones included", () => {
  assert.deepEqual(interpolations("${A:-${B_IMAGE-x}} ${C} ${D:?m} ${E+y}").map(({ name, operator, argument }) => [name, operator, argument]),
    [["A", ":-", "${B_IMAGE-x}"], ["B_IMAGE", "-", "x"], ["C", "", ""], ["D", ":?", "m"], ["E", "+", "y"]]);
  assert.equal(interpolations("${A:-x}")[0].whole, true);
  assert.equal(interpolations("a ${A:-x}")[0].whole, false);
  for (const bad of ["${", "${1A}", "${A:-x", "${:-x}", "${A/foo/bar}", "${A:bad}"]) assert.throws(() => interpolations(bad), /interpolation/, bad);
});
test("a dash default gets an env example entry like a colon default", () => temporary((out) => {
  const template = withExtra((c) => { c.services.hub.environment.EXTRA = "${EXTRA_SETTING-on}"; });
  assert.match(render(manifest, out, { template })[`docklet-next-${v}.env.example`], /^EXTRA_SETTING=on$/m);
}));
