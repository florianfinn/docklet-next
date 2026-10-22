// Source: docklet-next/issues/180
// Revision: 50517cb1043719584cdd3ce940f0c8abb1497ab8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original source-image extraction and exact-build correspondence check; never starts the source container.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { inputs } from "./build.mjs";

const [runtime, source] = process.argv.slice(2);
assert.ok(runtime && source, "usage: node deploy/tools/source-contract.mjs RUNTIME_IMAGE SOURCE_IMAGE");
const pins = inputs();
function command(program, args, input) {
  const result = spawnSync(program, args, { input, maxBuffer: 16 * 1024 * 1024 });
  assert.equal(result.status, 0, `${program} ${args[0]}: ${result.stderr}`);
  return result.stdout;
}
const sourceID = command("docker", ["create", "--network", "none", "--label", "t3.project=docklet-next", source, "/unused"]).toString().trim();
try {
  function extract(path) {
    const archive = command("docker", ["cp", `${sourceID}:/${path}`, "-"]);
    return command("tar", ["-xOf", "-", path], archive);
  }
  const architecture = (image) => JSON.parse(command("docker", ["image", "inspect", image]))[0].Architecture;
  assert.equal(architecture(source), architecture(runtime), "source platform matches runtime");
  const tarball = extract(`busybox-${pins.BUSYBOX_VERSION}.tar.bz2`);
  assert.equal(createHash("sha256").update(tarball).digest("hex"), pins.BUSYBOX_SHA256, "complete corresponding source archive");
  const generated = extract(".config");
  assert.match(generated.toString(), /^CONFIG_STATIC=y$/m);
  const runtimeConfig = command("docker", ["run", "--rm", "--network", "none", "--label", "t3.project=docklet-next",
    runtime, "head", "-c", "1000000", "/usr/share/licenses/docklet-tools/busybox.config"]);
  assert.equal(generated.equals(runtimeConfig), true, "source configuration is the binary's build configuration");
  for (const file of ["Dockerfile", "build.mjs", "inputs.env", "busybox.config", "applets.txt"])
    assert.equal(extract(file).equals(readFileSync(`deploy/tools/${file}`)), true, `source recipe ${file}`);
  assert.equal(extract("Arcane-Tools-BSD-3-Clause.txt").equals(readFileSync("third_party/arcane/kit/LICENSE")), true, "tools recipe source copyright");
  assert.equal(extract("Apache-2.0.txt").equals(readFileSync("LICENSE")), true, "own build scripts source licence");
  assert.match(extract("toolchain-packages.txt").toString(), /^musl-/m);
  console.log(`tools source contract passed: exact tarball, generated static config, applets, recipe and toolchain manifest (${architecture(runtime)})`);
} finally {
  command("docker", ["rm", sourceID]);
}
