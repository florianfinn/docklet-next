// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original licence evidence for compiled Go modules added after the Arcane adoption baseline.

// Assesses every compiled hub/agent module version absent from arcane-binary.json with the same
// legal-file, blocked-licence and copyright rules as the adoption inventory.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { assertLicense, hash, inspectLegal, legalFiles } from "./arcane-license-policy.mjs";

const file = "docs/provenance/core-added-modules.json";
const go = (args, env = {}) => execFileSync("go", ["run", resolve("scripts/go-command.go"), "--", ...args], {
  encoding: "utf8", maxBuffer: 64 * 1024 * 1024, env: { ...process.env, GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "0", ...env },
});
const objects = (text) => text.trim() ? JSON.parse(`[${text.trim().replace(/}\s*{/g, "},{")}]`) : [];
const baseline = new Set(JSON.parse(readFileSync("docs/provenance/arcane-binary.json", "utf8")).packages.map((p) => `${p.name}@${p.version}`));
const local = (name) => name.startsWith("github.com/florianfinn/docklet-next/") || name.startsWith("go.getarcane.app/") || name === "github.com/italypaleale/francis";
const sanitize = (text) => text.replace(/[\w.!#$%&'*+/=?^`{|}~-]+@[\w.-]+\.[A-Za-z]{2,}/g, "[address omitted]").trim();
const copyrights = (text) => [...new Set(text.split(/\r?\n/).filter((s) => /copyright[^\r\n]*(?:[12][0-9]{3}|\([cC]\)|©)/i.test(s)).map(sanitize))];

const modules = new Map();
for (const [target, tags] of [["hub", []], ["agent", ["-tags=exclude_frontend"]]]) {
  for (const p of objects(go(["list", "-deps", ...tags, "-json=ImportPath,Dir,GoFiles,Module", "./backend/cmd"]))) {
    if (!p.Module || local(p.Module.Path)) continue;
    const version = p.Module.Replace?.Version ?? p.Module.Version;
    const key = `${p.Module.Path}@${version}`;
    if (baseline.has(key)) continue;
    const entry = modules.get(key) ?? { name: p.Module.Path, version, targets: [], packages: [] };
    if (!entry.targets.includes(target)) entry.targets.push(target);
    if (!entry.packages.some((q) => q.ImportPath === p.ImportPath)) entry.packages.push(p);
    modules.set(key, entry);
  }
}
const downloads = new Map(objects(go(["mod", "download", "-json", ...[...modules.keys()]])).map((d) => [`${d.Path}@${d.Version}`, d]));
const rows = [...modules.values()].sort((a, b) => a.name.localeCompare(b.name, "en")).map((item) => {
  const download = downloads.get(`${item.name}@${item.version}`);
  if (!download || download.Error) throw new Error(`module download failed: ${item.name}`);
  const zip = `https://proxy.golang.org/${item.name.replace(/[A-Z]/g, (s) => "!" + s.toLowerCase())}/@v/${item.version}.zip`;
  const evidence = (path) => {
    const text = readFileSync(path, "utf8");
    const rel = relative(download.Dir, path);
    return { path: rel, sha256: hash(text), source: zip, ...inspectLegal(item.name, item.version, rel, text), copyright: copyrights(text) };
  };
  const rootFiles = legalFiles(download.Dir);
  const licenseFiles = rootFiles.map(evidence);
  const nested = legalFiles(download.Dir, true).filter((p) => !rootFiles.includes(p)).map(evidence);
  const license = [...new Set([...licenseFiles, ...nested].filter((f) => f.review?.scope !== "non-code").flatMap((f) => (f.detectedLicense ?? "").split(" AND ")).filter(Boolean))].sort().join(" AND ");
  try { assertLicense(license); } catch { throw new Error(`unapproved module licence: ${item.name}: ${license}`); }
  const copyrightSources = [];
  if (!licenseFiles.some((f) => f.copyright.length)) {
    const seen = new Set();
    for (const p of item.packages) for (const name of p.GoFiles ?? []) {
      const text = readFileSync(join(p.Dir, name), "utf8");
      const statements = copyrights(text.slice(0, 4096)).filter((s) => !seen.has(s));
      statements.forEach((s) => seen.add(s));
      if (statements.length) copyrightSources.push({ path: relative(download.Dir, join(p.Dir, name)), sha256: hash(text), copyright: statements });
    }
  }
  const origin = download.Origin;
  return { name: item.name, version: item.version, source: origin?.URL ?? zip, sourceRevision: origin?.Hash ?? item.version,
    correspondingSource: origin?.URL ? `${origin.URL}/tree/${origin.Hash}${origin.Subdir ? `/${origin.Subdir}` : ""}` : zip,
    checksums: { module: download.Sum, goMod: download.GoModSum }, targets: item.targets.sort(), binaryPackageCount: item.packages.length,
    license, copyright: [...new Set([...licenseFiles, ...copyrightSources].flatMap((f) => f.copyright))], copyrightSources,
    licenseFiles, nestedLicenseEvidence: nested, compiledLicenseNotices: [],
    redistributionObligations: "Preserve applicable copyright, licence and NOTICE texts in source and binary distributions; no endorsement." };
});
const head = { schemaVersion: 1, ecosystem: "go", date: "2026-11-22", target: "linux/amd64", tool: "go list -deps; go mod download (Go 1.27.4)",
  reason: "Compiled modules added or upgraded after the Arcane adoption baseline (#573 embedded Tailscale route)." };
writeFileSync(file, JSON.stringify(head, null, 2).slice(0, -2) + ',\n  "packages": [\n' + rows.map((p) => "    " + JSON.stringify(p)).join(",\n") + "\n  ]\n}\n");
console.log(JSON.stringify({ modules: rows.length, licenses: [...new Set(rows.map((r) => r.license))] }));
