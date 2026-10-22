// Source: docklet-next/issues/180
// Revision: d3fefb2c6d4130f1a13a046f6863f9ab1e139ccb
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original fail-closed preflight; no mutation or registry publication is performed here.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { pathToFileURL } from "node:url";

export function assertVersionAvailable(image, version, repository, run = (program, args) => spawnSync(program, args, { encoding: "utf8" })) {
  assert.match(version, /^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/);
  const releases = run("gh", ["api", "--paginate", `repos/${repository}/releases`, "--jq", ".[] | {tag: .tag_name, assets: [.assets[].name]} | @json"]);
  assert.equal(releases.status, 0, "release asset lookup failed; publication refused");
  const asset = new RegExp(`(?:^|[-_])(?:docklet-)?tools(?:-source)?[-_]v?${version.replaceAll(".", "\\.")}(?:[-_.]|$)`);
  for (const line of releases.stdout.trim().split("\n").filter(Boolean)) {
    const release = JSON.parse(line);
    assert.notEqual(release.tag, `tools-v${version}`, "tools release already exists");
    assert.equal(release.assets.some((name) => asset.test(name)), false, "tools version release asset already exists");
  }
  for (const suffix of ["", "-source", "-amd64", "-arm64", "-source-amd64", "-source-arm64"]) {
    const ref = `${image}:${version}${suffix}`;
    const result = run("docker", ["buildx", "imagetools", "inspect", ref]);
    assert.notEqual(result.status, 0, `refusing to overwrite existing registry tag: ${ref}`);
    const error = `${result.stderr ?? ""}\n${result.stdout ?? ""}`;
    assert.match(error, /manifest unknown|not found|404 Not Found/i, `registry absence not established: ${ref}`);
    assert.doesNotMatch(error, /unauthorized|denied|timed? ?out|timeout/i, "registry lookup failed; publication refused");
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  assertVersionAvailable(...process.argv.slice(2));
  console.log("version absent from registry tags and release assets; publication may proceed");
}
