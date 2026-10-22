import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { inputs } from "../deploy/tools/build.mjs";

const pins = inputs();
const dockerfile = readFileSync("deploy/tools/Dockerfile", "utf8");
test("tools inputs pin two platforms, versions, archives and base images", () => {
  assert.deepEqual(pins.PLATFORMS.split(","), ["linux/amd64", "linux/arm64"]);
  for (const key of ["GO_IMAGE", "BUILD_IMAGE"])
    assert.match(pins[key], /:\d+\.\d+\.\d+(?:-[a-z]+)?@sha256:[a-f0-9]{64}$/);
  for (const key of ["BUSYBOX_VERSION", "TRIVY_VERSION"])
    assert.match(pins[key], /^\d+\.\d+\.\d+$/);
  assert.match(pins.CA_VERSION, /^\d{4}-\d{2}-\d{2}$/);
  for (const key of ["BUSYBOX_SHA256", "TRIVY_AMD64_SHA256", "TRIVY_ARM64_SHA256", "CA_SHA256"])
    assert.match(pins[key], /^[a-f0-9]{64}$/);
  assert.match(pins.RUNTIME_USER, /^\d+:\d+$/);
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
  assert.match(dockerfile, /echo "\$BUSYBOX_SHA256 .*sha256sum -c -/);
  assert.match(dockerfile, /echo "\$checksum .*sha256sum -c -/);
  assert.match(dockerfile, /echo "\$CA_SHA256 .*sha256sum -c -/);
  assert.match(dockerfile, /cp \.config \/source\/\.config/);
  assert.match(dockerfile, /FROM scratch AS corresponding-source/);
  assert.match(dockerfile, /FROM scratch AS runtime/);
});
