// Source: docklet-next/issues/84
// Revision: 8c30a9afd998fed11dc97f61068507f57560553a
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original isolated agent image smoke test, without a Docker socket mount.

import { spawnSync } from "node:child_process";

const image = process.argv[2];
if (!image) throw new Error("expected candidate agent image");
const labels = process.env.SMOKE_DOCKER_LABEL ? ["--label", process.env.SMOKE_DOCKER_LABEL] : [];
function docker(args, required = true) {
  const result = spawnSync("docker", args, { encoding: "utf8", timeout: 20000 });
  if (required && result.status !== 0) throw new Error("candidate agent image operation failed");
  return result;
}
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
let id;
try {
  docker(["run", "--rm", ...labels, "--network", "none", "--entrypoint", "sh", image, "-ec", "test -x /usr/local/bin/docklet-agent; test ! -e /usr/local/bin/docklet-hub; test ! -d /usr/share/licenses/docklet-next/web; test -s /usr/share/licenses/docklet-next/LICENSE"]);
  id = docker(["run", "-d", ...labels, "--network", "none", "-e", "AGENT_TOKEN=test-only", "-e", "MANAGER_API_URL=", image]).stdout.trim();
  let healthy = false;
  for (let attempt = 0; attempt < 60; attempt++) {
    if (docker(["exec", id, "/usr/local/bin/docklet-agent", "health"], false).status === 0) { healthy = true; break; }
    await delay(500);
  }
  if (!healthy) throw new Error("candidate agent never became healthy");
  const version = docker(["exec", id, "wget", "-qO-", "--header=X-API-Key: test-only", "http://127.0.0.1:3552/api/app-version"]).stdout;
  if (!version.includes('"agentContractVersion"')) throw new Error("candidate agent contract missing");
  for (const path of ["/", "/api/auth/mode"]) {
    const absent = docker(["exec", id, "wget", "-S", "-O", "/dev/null", "--header=X-API-Key: test-only", `http://127.0.0.1:3552${path}`], false);
    if (!`${absent.stdout}${absent.stderr}`.includes("404 Not Found")) throw new Error("candidate agent exposed a hub surface");
  }
  console.log("agent image: default agent mode, health, contract, absent hub/UI routes and Go licence evidence passed");
} finally {
  if (id) docker(["rm", "-f", id], false);
}
