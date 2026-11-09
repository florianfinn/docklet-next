// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { buildStamp, layout, releaseReference, sha256, version } from "../deploy/release/common.mjs";
import { build } from "../deploy/release/build.mjs";
import { render } from "../deploy/release/render.mjs";
import { verifyVersion } from "../scripts/release-verify.mjs";
import { fixture } from "./release-fixture.mjs";
const { parse } = createRequire(new URL("../web/package.json", import.meta.url))("yaml");
const v = "1.0.0-rc.1", sha = "a".repeat(40), repo = "registry.example.com/project/docklet";
const pin = role => `${repo}-${role}:${v}@sha256:${"b".repeat(64)}`;
const temporary = fn => { const p = mkdtempSync(join(tmpdir(), "docklet-release-test-")); try { return fn(p); } finally { rmSync(p, { recursive: true, force: true }); } };

test("release references parse registry/path, exact SemVer and lowercase digest; operator overrides are outside this guard", () => {
  assert.equal(releaseReference(pin("hub")).tag, v);
  assert.equal(releaseReference(`localhost:5000/a-b/c_d:0.90.1@sha256:${"a".repeat(64)}`).registry, "localhost:5000");
  for (const ref of [pin("hub").split("@")[0], pin("hub").replace(v, "latest"), pin("hub").replace(v, "1.0"), pin("hub").replace(v, "01.0.0"), pin("hub").replace(v, "1.0.0-rc.01"), pin("hub").replaceAll("b", "B"), `docklet-hub:1.0.0@sha256:${"a".repeat(64)}`]) assert.throws(() => releaseReference(ref));
  for (const s of ["1.0.0", "0.90.0", "1.0.0-rc.10"]) assert.equal(version(s), s);
});

