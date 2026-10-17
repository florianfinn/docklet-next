// Source: docklet-next/issues/84
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original installer security and shell regression tests.

// @vitest-environment node
import { execFileSync, spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { createTar } from "@/shared/lib/tar";
import { buildInstallArtifacts, installPackageFiles, type AgentInstallPlan } from "./agent-install";
import { absolutePath, installDirProblem } from "./validation";

const roots: string[] = [];
afterEach(() => { for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true }); });
const plan: AgentInstallPlan = {
  hostName: "Example host", connection: "edge", address: "agent.example.com", port: 3552,
  hubUrl: "http://hub.example.com", projectDir: "/srv/projects", installDir: "/srv/projects/docklet-agent",
  token: "test-only", timeZone: "UTC", image: "registry.example.com/docklet/agent:0.1.0",
};

function fixture(outside = false) {
  const root = mkdtempSync(join(tmpdir(), "docklet-installer-"));
  roots.push(root);
  for (const dir of ["projects", "package", "bin", "temp"]) mkdirSync(join(root, dir));
  const installPlan = { ...plan, projectDir: join(root, "projects"), installDir: outside ? join(root, "agent") : join(root, "projects", "agent") };
  const a = buildInstallArtifacts(installPlan);
  for (const file of installPackageFiles(a)) writeFileSync(join(root, "package", file.name), file.content, { mode: file.mode });
  // Canonical Compose output; no Docker endpoint or running service is used.
  writeFileSync(join(root, "bin", "docker"), `#!/bin/sh
set -eu
file=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -f) file=$2; shift 2;;
    config) cat "$file"; exit 0;;
    up) stat -c '%a' .env > "$TEST_ROOT/env-mode"; exit 0;;
    *) shift;;
  esac
done
exit 1
`, { mode: 0o700 });
  const env: NodeJS.ProcessEnv = { ...process.env, PATH: `${join(root, "bin")}:${process.env.PATH}`, TMPDIR: join(root, "temp"), TEST_ROOT: root };
  delete env.DOCKER_HOST;
  function run(kind: "script" | "composeCommand" | "scriptCommand" = "script") {
    return spawnSync("sh", kind === "script" ? [join(root, "package", "install.sh")] : ["-c", a[kind]], { env, encoding: "utf8" });
  }
  return { root, installPlan, a, run };
}

