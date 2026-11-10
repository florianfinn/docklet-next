// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { isMain, layout, options, run } from "../deploy/release/common.mjs";
// Smoke tests import the exact OCI payload into a containerd-backed Docker image store.
// Publication always uses push-oci.mjs and never this import path.
export function load(directory) {
  const source = layout(directory);
  const manifest = source.manifests.find(m => m.mediaType.endsWith("manifest.v1+json") || m.mediaType.endsWith("manifest.v2+json"));
  const image = manifest.digest;
  const temporary = mkdtempSync(join(tmpdir(), "docklet-release-load-"));
  try {
    const archive = join(temporary, "image.tar");
    run("tar", ["-cf", archive, "-C", resolve(directory), "."]);
    run("docker", ["image", "load", "--input", archive], { stdio: ["ignore", "pipe", "inherit"] });
    run("docker", ["image", "inspect", image]);
    return image;
  } finally { rmSync(temporary, { recursive: true, force: true }); }
}
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--dir", "--out"]);
  for (const role of ["agent", "hub"]) console.log(`${role}=${load(join(args["--dir"], role))}`);
}
