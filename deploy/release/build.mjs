// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { resolve, join } from "node:path";
import { buildStamp, isMain, layout, options, releaseRepository, repository, run, version } from "./common.mjs";

export function build({ version: v, out, releaseRepository: repo = repository }, execute = run) {
  version(v); releaseRepository(repo);
  if (!out) throw new Error("--out is required");
  const revision = execute("git", ["rev-parse", "HEAD"]);
  // sourceSHA must describe the built bytes: tracked changes refuse the build.
  if (String(execute("git", ["status", "--porcelain", "--untracked-files=no"]) ?? "").trim()) throw new Error("source tree has uncommitted tracked changes; build from a clean checkout");
  const epoch = process.env.SOURCE_DATE_EPOCH ?? execute("git", ["show", "-s", "--format=%ct", "HEAD"]);
  buildStamp({ version: v, revision, epoch, releaseRepository: repo });
  const destination = resolve(out);
  mkdirSync(destination, { recursive: true });
  const images = {};
  // Agent precedes hub so the hub's compiled default has the exact exported agent index digest.
  for (const role of ["agent", "hub"]) {
    const directory = join(destination, role);
    if (existsSync(directory)) throw new Error("OCI destination already exists; use a fresh --out directory");
    const stamp = { version: v, revision, epoch, agentImage: images.agent?.reference ?? "", releaseRepository: repo };
    execute("docker", ["buildx", "build", "--platform", "linux/amd64", "--provenance=false", "--sbom=false",
      "--file", "deploy/hub/Dockerfile", "--target", role, "--tag", `${repo}-${role}:${v}`,
      "--build-arg", `DOCKLET_VERSION=${v}`, "--build-arg", `SOURCE_DATE_EPOCH=${epoch}`,
      "--build-arg", `DOCKLET_REVISION=${revision}`, "--build-arg", `DOCKLET_LDFLAGS=${buildStamp(stamp)}`,
      "--output", `type=oci,dest=${directory},tar=false,oci-mediatypes=true,rewrite-timestamp=true`, "."], { stdio: "inherit" });
    const exported = layout(directory);
    images[role] = { digest: exported.digest, reference: `${repo}-${role}:${v}@${exported.digest}`,
      manifests: exported.manifests.filter(m => !m.mediaType.includes("index") && !m.mediaType.includes("list")).map(m => m.digest) };
  }
  const manifest = { schemaVersion: 1, version: v, sourceSHA: revision, sourceDateEpoch: Number(epoch), releaseRepository: repo, platform: "linux/amd64", images };
  writeFileSync(join(destination, "release-manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
  return manifest;
}
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--version", "--out", "--release-repository"]);
  build({ version: args["--version"], out: args["--out"], releaseRepository: args["--release-repository"] });
}
