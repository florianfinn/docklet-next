// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { isMain, layout, options, releaseReference, version } from "../deploy/release/common.mjs";
export function checkPackage(directory, expectedVersion, sourceSHA) {
  const manifest = JSON.parse(readFileSync(join(directory, "release-manifest.json")));
  if (manifest.version !== version(expectedVersion) || manifest.sourceSHA !== sourceSHA) throw new Error("package source/version mismatch");
  for (const role of ["agent", "hub"]) {
    const ref = releaseReference(manifest.images[role].reference);
    if (ref.tag !== expectedVersion || ref.digest !== layout(join(directory, role)).digest || ref.digest !== manifest.images[role].digest || `${ref.registry}/${ref.path}` !== `${manifest.releaseRepository}-${role}`) throw new Error("package OCI identity mismatch");
  }
  return manifest;
}
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--dir", "--version", "--source"]);
  checkPackage(args["--dir"], args["--version"], args["--source"]);
}