describe("agent installer security contract", () => {
  it("archives only .env with the token and stores private modes", () => {
    const a = buildInstallArtifacts(plan);
    const tar = createTar(installPackageFiles(a), 0);
    const entries: { name: string; mode: number; body: string }[] = [];
    const text = new TextDecoder();
    for (let offset = 0; tar[offset];) {
      const name = text.decode(tar.slice(offset, offset + 100)).replace(/\0.*$/, "");
      const mode = parseInt(text.decode(tar.slice(offset + 100, offset + 108)), 8);
      const size = parseInt(text.decode(tar.slice(offset + 124, offset + 136)), 8);
      entries.push({ name, mode, body: text.decode(tar.slice(offset + 512, offset + 512 + size)) });
      offset += 512 + Math.ceil(size / 512) * 512;
    }
    expect(entries.map(({ name, mode }) => [name, mode])).toEqual([
      ["compose.yaml", 0o644], [".env", 0o600], ["install.sh", 0o700], ["README.txt", 0o644],
    ]);
    expect(entries.filter((entry) => entry.body.includes(plan.token)).map((entry) => entry.name)).toEqual([".env"]);
    expect(a.readme).toContain("DELETE THE ARCHIVE");
  });

  it("uses private umask, random temporary directories and cleanup traps", () => {
    const a = buildInstallArtifacts(plan);
    for (const value of [a.script, a.scriptCommand, a.composeCommand]) {
      expect(value).toContain("umask 077");
      expect(value).toContain("mktemp -d");
      expect(value).toContain("trap ");
      expect(value).not.toContain("/tmp/");
      expect(spawnSync("sh", ["-n"], { input: value }).status).toBe(0);
    }
    expect(a.script).not.toContain(plan.token);
    expect(a.script).toContain('"$PACKAGE_DIR/.env"');
  });

  it.each(["script", "composeCommand", "scriptCommand"] as const)("%s installs privately and reinstalls recognised agent in place", (kind) => {
    const f = fixture();
    for (let i = 0; i < 2; i++) {
      const result = f.run(kind);
      expect(result.stderr).toBe("");
      expect(result.status).toBe(0);
      expect(statSync(join(f.installPlan.installDir, ".env")).mode & 0o777).toBe(0o600);
      expect(readFileSync(join(f.root, "env-mode"), "utf8").trim()).toBe("600");
      expect(readFileSync(join(f.installPlan.installDir, ".env"), "utf8")).toContain(`PUID=${statSync(f.installPlan.projectDir).uid}`);
      expect(readFileSync(join(f.installPlan.installDir, ".env"), "utf8")).toContain(plan.token);
      expect(readdirSync(join(f.root, "temp"))).toEqual([]);
      expect(readdirSync(f.installPlan.installDir).filter((name) => name.startsWith(".docklet-install"))).toEqual([]);
    }
  });

  it.each(["script", "composeCommand", "scriptCommand"] as const)("%s refuses unrelated compose before writes", (kind) => {
    const f = fixture();
    mkdirSync(f.installPlan.installDir);
    const existing = "services:\n  database:\n    image: postgres:18.6\n";
    writeFileSync(join(f.installPlan.installDir, "compose.yaml"), existing);
    const result = f.run(kind);
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("Refusing to overwrite");
    expect(readdirSync(f.installPlan.installDir)).toEqual(["compose.yaml"]);
    expect(readFileSync(join(f.installPlan.installDir, "compose.yaml"), "utf8")).toBe(existing);
    expect(readdirSync(join(f.root, "temp"))).toEqual([]);
  });

  it.each(["docker-compose.yml", "compose.override.yaml", "compose.yaml"])("checks alternate, override and mixed service files (%s)", (name) => {
    const f = fixture();
    mkdirSync(f.installPlan.installDir);
    writeFileSync(join(f.installPlan.installDir, name), `${f.a.compose}  database:\n    image: postgres:18.6\n`);
    expect(f.run().status).toBe(1);
    expect(readdirSync(f.installPlan.installDir)).toEqual([name]);
  });

  it("rejects missing project directories and canonical directory aliases before writes", () => {
    const f = fixture();
    rmSync(f.installPlan.projectDir, { recursive: true });
    expect(f.run().status).toBe(1);
    expect(readdirSync(f.root).includes("env-mode")).toBe(false);
    mkdirSync(f.installPlan.projectDir);
    execFileSync("ln", ["-s", f.installPlan.projectDir, join(f.root, "alias")]);
    const alias = buildInstallArtifacts({ ...f.installPlan, installDir: join(f.root, "alias") });
    writeFileSync(join(f.root, "package", "install.sh"), alias.script);
    expect(f.run().status).toBe(1);
    expect(readdirSync(f.installPlan.projectDir)).toEqual([]);
  });

  it("uses Arcane configuration and manual-only supplementary groups", () => {
    const auto = buildInstallArtifacts(plan);
    expect(auto.env).toContain("AGENT_MODE=true\nEDGE_AGENT=true\nAGENT_TOKEN=");
    expect(auto.env).toContain("PROJECTS_DIRECTORY=");
    expect(auto.env).not.toMatch(/DOCKLET_(MODE|HUB_URL|AGENT_TOKEN|PROJECT_DIR|LISTEN)/);
    expect(auto.compose).not.toContain("group_add");
    expect(auto.compose).not.toContain("ports:");
    expect(auto.compose).toContain('com.getarcaneapp.arcane.hidden: "true"');
    const manual = buildInstallArtifacts({ ...plan, connection: "direct", fileOwner: { uid: 123, gid: 456 }, socketGid: 0 });
    expect(manual.compose).toContain('group_add:\n      - "0"');
    expect(manual.compose).toContain('"3552:3552"');
    expect(manual.env).toContain("PUID=123\nPGID=456");
    expect(manual.compose).toContain(plan.image);
  });

  it.each([
    ["/srv/projects", "sameDir"], ["/srv/projects/agent", undefined], ["/opt/agent", undefined],
    ["/srv", "containsProject"], ["/srv/projects/stack/agent", "insideStack"],
  ])("validates directory relation %s", (installDir, problem) => {
    expect(installDirProblem(plan.projectDir, installDir as string)).toBe(problem);
  });

  it("rejects dot aliases and unsafe or unpinned plans", () => {
    expect(absolutePath("/srv/projects/.")).toBe("pathChars");
    for (const patch of [{ image: "registry.example.com/agent:latest" }, { token: "<invalid>" }, { projectDir: "/srv/projects/." }, { hubUrl: "https://example:example@hub.example.com" }]) {
      expect(() => buildInstallArtifacts({ ...plan, ...patch })).toThrow();
    }
  });
});

it.each([["192.0.2.12", "192.0.2.12:"], ["[2001:db8::12]", "[2001:db8::12]:"], ["2001:db8::12", "[2001:db8::12]:"], ["agent.example.com", ""]])("binds direct agent address %s to the configured IP when literal", (address, binding) => {
  const a = buildInstallArtifacts({ ...plan, connection: "direct", address, port: 4552 });
  expect(a.compose).toContain(`      - "${binding}4552:3552"`);
  expect(a.compose).toContain("host firewall");
});

it.each(["script", "composeCommand", "scriptCommand"] as const)("%s warns and continues when only the agent identity can write projects", (kind) => {
  const f = fixture(true);
  chmodSync(f.installPlan.projectDir, 0o555);
  try {
    expect(spawnSync("test", ["-w", f.installPlan.projectDir]).status).toBe(1);
    const result = f.run(kind);
    expect(result.status).toBe(0);
    expect(result.stderr).toContain("Warning: project directory is not writable");
    expect(readFileSync(join(f.root, "env-mode"), "utf8").trim()).toBe("600");
    expect(statSync(f.installPlan.projectDir).mode & 0o777).toBe(0o555);
  } finally { chmodSync(f.installPlan.projectDir, 0o755); }
});
