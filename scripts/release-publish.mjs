// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { isMain, options } from "../deploy/release/common.mjs";
import { pushOCI } from "../deploy/release/push-oci.mjs";
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--dir"]);
  const manifest = JSON.parse(readFileSync(join(args["--dir"], "release-manifest.json")));
  for (const role of ["agent", "hub"]) await pushOCI({ directory: join(args["--dir"], role), reference: manifest.images[role].reference });
}
