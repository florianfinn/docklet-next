// Source: docklet-next/issues/180
// Revision: d3fefb2c6d4130f1a13a046f6863f9ab1e139ccb
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original offline assembly of exact notices and source pointers from hash-bound release evidence.
import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { dependencies, checkBuildEvidence, digest } from "./license-contract.mjs";

const [out, arch, version, revision, image] = process.argv.slice(2);
assert.match(revision, /^[a-f0-9]{40}(?:-dirty)?$/);
const data = dependencies();
checkBuildEvidence(data, (name) => readFileSync(`/evidence/${name}`, "utf8"), arch);
assert.equal(digest(readFileSync("/trivy")), data.release.platforms[arch].binarySha256, "exact release Trivy executable");
for (const file of data.files) {
  assert.match(file.imagePath, /^(?:trivy|busybox)\/[a-z]+\/[a-f0-9]{64}\.txt$/);
  const path = join(out, file.imagePath);
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, data.texts.get(file.sha256));
}
writeFileSync(join(out, "trivy/inventory.json"), JSON.stringify(data.trivy, null, 2) + "\n");
writeFileSync(join(out, "busybox/inventory.json"), JSON.stringify(data.embedded.busybox, null, 2) + "\n");
writeFileSync(join(out, "SOURCE.txt"), `Docklet tools ${version}\nSource repository: https://github.com/florianfinn/docklet-next\nSource revision: ${revision}\nBusyBox complete corresponding source: ${image}:${version}-source-${arch}\nMulti-platform source manifest: ${image}:${version}-source\nExtract without starting: docker create ${image}:${version}-source-${arch} /unused; docker cp CONTAINER:/. ./tools-source; docker rm CONTAINER\nTrivy dependencies, including unmodified MPL-covered files: exact module/source revisions and licence notices are in trivy/inventory.json.\nLocal development tags are unpublished; official version tags are immutable. Mirror corresponding source together with binaries.\n`);
console.log(`assembled ${data.trivy.packages.length} Trivy dependency records and ${data.texts.size} exact legal texts`);
