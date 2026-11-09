// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
export const labels = process.env.SMOKE_DOCKER_LABEL ? ["--label", process.env.SMOKE_DOCKER_LABEL] : [];
export function docker(args, required = true) {
  const result = spawnSync("docker", args, { encoding: "utf8", timeout: 120000, maxBuffer: 8 * 1024 * 1024 });
  if (required && result.status !== 0) throw new Error(`candidate operation ${args[0]} failed (exit ${result.status}): ${result.stderr}`);
  return result;
}
export function smokeRuntime(image, role, runningID) {
  const temp = mkdtempSync(join(tmpdir(), "docklet-release-smoke-"));
  const containers = [], volumes = [];
  try {
    const assessed = JSON.parse(readFileSync("deploy/release/ca-bundle.json"));
    const id = docker(["create", ...labels, image]).stdout.trim(); containers.push(id);
    assert.equal(docker(["cp", `${id}:${assessed.path}`, join(temp, "shipped-ca.crt")], false).status, 0, "shipped CA bundle must exist");
    assert.equal(createHash("sha256").update(readFileSync(join(temp, "shipped-ca.crt"))).digest("hex"), assessed.sha256, "shipped CA bundle hash");
    for (const name of assessed.notices) {
      const base = name.split("/").at(-1);
      docker(["cp", `${id}:/usr/share/licenses/docklet-next/ca/${base}`, join(temp, base)]);
      assert.deepEqual(readFileSync(join(temp, base)), readFileSync(name), "exact CA licence evidence");
    }
    for (const path of ["/bin/sh", "/usr/bin/wget", "/usr/bin/nvidia-smi", "/usr/bin/intel_gpu_top", ...(role === "agent" ? ["/usr/local/bin/docklet-hub", "/usr/share/licenses/docklet-next/web"] : [])])
      assert.notEqual(docker(["cp", `${id}:${path}`, join(temp, "absent")], false).status, 0, "unexpected runtime payload");
    const compiled = spawnSync("go", ["build", "-trimpath", "-o", join(temp, "probe"), "deploy/release/probe.go"], { encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64", GOWORK: "off" } });
    assert.equal(compiled.status, 0, `compile smoke probe: ${compiled.stderr}`);
    docker(["cp", join(temp, "probe"), `${runningID}:/tmp/release-probe`]);
    docker(["exec", runningID, "/tmp/release-probe", "runtime", role]);
    for (const name of ["server", "wrong"]) {
      const generated = spawnSync("openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1", "-keyout", join(temp, `${name}.key`), "-out", join(temp, `${name}.crt`)], { encoding: "utf8" });
      assert.equal(generated.status, 0, "generate isolated smoke TLS fixture");
    }
    for (const good of [true, false]) {
      const volume = `docklet-roots-${randomBytes(6).toString("hex")}`; volumes.push(volume);
      docker(["volume", "create", ...labels, volume]);
      const client = docker(["create", ...labels, "--network", "none", "-v", `${volume}:/etc/ssl/certs`, "--entrypoint", "/tmp/release-probe", image, "tls"]).stdout.trim(); containers.push(client);
      docker(["cp", join(temp, good ? "server.crt" : "wrong.crt"), `${client}:/etc/ssl/certs/ca-certificates.crt`]);
      for (const [source, destination] of [["probe", "release-probe"], ["server.crt", "server.crt"], ["server.key", "server.key"]])
        docker(["cp", join(temp, source), `${client}:/tmp/${destination}`]);
      const result = docker(["start", "--attach", client], false);
      const state = JSON.parse(docker(["inspect", client]).stdout)[0].State;
      if (good) assert.equal(state.ExitCode, 0, `test CA must be trusted via system roots: ${result.stderr}`);
      else { assert.notEqual(state.ExitCode, 0, "wrong CA must fail TLS"); assert.match(result.stderr, /certificate signed by unknown authority/, "wrong CA fails at TLS trust boundary"); }
    }
    console.log(`${role} scratch: shipped CA hash/licences, absent shell/GPU tools, directory modes, non-root re-exec and mounted default roots (trusted/untrusted) passed`);
  } finally {
    for (const id of containers) docker(["rm", "-f", id], false);
    for (const volume of volumes) docker(["volume", "rm", volume], false);
    rmSync(temp, { recursive: true, force: true });
  }
}
