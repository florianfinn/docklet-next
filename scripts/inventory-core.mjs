import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { tracked } from "./repository.mjs";

const file = "docs/provenance/core-import.json";
const hash = (value) => createHash("sha256").update(value).digest("hex");
const read = (path) => JSON.parse(readFileSync(path, "utf8"));
const paths = tracked().filter((path) => /(?:^|\/)go\.(?:mod|sum|work|work.sum)$/.test(path));
const lockfiles = paths.map((path) => ({ path, sha256: hash(readFileSync(path)) }));
const check = process.argv.includes("--check");
if (check) {
  for (const entry of read("scripts/license-policy.json").adoptedSources) {
    if (hash(readFileSync(entry.path)) !== entry.sha256) throw new Error("adopted-license-drift");
  }
  if (JSON.stringify(read(file).lockfiles) !== JSON.stringify(lockfiles)) throw new Error("core-inventory-lockfile-drift");
} else {
  const source = read("docs/provenance/arcane-modules.json");
  const original = read("docs/provenance/arcane-binary.json");
  const namespace = (name) => name.replace(/^github\.com\/florianfinn\/docklet-next\/(backend|types|cli)(?=\/|$)/, "github.com/getarcaneapp/arcane/$1/v2");
  const records = new Map();
  const targets = [];
  for (const [target, cgo, flags] of [["hub", "1", []], ["agent", "0", ["-tags=exclude_frontend"]]]) {
    const format = '{{if .Module}}{{.ImportPath}}|{{.Module.Path}}|{{.Module.Version}}{{with .Module.Replace}}|{{.Path}}|{{.Version}}{{end}}{{end}}';
    const result = execFileSync("go", ["run", resolve("scripts/go-command.go"), "--", "list", "-deps", ...flags, "-f", format, "./backend/cmd/..."], {
      encoding: "utf8", maxBuffer: 32 * 1024 * 1024,
      env: { ...process.env, CGO_ENABLED: cgo, GOOS: "linux", GOARCH: "amd64" },
    });
    const packages = result.trim().split("\n").filter(Boolean);
    targets.push({ target, cgo, tags: flags, packages: packages.length, packageGraphSha256: hash(packages.map((line) => line.split("|")[0]).sort().join("\n") + "\n") });
    for (const line of packages) {
      const [, name, declared, replacement, version] = line.split("|");
      const upstream = namespace(name);
      const local = name !== upstream || name.startsWith("go.getarcane.app/");
      const selected = local ? source.packages.find((p) => p.name === upstream && (p.sourceTree || p.version === source.revision)) :
        original.packages.find((p) => p.name === name && p.version === (version || declared));
      if (!selected) throw new Error(`unassessed-core-module: ${name}`);
      const record = records.get(name) ?? { name, version: local ? selected.sourceRevision : version || declared,
        localPath: local ? (name !== upstream ? name.split("/").at(-1) : "third_party/arcane/" + name.slice("go.getarcane.app/".length)) : null,
        upstream: selected.name, upstreamVersion: selected.version,
        license: ["go.getarcane.app/builds", "go.getarcane.app/kit"].includes(name) ? "BSD-3-Clause AND Apache-2.0" : selected.license, evidence: "arcane-modules.json", targets: [],
        modifications: local ? "Provenance; local module wiring; exact file deviations in source headers or scripts/provenance.json." : "None" };
      if (!record.targets.includes(target)) record.targets.push(target);
      records.set(name, record);
    }
  }
  const metadata = { schemaVersion: 1, ecosystem: "go", date: "2026-10-13", sourceRevision: source.revision,
    target: "linux/amd64", tool: "go list -deps (Go 1.27.2)", lockfiles, targets,
    limitations: "Pre-cut core; no release package, standalone CLI binary licence graph or container inventory. Exact upstream licence evidence remains in arcane-modules.json and arcane-binary.json." };
  const packages = [...records.values()].sort((a, b) => a.name.localeCompare(b.name, "en"));
  writeFileSync(file, JSON.stringify(metadata, null, 2).slice(0, -2) + ',\n  "packages": [\n' + packages.map((p) => '    ' + JSON.stringify(p)).join(',\n') + '\n  ]\n}\n');
  console.log(JSON.stringify({ modules: packages.length, targets }));
}
