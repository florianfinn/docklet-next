// Source: docklet-next/README.md
// Revision: e32adfed592cb71a2c8a85c8a5e539e7522ae8bc
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original drift guard for the README quick-start Compose example; project-directory layout; M29 sign-in chosen in setup.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";

const { parse } = createRequire(new URL("../web/package.json", import.meta.url))("yaml");
const readme = readFileSync("README.md", "utf8");
const template = parse(readFileSync("deploy/hub/compose.yaml", "utf8"));
const { next } = JSON.parse(readFileSync("deploy/release/version.json", "utf8"));

function quickStart() {
  const blocks = [...readme.matchAll(/```yaml\n([\s\S]*?)```/g)].map((m) => m[1]);
  assert.equal(blocks.length, 1, "README has exactly one YAML example");
  return parse(blocks[0]);
}

function dotenv() {
  const blocks = [...readme.matchAll(/```dotenv\n([\s\S]*?)```/g)].map((m) => m[1]);
  assert.equal(blocks.length, 1, "README has exactly one dotenv example");
  // Compose drops an unquoted value's inline comment that follows whitespace.
  return Object.fromEntries(blocks[0].trim().split("\n")
    .map((line) => line.replace(/\s+#.*$/, "").split(/=(.*)/s).slice(0, 2)));
}

const destination = (mount) => mount.split(":")[1];
const literal = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

test("README quick start pins the official hub image at the release train version", () => {
  const example = quickStart();
  assert.deepEqual(Object.keys(example.services), ["hub"]);
  // The in-app update rewrites HUB_IMAGE in .env; a literal reference would refuse it.
  assert.equal(example.services.hub.image, `\${HUB_IMAGE:-ghcr.io/florianfinn/docklet-next-hub:${next}}`);
  assert.match(readme, /^\| `HUB_IMAGE` \|/m);
});

test("README quick start keeps the template's container paths and runtime settings", () => {
  const hub = quickStart().services.hub;
  const source = template.services.hub;
  for (const key of ["restart", "stop_grace_period"]) assert.equal(hub[key], source[key], key);
  assert.deepEqual(hub.labels, { "io.docklet.system": "hub" }, "the install-directory label keeps Docklet out of the stack list");
  assert.deepEqual(hub.labels, source.labels);
  assert.equal(hub.environment.TZ, source.environment.TZ);
  // The example relies on the image defaults; they must equal the template's explicit container paths.
  const dockerfile = readFileSync("deploy/hub/Dockerfile", "utf8");
  assert.match(dockerfile, new RegExp(`^ENV DATABASE_URL=${literal(source.environment.DATABASE_URL)} `, "m"));
  assert.match(readFileSync("backend/internal/config/config.go", "utf8"),
    new RegExp(`env:"DOCKLET_BACKUP_DIR" default:"${literal(source.environment.DOCKLET_BACKUP_DIR)}"`));
  assert.match(readFileSync("backend/pkg/libarcane/startup/runtime_identity.go", "utf8"),
    new RegExp(`defaultDockerConfigDir\\s*=\\s*"${literal(source.environment.DOCKER_CONFIG)}"`));
  // A root runtime gets no DOCKER_CONFIG default, so the README must name the explicit setting.
  assert.ok(readme.includes(`with \`PUID=0\` and \`PGID=0\`, add \`DOCKER_CONFIG: ${source.environment.DOCKER_CONFIG}\` under \`environment\``));
  assert.deepEqual(hub.ports, ["127.0.0.1:3552:3552"], "loopback only, template default port");
  // Same container paths as the template; host sides are the documented project-directory layout.
  const projects = `${hub.environment.PROJECTS_DIRECTORY}:${hub.environment.PROJECTS_DIRECTORY}`;
  assert.deepEqual(hub.volumes.filter((v) => v !== projects).map(destination), source.volumes.map(destination));
  assert.deepEqual(hub.volumes, ["./data:/app/data", "./backups:/var/lib/docklet/backups",
    projects, "/var/run/docker.sock:/var/run/docker.sock"]);
});

test("README quick start mounts the projects directory at the same path", () => {
  const hub = quickStart().services.hub;
  const projects = hub.environment.PROJECTS_DIRECTORY;
  assert.ok(hub.volumes.includes(`${projects}:${projects}`), "projects directory mounted at the same path");
  assert.match(readme, new RegExp(`\`${projects}/docklet/compose\\.yaml\``), "Docklet lives inside the projects directory");
});

test("README quick start requires the deployment inputs and documents every variable", () => {
  const { environment } = quickStart().services.hub;
  // Intentional differences from the package: no image overrides or sign-in inputs; setup chooses the sign-in.
  assert.deepEqual(Object.keys(environment).sort(), ["APP_URL", "ENCRYPTION_KEY", "PGID",
    "PROJECTS_DIRECTORY", "PUID", "TZ"]);
  for (const key of ["APP_URL", "ENCRYPTION_KEY", "PUID", "PGID"]) {
    assert.match(environment[key], new RegExp(`^\\$\\{${key}:\\?`), `${key} is required`);
  }
  for (const key of Object.keys(environment)) {
    assert.match(readme, new RegExp(`^\\| [^|]*\`${key}\``, "m"), `${key} has a table row`);
  }
  const env = dotenv();
  assert.deepEqual(Object.keys(env).sort(), ["APP_URL", "PGID", "PUID", "TZ"], "the key is generated, not written");
  assert.match(env.APP_URL, /^https:\/\/[a-z.-]+\.example\.com$/);
  assert.match(readme, /echo "ENCRYPTION_KEY=\$\(openssl rand -hex 32\)" >> \.env/);
});
