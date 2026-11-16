import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
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

function busyboxFetchScript(kind, cache) {
  if (kind === "CI") {
    const workflow = readFileSync(".github/workflows/tools-image.yml", "utf8");
    const step = workflow.split("      - name: Fetch and verify BusyBox source\n")[1];
    assert.ok(step, "CI must fetch and verify source before building");
    return step.split("        run: |\n")[1].split(/^      - /m)[0].replace(/^          /gm, "");
  }
  const instruction = executable.split("\n").find((line) => line.startsWith("RUN ") && line.includes('"$BUSYBOX_SHA256  busybox.tar.bz2"'));
  assert.ok(instruction, "Docker build must verify BusyBox source");
  return instruction.replace(/^RUN (?:--mount=\S+ )?/, "").replaceAll("/busybox-cache", cache)
    .split("    mkdir source &&")[0].replace(/&&\s*$/, "");
}

for (const kind of ["CI", "Docker"]) {
  for (const state of ["miss", "hit", "corrupt", "unavailable"]) {
    test(`tools ${kind} BusyBox source ${state} preserves download and verification behaviour`, (t) => {
      const root = mkdtempSync(join(tmpdir(), "tools-source-"));
      t.after(() => rmSync(root, { recursive: true, force: true }));
      const cache = join(root, kind === "CI" ? "docklet-tools-busybox" : "cache");
      const bin = join(root, "bin");
      mkdirSync(cache); mkdirSync(bin);
      const archive = "synthetic pinned source archive";
      if (["hit", "corrupt"].includes(state)) writeFileSync(join(cache, "busybox.tar.bz2"), state === "hit" ? archive : "corrupt");
      // Exercise the recipe shell with a deterministic downloader; checksum verification stays real.
      writeFileSync(join(bin, "curl"), `#!/bin/sh
printf '%s\\n' "$*" >> "$FETCH_LOG"
if [ "$FETCH_STATE" = unavailable ]; then exit 28; fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; printf '%s' "$FETCH_ARCHIVE" > "$1"; exit 0; fi
  shift
done
exit 1
`, { mode: 0o755 });
      const script = busyboxFetchScript(kind, cache);
      const result = spawnSync("sh", ["-eu", "-c", script], { cwd: root, encoding: "utf8", env: {
        ...process.env, PATH: `${bin}:${process.env.PATH}`, RUNNER_TEMP: root,
        BUSYBOX_VERSION: pins.BUSYBOX_VERSION, BUSYBOX_SHA256: createHash("sha256").update(archive).digest("hex"),
        FETCH_LOG: join(root, "fetch.log"), FETCH_ARCHIVE: archive, FETCH_STATE: state,
      } });
      assert.equal(result.status, state === "unavailable" ? 28 : state === "corrupt" ? 1 : 0,
        result.stdout + result.stderr);
      if (["hit", "corrupt"].includes(state)) {
        assert.throws(() => readFileSync(join(root, "fetch.log")), { code: "ENOENT" });
      } else {
        const args = readFileSync(join(root, "fetch.log"), "utf8");
        for (const flag of ["--retry 3", "--retry-all-errors", "--retry-max-time 180", "--connect-timeout 15", "--max-time 45"])
          assert.ok(args.includes(flag), flag);
        assert.doesNotMatch(args, /--retry-delay/, "curl default exponential backoff");
        assert.match(args, /https:\/\/busybox[.]net\/downloads\/busybox-/);
      }
      if (["hit", "miss"].includes(state)) assert.match(result.stdout, /busybox[.]tar[.]bz2: OK/);
      if (state === "corrupt") assert.match(result.stdout + result.stderr, /FAILED/);
    });
  }
}

test("tools CI keys verified source by version and hash and supplies both build targets", () => {
  const workflow = readFileSync(".github/workflows/tools-image.yml", "utf8");
  const cache = workflow.split("      - name: Cache verified BusyBox source\n")[1]?.split("      - name:")[0];
  assert.ok(cache);
  assert.match(cache, /uses: actions\/cache@[a-f0-9]{40}/);
  assert.match(cache, /key: busybox-source-\$\{\{ steps[.]busybox[.]outputs[.]version \}\}-\$\{\{ steps[.]busybox[.]outputs[.]sha256 \}\}/);
  assert.match(cache, /path: \$\{\{ runner[.]temp \}\}\/docklet-tools-busybox\/busybox[.]tar[.]bz2/);
  assert.doesNotMatch(cache, /restore-keys|secrets[.]/);
  const builds = workflow.split("\n").filter((line) => line.includes("node deploy/tools/build.mjs --platform"));
  assert.equal(builds.length, 2);
  for (const build of builds) assert.ok(build.includes('--build-context "busybox-cache=$RUNNER_TEMP/docklet-tools-busybox"'));
  assert.match(dockerfile, /FROM scratch AS busybox-cache/);
  assert.match(dockerfile, /RUN --mount=type=bind,from=busybox-cache,target=\/busybox-cache/);
});
