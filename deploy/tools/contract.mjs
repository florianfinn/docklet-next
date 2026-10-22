// Source: docklet-next/issues/180
// Revision: 50517cb1043719584cdd3ce940f0c8abb1497ab8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original executable helper-image contract; invoked explicitly, outside default tests.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { dependencies, checkBuildEvidence, digest } from "./license-contract.mjs";
import { inputs } from "./build.mjs";

const pins = inputs();
const image = process.argv[2];
assert.ok(image, "usage: node deploy/tools/contract.mjs IMAGE");
function docker(args) {
  const result = spawnSync("docker", args, { encoding: "utf8" });
  assert.equal(result.status, 0, `${args[0]} failed: ${result.stderr}`);
  return result.stdout.trim();
}
function run(...args) {
  return docker(["run", "--rm", "--network", "none", "--label", "t3.project=docklet-next", image, ...args]);
}
const config = JSON.parse(docker(["image", "inspect", image]))[0];
assert.equal(pins.RUNTIME_USER, "0:0", "helper volume operations require root");
assert.equal(config.Config.User, "0:0", "explicit root-run image contract");
const labels = config.Config.Labels;
assert.equal(Object.keys(labels).some((name) => name.startsWith("t3.")), false, "published images contain no sandbox labels");
assert.equal(labels["org.opencontainers.image.source"], "https://github.com/florianfinn/docklet-next");
assert.match(labels["org.opencontainers.image.revision"], /^[a-f0-9]{40}(?:-dirty)?$/);
assert.equal(config.Os, "linux");
assert.ok(pins.PLATFORMS.split(",").includes(`linux/${config.Architecture}`));
const version = JSON.parse(run("acfs", "version"));
assert.equal(version.version, pins.ACFS_VERSION, "vendored acfs version");
assert.equal(version.revision, pins.ACFS_REVISION, "vendored acfs revision");
assert.equal(version.protocol, Number(pins.ACFS_PROTOCOL), "vendored acfs protocol");
assert.match(run("trivy", "--version"), new RegExp(`^Version: ${pins.TRIVY_VERSION.replaceAll(".", "\\.")}$`, "m"));
const applets = readFileSync(new URL("applets.txt", import.meta.url), "utf8")
  .split("\n").filter((line) => line && !line.startsWith("#"));
for (const applet of applets) run("test", "-x", `/bin/${applet}`);
const [uid, gid] = ["0", "0"];
run("sh", "-ec", `
  while read -r key real effective saved fs; do
    case "$key" in
      Uid:) test "$real" = '${uid}'; test "$effective" = '${uid}'; uid_checked=yes ;;
      Gid:) test "$real" = '${gid}'; test "$effective" = '${gid}'; gid_checked=yes ;;
    esac
  done < /proc/self/status
  test "$uid_checked" = yes; test "$gid_checked" = yes
  work=$(mktemp -d); trap 'rm -r "$work"' EXIT
  mkdir "$work/in" "$work/out"
  printf '%s\\n' 'archive roundtrip' > "$work/in/payload"
  tar -czf "$work/data.tar.gz" -C "$work/in" payload
  test "$(tar -tzf "$work/data.tar.gz")" = payload
  tar -xzf "$work/data.tar.gz" -C "$work/out"
  read -r content < "$work/out/payload"
  test "$content" = 'archive roundtrip'
  test "$(stat -c '%u:%g' "$work/in/payload")" = '0:0'
  mkdir /var/cache/docklet-tools/root-contract
  rmdir /var/cache/docklet-tools/root-contract
`);
const listing = JSON.parse(run("acfs", "list", "--root", "/tmp", "--path", "/"));
assert.equal(listing.version, Number(pins.ACFS_PROTOCOL), "actual list response protocol");
run("test", "-s", "/etc/ssl/certs/ca-certificates.crt");
const data = dependencies();
const directory = mkdtempSync(join(tmpdir(), "docklet-tools-contract-"));
const container = docker(["create", "--network", "none", "--label", "t3.project=docklet-next", image]);
try {
  mkdirSync(join(directory, "licenses"));
  docker(["cp", `${container}:/usr/share/licenses/docklet-tools/.`, join(directory, "licenses")]);
  const evidence = (name) => readFileSync(join(directory, "licenses", name), "utf8");
  for (const file of data.inventory.components.flatMap((component) => component.legalFiles)) {
    const name = file.imagePath.split("/").at(-1);
    assert.equal(digest(readFileSync(join(directory, "licenses", name))), file.sha256, `packaged licence hash: ${file.imagePath}`);
  }
  for (const file of data.files)
    assert.equal(digest(readFileSync(join(directory, "licenses", file.imagePath))), file.sha256, `packaged dependency notice: ${file.sha256}`);
  assert.deepEqual(JSON.parse(evidence("trivy/inventory.json")), data.trivy, "packaged Trivy dependency/source inventory");
  checkBuildEvidence(data, evidence, config.Architecture);
  for (const [path, name, hash] of [
    ["/usr/local/bin/trivy", "trivy", data.release.platforms[config.Architecture].binarySha256],
    ["/etc/ssl/certs/ca-certificates.crt", "ca.pem", pins.CA_SHA256],
    ["/bin/busybox", "busybox", evidence("busybox.sha256").split(/\s+/)[0]],
  ]) {
    docker(["cp", `${container}:${path}`, join(directory, name)]);
    assert.equal(digest(readFileSync(join(directory, name))), hash, `packaged executable/source pin: ${name}`);
  }
  const source = evidence("SOURCE.txt");
  assert.ok(source.includes(`${pins.IMAGE_REPOSITORY}:${labels["org.opencontainers.image.version"]}-source-${config.Architecture}`), "discoverable exact corresponding source tag");
  assert.ok(source.includes(labels["org.opencontainers.image.revision"]), "source pointer matches OCI revision");
} finally {
  docker(["rm", container]);
  rmSync(directory, { recursive: true, force: true });
}
console.log(`tools root-run contract passed: ${config.Architecture}, acfs ${version.version}/protocol ${version.protocol}, ${applets.length} applets, Trivy/dependency notices, tar/gzip roundtrip, UID/GID 0:0`);
