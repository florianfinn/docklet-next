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
const common = ["--label", "t3.project=docklet-next", "-v", `${volume}:/app/data`, "-e", `ENCRYPTION_KEY=${encryption}`, "-e", "APP_URL=http://localhost:3552"];
function docker(args, required = true) {
  const result = spawnSync("docker", args, { encoding: "utf8", maxBuffer: 4 * 1024 * 1024 });
  if (required && result.status !== 0) throw new Error("candidate image operation failed");
  return result;
}
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let id;
try {
  docker(["volume", "create", "--label", "t3.project=docklet-next", volume]);
  id = docker(["run", "-d", ...common, "-p", `127.0.0.1:${hostPort||""}:3552`, image]).stdout.trim();
  const port = docker(["port", id, "3552/tcp"]).stdout.trim();
  if (!/^127\.0\.0\.1:\d+$/.test(port)) throw new Error("candidate port must bind loopback");
  const base = `http://${port}`;
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
  docker(["restart", id]); await ready();
  const target = resolve(".scratch", `image-smoke-${suffix}`); mkdirSync(target, { recursive: true });
  docker(["cp", `${id}:/usr/share/licenses/docklet-next/.`, target]);
  for (const path of ["LICENSE", "NOTICE", "LICENSES/BSD-3-Clause-Arcane.txt", "go/github.com/libtnb/sqlite/LICENSE", "go/modernc.org/sqlite/LICENSE", "go/github.com/italypaleale/francis/LICENSE.md", "web/index.json"])
    if (!readFileSync(join(target, path)).length) throw new Error("packaged licence evidence is missing");
  const digest = path => createHash("sha256").update(readFileSync(path)).digest("hex");
  const inventory = JSON.parse(readFileSync(join(target,"docs/provenance/core-import.json")));
  for (const module of inventory.packages) for(const file of module.legalFiles)
    if(digest(join(target,"go",module.name,file.path)) !== file.sha256) throw new Error("extracted Go licence hash mismatch");
  const web = JSON.parse(readFileSync(join(target,"web/index.json")));
  for(const module of web) for(const file of module.evidence)
    if(digest(join(target,"web",`${module.name}@${module.version}`,file.path)) !== file.sha256) throw new Error("extracted web licence hash mismatch");
  if (!readdirSync(join(target, "web")).length) throw new Error("web licence bundle is empty");
  console.log("Candidate image: empty install, health, production UI, second-hub refusal, restart and extracted licence evidence passed.");
} finally {
  if (id) docker(["rm", "-f", id], false);
  docker(["volume", "rm", volume], false);
}
