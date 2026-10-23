import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { inputs } from "../deploy/tools/build.mjs";
import { dependencies, digest, requiredNotices } from "../deploy/tools/license-contract.mjs";
import { assertVersionAvailable } from "../deploy/tools/publish-check.mjs";

const pins = inputs();
const dockerfile = readFileSync("deploy/tools/Dockerfile", "utf8");
// Ignore Docker/shell comments outside quotes before checking executable instructions.
function executableDocker(text) {
  const lines = text.split("\n").filter((line) => !line.trimStart().startsWith("#"));
  return lines.map((line) => {
    let quote = "";
    for (let i = 0; i < line.length; i++) {
      if (line[i] === "\\") { i++; continue; }
      if (line[i] === quote) quote = "";
      else if (!quote && ["'", '"'].includes(line[i])) quote = line[i];
      else if (!quote && line[i] === "#" && (i === 0 || /\s/.test(line[i - 1]))) return line.slice(0, i);
    }
    return line;
  }).join("\n").replace(/\\\n/g, " ");
}
const executable = executableDocker(dockerfile);
test("tools inputs pin two platforms, versions, archives and base images", () => {
  assert.deepEqual(pins.PLATFORMS.split(","), ["linux/amd64", "linux/arm64"]);
  for (const key of ["GO_IMAGE", "BUILD_IMAGE", "NODE_IMAGE"])
    assert.match(pins[key], /:\d+\.\d+\.\d+(?:-[a-z0-9-]+)?@sha256:[a-f0-9]{64}$/);
  for (const key of ["BUSYBOX_VERSION", "TRIVY_VERSION"])
    assert.match(pins[key], /^\d+\.\d+\.\d+$/);
  assert.match(pins.CA_VERSION, /^\d{4}-\d{2}-\d{2}$/);
  for (const key of ["BUSYBOX_SHA256", "TRIVY_AMD64_SHA256", "TRIVY_ARM64_SHA256", "CA_SHA256"])
    assert.match(pins[key], /^[a-f0-9]{64}$/);
  assert.equal(pins.RUNTIME_USER, "0:0", "volume ownership requires an explicit root-run contract");
  const modules = JSON.parse(readFileSync("docs/provenance/core-import.json", "utf8"));
  const acfs = modules.packages.find((entry) => entry.name === "go.getarcane.app/acfs");
  assert.ok(acfs, "acfs source inventory");
  assert.equal(pins.ACFS_VERSION, acfs.upstreamVersion);
  assert.equal(pins.ACFS_REVISION, acfs.version);
  const protocol = readFileSync("third_party/arcane/acfs/types/entry.go", "utf8").match(/const ProtocolVersion = (\d+)/)[1];
  assert.equal(pins.ACFS_PROTOCOL, protocol);
});
test("tools recipe builds local acfs offline, verifies downloads and exports matching BusyBox source", () => {
  assert.match(dockerfile, /COPY third_party\/arcane\/acfs/);
  assert.match(dockerfile, /GOPROXY=off GOSUMDB=off/);
  assert.match(dockerfile, /RUN --network=none/);
  assert.match(dockerfile, /go build -mod=mod/);
  assert.doesNotMatch(dockerfile, /git clone|go install|go mod download/);
  assert.match(executable, /echo "\$BUSYBOX_SHA256  busybox\.tar\.bz2" \| sha256sum -c - &&/);
  assert.match(executable, /echo "\$checksum  trivy\.tar\.gz" \| sha256sum -c - &&/);
  assert.match(executable, /echo "\$CA_SHA256  cacert\.pem" \| sha256sum -c - &&/);
  assert.match(dockerfile, /cp \.config \/source\/\.config/);
  assert.match(dockerfile, /FROM scratch AS corresponding-source/);
  assert.match(dockerfile, /FROM scratch AS runtime/);
  assert.doesNotMatch(executable, /LABEL[^\n]*t3\./);
  assert.match(executable, /node deploy\/tools\/assemble-licenses\.mjs/);
  assert.match(executable, /COPY --from=legal \/out\/ \/usr\/share\/licenses\/docklet-tools\//);
  assert.match(executable, /COPY LICENSES\/NOTICE-Trivy\.txt \/usr\/share\/licenses\/docklet-tools\/Trivy-NOTICE\.txt/);
  assert.match(executable, /sha256sum busybox > \/source\/busybox\.sha256/);
});

test("tools CI verifies both native platforms and gates source-first publication on main SemVer tags", () => {
  const workflow = readFileSync(".github/workflows/tools-image.yml", "utf8");
  assert.equal(workflow.match(/^      IMAGE: (\S+)$/m)?.[1], pins.IMAGE_REPOSITORY, "published image repository must match source metadata repository");
  assert.match(workflow, /^permissions: \{\}$/m);
  assert.match(workflow, /arch: amd64\n            runner: ubuntu-24.04\n/);
  assert.match(workflow, /arch: arm64\n            runner: ubuntu-24.04-arm\n/);
  assert.match(workflow, /node deploy\/tools\/contract.mjs/);
  assert.match(workflow, /node deploy\/tools\/source-contract.mjs/);
  const [verify, publish] = workflow.split("  publish:\n");
  assert.doesNotMatch(verify, /packages: write|docker push|docker login/);
  assert.match(publish, /needs: verify/);
  assert.match(publish, /if: github.event_name == 'push' && startsWith\(github.ref, 'refs\/tags\/tools-v'\)/);
  assert.match(publish, /git merge-base --is-ancestor "\$GITHUB_SHA" origin\/main/);
  assert.match(publish, /packages: write/);
  assert.match(publish, /concurrency:\n      group: tools-image-publish-/);
  assert.match(publish, /node deploy\/tools\/publish-check.mjs/);
  assert.ok(publish.indexOf("node deploy/tools/publish-check.mjs") < publish.indexOf('docker push "$IMAGE:'));
  assert.match(workflow, /upload-artifact@[a-f0-9]{40} # v7\./);
  assert.match(workflow, /download-artifact@[a-f0-9]{40} # v8\./);
  assert.equal((workflow.match(/packages: write/g) ?? []).length, 1);
  assert.match(publish, /docker push "\$IMAGE:\$VERSION-source-\$arch"/);
  assert.match(publish, /docker push "\$IMAGE:\$VERSION-\$arch"/);
  assert.ok(publish.indexOf('docker push "$IMAGE:$VERSION-source-$arch"') < publish.indexOf('docker push "$IMAGE:$VERSION-$arch"'));
  for (const match of workflow.matchAll(/uses: (\S+)/g)) assert.match(match[1], /@[a-f0-9]{40}$/);
  assert.doesNotMatch(workflow, /pull_request_target|workflow_run|secrets\.|contents: write|:latest/);
});

test("tools legal inventory matches the committed notices and pinned components", async () => {
  const { createHash } = await import("node:crypto");
  const { inventory } = dependencies();
  assert.deepEqual(inventory.platforms, pins.PLATFORMS.split(","));
  for (const [name, pin] of [["acfs", "ACFS_VERSION"], ["BusyBox", "BUSYBOX_VERSION"], ["Trivy", "TRIVY_VERSION"], ["Mozilla CA bundle", "CA_VERSION"]])
    assert.equal(inventory.components.find((entry) => entry.name === name).version, pins[pin]);
  for (const file of inventory.components.flatMap((entry) => entry.legalFiles)) {
    assert.match(file.sha256, /^[a-f0-9]{64}$/);
    if (file.path) assert.equal(createHash("sha256").update(readFileSync(file.path)).digest("hex"), file.sha256, file.path);
  }
});


test("tools Trivy dependency licences cover exact release and embedded dependencies", () => {
  const data = dependencies();
  assert.equal(data.trivy.packages.length, 462, "460 SBOM modules plus two linked versions absent from upstream SBOM");
  for (const [path, hash] of Object.entries(data.embedded.acfs.sourceFiles))
    assert.equal(digest(readFileSync(path)), hash, `acfs source/import change requires dependency refresh: ${path}`);
  for (const file of requiredNotices) assert.ok(data.inventory.components.some((row) => row.legalFiles.some((entry) => entry.imagePath.endsWith(`/${file}`))));
});

test("tools publication refuses existing tags, release assets and uncertain registry responses", () => {
  const absent = (program) => program === "gh" ? { status: 0, stdout: "" } : { status: 1, stderr: "manifest unknown" };
  assertVersionAvailable("ghcr.io/example/tools", "1.2.3", "example/tools", absent);
  for (const suffix of ["", "-source", "-amd64", "-arm64", "-source-amd64", "-source-arm64"])
    assert.throws(() => assertVersionAvailable("ghcr.io/example/tools", "1.2.3", "example/tools", (program, args) =>
      program === "docker" && args.at(-1).endsWith(`:1.2.3${suffix}`) ? { status: 0, stdout: "existing" } : absent(program)), /overwrite existing registry tag/);
  for (const release of [{ tag: "tools-v1.2.3", assets: [] }, { tag: "archive", assets: ["docklet-tools-1.2.3-source.tar.gz"] }])
    assert.throws(() => assertVersionAvailable("ghcr.io/example/tools", "1.2.3", "example/tools", (program) =>
      program === "gh" ? { status: 0, stdout: JSON.stringify(release) } : absent(program)), /already exists/);
  for (const stderr of ["unauthorized", "connection timed out", "internal server error", "denied: not found"])
    assert.throws(() => assertVersionAvailable("ghcr.io/example/tools", "1.2.3", "example/tools", (program) =>
      program === "gh" ? absent(program) : { status: 1, stderr }));
  assert.throws(() => assertVersionAvailable("ghcr.io/example/tools", "1.2.3", "example/tools", () => ({ status: 1 })), /release asset lookup failed/);
});
