// Source: docklet-next/issues/180
// Revision: 50517cb1043719584cdd3ce940f0c8abb1497ab8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original executable helper-image contract; invoked explicitly, outside default tests.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
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
assert.equal(config.Config.User, pins.RUNTIME_USER, "configured runtime user");
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
const [uid, gid] = pins.RUNTIME_USER.split(":");
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
`);
const listing = JSON.parse(run("acfs", "list", "--root", "/tmp", "--path", "/"));
assert.equal(listing.version, Number(pins.ACFS_PROTOCOL), "actual list response protocol");
run("test", "-s", "/etc/ssl/certs/ca-certificates.crt");
console.log(`tools contract passed: ${config.Architecture}, acfs ${version.version}/protocol ${version.protocol}, ${applets.length} applets, Trivy, tar/gzip roundtrip, UID/GID ${pins.RUNTIME_USER}`);
