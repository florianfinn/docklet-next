// Source: docklet-next/issues/180
// Revision: d3fefb2c6d4130f1a13a046f6863f9ab1e139ccb
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original immutable release/dependency/notice coverage validation shared by CI and image assembly.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { inputs } from "./build.mjs";

export const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const json = (path) => JSON.parse(readFileSync(path, "utf8"));
export const requiredNotices = [
  "acfs-BSD-3-Clause.txt", "kit-BSD-3-Clause.txt", "Arcane-Tools-BSD-3-Clause.txt",
  "Arcane-BSD-3-Clause.txt", "Go-BSD-3-Clause.txt", "BusyBox-GPL-2.0.txt",
  "Trivy-Apache-2.0.txt", "Trivy-NOTICE.txt", "CA-MPL-2.0.txt", "CA-NOTICE.txt",
  "musl-COPYRIGHT.txt", "Docklet-Apache-2.0.txt",
];
export function dependencies() {
  const pins = inputs();
  const inventory = json("docs/provenance/tools-image.json");
  const trivy = json("docs/provenance/tools-trivy.json");
  const release = json("docs/provenance/tools-release.json");
  const embedded = json("docs/provenance/tools-embedded.json");
  const component = inventory.components.find((row) => row.name === "Trivy");
  assert.equal(release.version, pins.TRIVY_VERSION, "Trivy version requires refreshed release evidence");
  assert.equal(trivy.version, release.version, "Trivy version requires refreshed dependency inventory");
  assert.equal(digest(readFileSync(component.dependencyInventory.path)), component.dependencyInventory.sha256, "Trivy dependency inventory hash");
  assert.equal(digest(readFileSync(trivy.releaseEvidence.path)), trivy.releaseEvidence.sha256, "Trivy release evidence hash");
  const expected = new Map(release.sbom.modules.map(({ name, version }) => [`${name}@${version}`, { name, version }]));
  for (const platform of Object.values(release.platforms))
    for (const [name, version] of platform.modules) expected.set(`${name}@${version}`, { name, version });
  const identity = (row) => `${row.name}@${row.version}`;
  const byIdentity = (a, b) => identity(a) < identity(b) ? -1 : identity(a) > identity(b) ? 1 : 0;
  assert.deepEqual(trivy.packages.map(({ name, version }) => ({ name, version })).sort(byIdentity), [...expected.values()].sort(byIdentity), "every SBOM and linked release dependency has licence evidence");
  assert.equal(digest(readFileSync(inventory.embeddedInventory.path)), inventory.embeddedInventory.sha256, "embedded dependency inventory hash");
  assert.equal(component.dependencyInventory.moduleCount, trivy.packages.length);
  for (const arch of ["amd64", "arm64"]) {
    assert.equal(release.platforms[arch].archiveSha256, pins[`TRIVY_${arch.toUpperCase()}_SHA256`], "release archive pin");
    for (const [name, version] of release.platforms[arch].modules)
      assert.ok(trivy.packages.some((row) => row.name === name && row.version === version), `linked Trivy dependency: ${name}@${version}`);
  }
  assert.equal(embedded.acfs.version, pins.ACFS_VERSION);
  assert.equal(embedded.busybox.version, pins.BUSYBOX_VERSION);
  assert.equal(embedded.busybox.muslVersion, pins.MUSL_VERSION);
  assert.equal(embedded.busybox.gccVersion, pins.GCC_VERSION);
  assert.equal(embedded.busybox.muslPackage, pins.MUSL_PACKAGE);
  assert.equal(embedded.busybox.gccPackage, pins.GCC_PACKAGE);
  const base = inventory.components.flatMap((row) => row.legalFiles);
  for (const name of requiredNotices)
    assert.ok(base.some((file) => file.imagePath === `/usr/share/licenses/docklet-tools/${name}`), `required packaged notice: ${name}`);
  const texts = new Map();
  for (const file of trivy.payloads) {
    const bytes = readFileSync(file.path);
    assert.equal(digest(bytes), file.sha256, `legal payload: ${file.path}`);
    for (const entry of JSON.parse(bytes).texts) {
      assert.equal(digest(entry.text), entry.sha256, "exact legal text bytes");
      assert.equal(texts.has(entry.sha256), false, "duplicate legal payload");
      texts.set(entry.sha256, entry.text);
    }
  }
  const files = [...trivy.packages.flatMap((row) => row.legalFiles), ...embedded.busybox.gccLegalFiles, ...embedded.busybox.muslLegalFiles];
  for (const row of trivy.packages) {
    assert.ok(row.license && row.legalFiles.length, `dependency grant: ${row.name}`);
    assert.match(row.source, /^https:\/\//);
    if (row.license.includes("MPL-2.0")) assert.equal(row.sourceForm, row.source, `MPL source form: ${row.name}`);
  }
  for (const file of files) assert.ok(texts.has(file.sha256), `missing dependency notice: ${file.sha256}`);
  return { pins, inventory, trivy, release, embedded, files, texts };
}
export function checkBuildEvidence(data, evidence, arch) {
  const lines = evidence("trivy-buildinfo.txt").split("\n");
  const modules = lines.filter((line) => line.trim().startsWith("dep\t")).map((line) => line.trim().split(/\s+/).slice(1, 4));
  modules.sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0);
  assert.deepEqual(modules, data.release.platforms[arch].modules, "exact linked Trivy module graph");
  assert.equal(lines[0].trim().split(/:\s+/).at(-1), data.release.platforms[arch].goVersion, "release Go toolchain");
  assert.match(evidence("trivy-buildinfo.txt"), /CGO_ENABLED=0/);
  const acfsModules = evidence("acfs-buildinfo.txt").split("\n")
    .map((line) => line.trim().split(/\s+/)).filter(([kind]) => ["mod", "dep"].includes(kind)).map(([, name]) => name).sort();
  assert.deepEqual(acfsModules, data.embedded.acfs.modules, "actual linked acfs module graph");
  const acfs = evidence("acfs-packages.txt").trim().split("\n").sort();
  assert.deepEqual(acfs, data.embedded.acfs.packages, "acfs embedded dependency graph");
  const packages = evidence("toolchain-packages.txt");
  for (const [name, version] of [["musl", data.pins.MUSL_VERSION], ["gcc", data.pins.GCC_VERSION]])
    assert.match(packages, new RegExp(`^${name}-${version.replaceAll(".", "\\.")}-r\\d+$`, "m"), `embedded ${name} version`);
  for (const [name, version] of [["musl-dev", data.pins.MUSL_PACKAGE], ["gcc", data.pins.GCC_PACKAGE], ["libgcc", data.pins.GCC_PACKAGE]])
    assert.ok(packages.split("\n").includes(`${name}-${version}`), `exact embedded build package: ${name}`);
  const map = evidence("busybox.map");
  const archives = [...new Set([...map.matchAll(/(?:^|[\s(])([^\s()]+\.a)(?:\(|\s)/gm)].map((match) => match[1]))];
  assert.ok(archives.some((path) => path.endsWith("/libc.a")), "static musl appears in link map");
  for (const path of archives.filter((path) => path.startsWith("/")))
    assert.ok(data.embedded.busybox.externalArchives.includes(path.split("/").at(-1)), `uninventoried BusyBox archive: ${path}`);
}
