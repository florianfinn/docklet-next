// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { sha256 } from "../deploy/release/common.mjs";
export function fixture(out) {
  mkdirSync(join(out, "blobs/sha256"), { recursive: true });
  function blob(bytes, mediaType) {
    const data = Buffer.from(typeof bytes === "string" ? bytes : JSON.stringify(bytes));
    const digest = sha256(data);
    writeFileSync(join(out, "blobs/sha256", digest.slice(7)), data);
    return { mediaType, digest, size: data.length };
  }
  const config = blob({ architecture: "amd64", os: "linux" }, "application/vnd.oci.image.config.v1+json");
  const layer = blob("synthetic release layer", "application/vnd.oci.image.layer.v1.tar+gzip");
  const manifest = blob({ schemaVersion: 2, mediaType: "application/vnd.oci.image.manifest.v1+json", config, layers: [layer] }, "application/vnd.oci.image.manifest.v1+json");
  manifest.platform = { architecture: "amd64", os: "linux" };
  writeFileSync(join(out, "index.json"), JSON.stringify({ schemaVersion: 2, mediaType: "application/vnd.oci.image.index.v1+json", manifests: [manifest] }));
  writeFileSync(join(out, "oci-layout"), '{"imageLayoutVersion":"1.0.0"}');
}
