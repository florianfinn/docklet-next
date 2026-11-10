// Source: docklet-next/issues/84
// Revision: 8c30a9afd998fed11dc97f61068507f57560553a
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original isolated agent image smoke test, without a Docker socket mount; shell-free scratch and trust-root evidence.

import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { docker, labels, smokeRuntime } from "./release-smoke.mjs";

const image = process.argv[2];
if (!image) throw new Error("expected candidate agent image");
const hostPort = Number(process.argv[3] ?? process.env.SMOKE_PORT ?? 0);
if (!Number.isInteger(hostPort) || hostPort < 0 || hostPort > 65535) throw new Error("invalid smoke port");
const volume = `docklet-agent-smoke-${randomBytes(6).toString("hex")}`;
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let id;
try {
  docker(["volume", "create", ...labels, volume]);
  id = docker(["run", "-d", ...labels, "-v", `${volume}:/app/data`, "-p", `127.0.0.1:${hostPort || ""}:3552`, "-e", "AGENT_TOKEN=test-only", "-e", "MANAGER_API_URL=", image]).stdout.trim();
  let healthy = false;
  for (let attempt = 0; attempt < 60; attempt++) {
    if (docker(["exec", id, "/usr/local/bin/docklet-agent", "health"], false).status === 0) { healthy = true; break; }
    await delay(500);
  }
  if (!healthy) throw new Error("candidate agent never became healthy");
  smokeRuntime(image, "agent", id);
  const port = docker(["port", id, "3552/tcp"]).stdout.trim();
  assert.match(port, /^127\.0\.0\.1:\d+$/);
  const base = `http://${port}`;
  const headers = { "X-API-Key": "test-only" };
  const version = await fetch(`${base}/api/app-version`, { headers, signal: AbortSignal.timeout(5000) });
  assert.equal(version.status, 200);
  assert.ok((await version.json()).agentContractVersion, "candidate agent contract missing");
  for (const path of ["/", "/api/auth/mode"]) assert.equal((await fetch(base + path, { headers, signal: AbortSignal.timeout(5000) })).status, 404, "candidate agent exposed a hub surface");
  const temp = mkdtempSync(join(tmpdir(), "docklet-agent-legal-"));
  try {
    docker(["cp", `${id}:/usr/share/licenses/docklet-next/LICENSE`, join(temp, "LICENSE")]);
    assert.deepEqual(readFileSync(join(temp, "LICENSE")), readFileSync("LICENSE"));
  } finally { rmSync(temp, { recursive: true, force: true }); }
  console.log("agent image: default agent mode, health, contract, absent hub/UI routes and Go licence evidence passed");
} finally {
  if (id) docker(["rm", "-f", id], false);
  docker(["volume", "rm", volume], false);
}
