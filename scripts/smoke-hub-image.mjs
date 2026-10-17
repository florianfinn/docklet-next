import { spawnSync } from "node:child_process";
import { randomBytes, createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";

const image = process.argv[2];
const hostPort=Number(process.argv[3]??0);
if(!Number.isInteger(hostPort)||hostPort<0||hostPort>65535)throw new Error("invalid smoke port");
if (!image) throw new Error("expected candidate image");
const suffix = randomBytes(6).toString("hex"), volume = `docklet-sqlite-smoke-${suffix}`;
const encryption = randomBytes(32).toString("hex");
const adminKey = "arc_" + randomBytes(32).toString("hex"), backups = `${volume}-backups`;
const labels = process.env.SMOKE_DOCKER_LABEL ? ["--label", process.env.SMOKE_DOCKER_LABEL] : [];
const common = [...labels, "-v", `${volume}:/app/data`, "-v", `${backups}:/var/lib/docklet/backups`, "-e", `ADMIN_STATIC_API_KEY=${adminKey}`, "-e", `ENCRYPTION_KEY=${encryption}`, "-e", "APP_URL=http://localhost:3552", "-e", "TZ=Europe/Berlin"];
function docker(args, required = true) {
  const result = spawnSync("docker", args, { encoding: "utf8", maxBuffer: 4 * 1024 * 1024 });
  if (required && result.status !== 0) throw new Error("candidate image operation failed");
  return result;
}
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let id;
try {
  docker(["volume", "create", ...labels, volume]);
  docker(["volume", "create", ...labels, backups]);
  id = docker(["run", "-d", ...common, "-p", `127.0.0.1:${hostPort||""}:3552`, image]).stdout.trim();
  // An ephemeral host port is reassigned on restart, so resolve it again after each start.
  let base;
  function resolveBase() {
    const port = docker(["port", id, "3552/tcp"]).stdout.trim();
    if (!/^127\.0\.0\.1:\d+$/.test(port)) throw new Error("candidate port must bind loopback");
    base = `http://${port}`;
  }
  resolveBase();
  async function ready() {
    for (let attempt = 0; attempt < 120; attempt++) {
      try { if ((await fetch(`${base}/api/health`, { signal: AbortSignal.timeout(1000) })).ok) return; } catch { /* Startup may still be running. */ }
      await delay(500);
    }
    throw new Error("candidate never became healthy");
  }
  await ready();
  const html = await (await fetch(base)).text();
  const asset = html.match(/src="(\/assets\/[^\"]+\.js)"/);
  if (!asset || !(await fetch(base + asset[1])).ok) throw new Error("production web bundle was not served");
  const second = docker(["run", "--rm", ...common, "--network", "none", image], false);
  if (second.status === 0 || !`${second.stdout}${second.stderr}`.includes("hub.already_running")) throw new Error("second hub was not refused");
  docker(["restart", id]); resolveBase(); await ready();
  async function api(path, init = {}) {
    const response = await fetch(`${base}/api${path}`, { ...init, headers: { "X-API-Key": adminKey, ...init.headers }, signal: AbortSignal.timeout(10000) });
    if (!response.ok) throw new Error("candidate backup API operation failed");
    return response;
  }
  const settings = await (await api("/environments/0/settings")).json();
  if (!Array.isArray(settings) || settings.find(item => item.key === "effectiveTimeZone")?.value !== "Europe/Berlin")
    throw new Error("candidate did not resolve the configured IANA time zone");
  const storage = await (await api("/backups/storage")).json();
  if (!storage.available) throw new Error("candidate backup volume is unavailable");
  const created = await (await api("/backups", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" })).json();
  let completed;
  for (let attempt = 0; attempt < 120; attempt++) {
    try {
      const list = await (await api("/backups?limit=100")).json();
      completed = list.data.find(item => item.id === created.id && item.status === "succeeded");
      if (completed) break;
    } catch { /* The coordinated snapshot briefly blocks database requests. */ }
    await delay(250);
  }
  if (!completed || completed.archiveVersion !== 1) throw new Error("candidate did not create a SQLite archive");
  const archive = Buffer.from(await (await api(`/backups/${created.id}/download`)).arrayBuffer());
  if (archive.length !== completed.size) throw new Error("candidate archive download was incomplete");
  const uploaded = await (await api("/backups/upload", { method: "POST", headers: { "Content-Type": "application/octet-stream" }, body: archive })).json();
  docker(["stop", id]);
  docker(["run", "--rm", ...common, "--network", "none", image, "restore-hub", "--backup-id", uploaded.backupId]);
  docker(["start", id]); resolveBase(); await ready();
  const restored = await (await api("/backups?limit=100")).json();
  if (!restored.data.some(item => item.id === uploaded.backupId && item.status === "succeeded")) throw new Error("candidate offline restore lost archive metadata");

  const target = resolve(".scratch", `image-smoke-${suffix}`); mkdirSync(target, { recursive: true });
  docker(["cp", `${id}:/usr/share/licenses/docklet-next/.`, target]);
  for (const path of ["LICENSE", "NOTICE", "LICENSES/BSD-3-Clause-Arcane.txt", "go/github.com/libtnb/sqlite/LICENSE", "go/modernc.org/sqlite/LICENSE", "go/github.com/italypaleale/francis/LICENSE.md", "web/index.json"])
    if (!readFileSync(join(target, path)).length) throw new Error("packaged licence evidence is missing");
  const digest = path => createHash("sha256").update(readFileSync(path)).digest("hex");
  const policy = JSON.parse(readFileSync("scripts/license-policy.json"));
  if (digest(join(target, policy.source)) !== policy.sha256) throw new Error("extracted Arcane licence hash mismatch");
  const inventory = JSON.parse(readFileSync(join(target,"docs/provenance/core-import.json")));
  for (const module of inventory.packages) for(const file of module.legalFiles)
    if(digest(join(target,"go",module.name,file.path)) !== file.sha256) throw new Error("extracted Go licence hash mismatch");
  const web = JSON.parse(readFileSync(join(target,"web/index.json")));
  for(const module of web) for(const file of module.evidence)
    if(digest(join(target,"web",`${module.name}@${module.version}`,file.path)) !== file.sha256) throw new Error("extracted web licence hash mismatch");
  if (!readdirSync(join(target, "web")).length) throw new Error("web licence bundle is empty");
  console.log("Candidate image: empty install, health, production UI, Europe/Berlin time zone, second-hub refusal, restart, SQLite create/download/upload/offline restore and extracted licence evidence passed.");
} finally {
  if (id) docker(["rm", "-f", id], false);
  docker(["volume", "rm", volume], false);
  docker(["volume", "rm", backups], false);
}