test("rendered package keeps pinned defaults, no build, exact Compose variables and matching checksums", () => temporary(out => {
  const manifest = { version: v, releaseRepository: repo, images: Object.fromEntries(["hub", "agent"].map(role => [role, { reference: pin(role), digest: `sha256:${"b".repeat(64)}` }])) };
  const files = render(manifest, out);
  assert.deepEqual(files["BSD-3-Clause-Arcane.txt"], readFileSync("LICENSES/BSD-3-Clause-Arcane.txt"));
  const compose = parse(files[`docklet-next-${v}-compose.yaml`]);
  assert.equal(compose.services.hub.build, undefined);
  assert.equal(compose.services.hub.image, `\${HUB_IMAGE:-${pin("hub")}}`);
  assert.equal(compose.services.hub.environment.DOCKLET_AGENT_IMAGE, `\${DOCKLET_AGENT_IMAGE:-${pin("agent")}}`);
  const env = files[`docklet-next-${v}.env.example`];
  assert.ok(env.includes(`# HUB_IMAGE=${pin("hub")}`));
  const keys = [...env.matchAll(/^(?:# )?([A-Z_]+)=/gm)].map(m => m[1]).sort();
  assert.deepEqual(keys, [...new Set([...files[`docklet-next-${v}-compose.yaml`].matchAll(/\$\{([A-Z_]+)/g)].map(m => m[1]))].sort());
  for (const [name, bytes] of Object.entries(files)) assert.ok(readFileSync(join(out, "SHA256SUMS"), "utf8").includes(`${sha256(bytes).slice(7)}  ${name}\n`));
  const unpinned = structuredClone(manifest); unpinned.images.agent.reference = unpinned.images.agent.reference.split("@")[0];
  assert.throws(() => render(unpinned, out), /digest/);
}));

test("version verification rejects mismatch and non-tag publication; dry runs use the next train", () => {
  assert.equal(verifyVersion({ event: "pull_request", next: "1.0.0", runNumber: 42 }), "1.0.0-rc.42");
  assert.equal(verifyVersion({ event: "push", ref: "refs/tags/v1.0.0", candidateVersion: "1.0.0" }), "1.0.0");
  assert.throws(() => verifyVersion({ event: "push", ref: "refs/tags/v1.0.1", candidateVersion: "1.0.0" }), /mismatch/);
  assert.throws(() => verifyVersion({ event: "push", ref: "refs/heads/main", candidateVersion: "1.0.0" }));
});

test("build orders agent before hub, stamps pinned default, records and uses repository override", () => temporary(out => {
  const builds = [];
  const manifest = build({ version: v, out, releaseRepository: repo }, (program, args) => {
    if (program === "git") return args[0] === "rev-parse" ? sha : "1700000000";
    assert.equal(program, "docker");
    builds.push(args);
    const role = args[args.indexOf("--target") + 1];
    fixture(join(out, role));
  });
  assert.deepEqual(builds.map(a => a[a.indexOf("--target") + 1]), ["agent", "hub"]);
  assert.equal(manifest.releaseRepository, repo);
  assert.ok(manifest.images.agent.reference.startsWith(`${repo}-agent:${v}@sha256:`));
  const hubFlags = builds[1].find(a => a.startsWith("DOCKLET_LDFLAGS="));
  assert.ok(hubFlags.includes(`DefaultAgentImage=${manifest.images.agent.reference}`));
  assert.ok(hubFlags.includes(`ReleaseRepository=${repo}`));
  assert.equal(JSON.parse(readFileSync(join(out, "release-manifest.json"))).releaseRepository, repo);
  for (const args of builds) { assert.ok(args.includes("linux/amd64")); assert.ok(args.some(a => a.includes("rewrite-timestamp=true"))); }
  assert.equal(layout(join(out, "agent")).digest, manifest.images.agent.digest);
}));

test("real Go linker stamps all five config variables into a reading binary", () => temporary(out => {
  // Isolate the declarations from unrelated backend dependencies; use the real source and linker namespace.
  const module = "github.com/florianfinn/docklet-next/backend";
  mkdirSync(join(out, "internal/config"), { recursive: true });
  writeFileSync(join(out, "go.mod"), `module ${module}\n\ngo 1.27.1\n`);
  writeFileSync(join(out, "internal/config/version.go"), readFileSync("backend/internal/config/version.go"));
  writeFileSync(join(out, "main.go"), `package main\nimport ("encoding/json";"os";"${module}/internal/config")\nfunc main(){json.NewEncoder(os.Stdout).Encode([]string{config.Version,config.Revision,config.BuildTime,config.DefaultAgentImage,config.ReleaseRepository})}\n`);
  const flags = buildStamp({ version: v, revision: sha, epoch: "1700000000", agentImage: pin("agent"), releaseRepository: repo });
  const built = spawnSync("go", ["build", "-ldflags", flags, "-o", join(out, "stamp"), "."], { cwd: out, encoding: "utf8", env: { ...process.env, GOWORK: "off", CGO_ENABLED: "0" } });
  assert.equal(built.status, 0, built.stderr);
  const result = spawnSync(join(out, "stamp"), [], { encoding: "utf8" });
  assert.equal(result.status, 0);
  assert.deepEqual(JSON.parse(result.stdout), [v, sha, "2023-11-14T22:13:20Z", pin("agent"), repo]);
}));

test("scratch recipe ships assessed CA roots and every required licence, without runtime shell commands", () => {
  const dockerfile = readFileSync("deploy/hub/Dockerfile", "utf8");
  const assessed = JSON.parse(readFileSync("deploy/release/ca-bundle.json"));
  assert.ok(dockerfile.includes(assessed.sha256));
  for (const path of assessed.notices) { assert.ok(existsSync(path), `missing CA licence: ${path}`); assert.ok(readFileSync(path).length > 100); assert.ok(dockerfile.includes(path)); }
  for (const role of ["agent", "hub"]) {
    const runtime = dockerfile.split(`FROM scratch AS ${role}\n`)[1].split(/\nFROM /)[0];
    assert.ok(runtime.includes("COPY --from=runtime-files /etc/ssl/certs/ca-certificates.crt"));
    assert.doesNotMatch(runtime, /^RUN /m);
    assert.match(runtime, /HEALTHCHECK.*CMD \["\/usr\/local\/bin\/docklet-/);
  }
  for (const smoke of ["scripts/smoke-hub-image.mjs", "scripts/smoke-agent-image.mjs", "scripts/release-smoke.mjs"]) assert.doesNotMatch(readFileSync(smoke, "utf8"), /"sh"|"wget"/);
});

test("workflow evaluates tag-only publish, protected environment, least permissions and exact PR source", () => {
  const workflow = parse(readFileSync(".github/workflows/release.yml", "utf8"));
  assert.deepEqual(workflow.permissions, {});
  assert.deepEqual(workflow.on.push.tags, ["v*"]);
  assert.ok(workflow.on.pull_request.paths.includes("backend/**"));
  const publish = workflow.jobs.publish;
  const expr = (publish.if ?? "true").replace(/^\$\{\{(.*)\}\}$/, "$1");
  const evaluate = (event, ref) => Function("github", "startsWith", `return (${expr})`)({ event_name: event, ref }, (a, b) => a.startsWith(b));
  assert.equal(evaluate("pull_request", "refs/pull/1/merge"), false);
  assert.equal(evaluate("push", "refs/heads/main"), false);
  assert.equal(evaluate("push", "refs/tags/v1.0.0-rc.1"), true);
  assert.equal(publish.environment?.name ?? publish.environment, "release");
  for (const [name, job] of Object.entries(workflow.jobs)) {
    for (const [permission, value] of Object.entries(job.permissions ?? {})) if (value === "write") { assert.equal(name, "publish"); assert.ok(["contents", "packages"].includes(permission)); }
    assert.equal(job["runs-on"], "ubuntu-24.04");
    if (name !== "publish") assert.ok(job.steps.some(s => s.with?.ref?.includes("github.event.pull_request.head.sha")));
    for (const step of job.steps) if (step.uses) assert.match(step.uses, /@[a-f0-9]{40}$/);
  }
  assert.equal(publish.permissions.contents, "write"); assert.equal(publish.permissions.packages, "write");
  assert.deepEqual(publish.needs, ["verify", "build", "package"]);
  assert.ok(workflow.jobs.package.steps.some(s => s.run?.includes("smoke-hub-image.mjs")));
  assert.doesNotMatch(JSON.stringify(workflow), /pull_request_target|workflow_dispatch|docker push/);
});
