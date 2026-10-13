// Generate adoption evidence from a pristine Arcane checkout; no product code is copied.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync, mkdirSync, existsSync, readdirSync } from "node:fs";
import { resolve, relative, dirname, join } from "node:path";

const [checkout, report, output] = process.argv.slice(2);
if (!checkout || !report || !output) throw new Error("usage: inventory-arcane.mjs CHECKOUT LICENSE_REPORT OUTPUT");
const root = resolve(checkout), out = resolve(output);
const revision = "095cb778daba1034e2a39961199e712a75d4ba29";
const date = "2026-10-13";
const run = (cmd, args, cwd = root, overrides = {}) => execFileSync(cmd, args, {
  cwd, encoding: "utf8", maxBuffer: 100 * 1024 * 1024,
  env: { ...process.env, GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "1", GOFLAGS: "", ...overrides },
});
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const read = (p) => readFileSync(p, "utf8");
function objects(text) {
  const result = [];
  let depth = 0, start = 0, string = false, escaped = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (string) {
      if (escaped) escaped = false;
      else if (c === "\\") escaped = true;
      else if (c === '"') string = false;
    } else if (c === '"') string = true;
    else if (c === "{") { if (!depth) start = i; depth++; }
    else if (c === "}" && --depth === 0) result.push(JSON.parse(text.slice(start, i + 1)));
  }
  return result;
}
if (run("git", ["rev-parse", "HEAD"]).trim() !== revision ||
    run("git", ["rev-parse", "v2.15.1^{commit}"]).trim() !== revision ||
    run("git", ["status", "--porcelain", "--untracked-files=no"]).trim())
  throw new Error("checkout must match the pristine pinned revision");
const manifestPaths = ["backend", "cli", "types"].map((p) => `${p}/go.mod`);
const manifests = manifestPaths.map((p) => ({ path: p, ...JSON.parse(run("go", ["mod", "edit", "-json", p])) }));
const hubGraph = objects(run("go", ["list", "-deps", "-json=ImportPath,Dir,GoFiles,CgoFiles,Module,Error,DepsErrors", "./backend/cmd/..."]));
const agentGraph = objects(run("go", ["list", "-deps", "-tags=exclude_frontend", "-json=ImportPath,Dir,GoFiles,CgoFiles,Module,Error,DepsErrors", "./backend/cmd/..." ], root, { CGO_ENABLED: "0" }));
const graph = [...new Map([...hubGraph, ...agentGraph].map((p) => [p.ImportPath, p])).values()];
if (graph.some((p) => p.Error || p.DepsErrors?.length)) throw new Error("incomplete binary graph");
const selected = objects(run("go", ["list", "-m", "-json", "all"]));
const effective = new Map(selected.map((m) => [m.Path, m]));
const required = new Map();
for (const m of manifests) for (const dep of m.Require ?? []) {
  const key = `${dep.Path}@${dep.Version}`;
  if (!required.has(key)) required.set(key, { name: dep.Path, version: dep.Version, declarations: [] });
  required.get(key).declarations.push({ manifest: m.path, relationship: dep.Indirect ? "transitive" : "direct" });
}
// Keep declared versions even when workspace MVS or replace selects a different version.
for (const m of selected.filter((m) => graph.some((p) => p.Module?.Path === m.Path))) {
  const version = m.Main ? revision : m.Replace?.Version ?? m.Version;
  const key = `${m.Path}@${version}`;
  if (!required.has(key)) required.set(key, { name: m.Path, version, declarations: [] });
}
for (const m of manifests) {
  const key = `${m.Module.Path}@${revision}`;
  if (!required.has(key)) required.set(key, { name: m.Module.Path, version: revision, declarations: [] });
}
const remote = [...required.values()].filter((m) => !m.name.startsWith("github.com/getarcaneapp/arcane/"));
const downloads = objects(run("go", ["mod", "download", "-json", ...remote.map((m) => `${m.name}@${m.version}`)]));
if (downloads.some((m) => m.Error)) throw new Error("module download failed");
const downloaded = new Map(downloads.map((m) => [`${m.Path}@${m.Version}`, m]));
// go-licenses emits unquoted comma-separated package, licence URL and SPDX fields.
const reports = read(resolve(report)).trim().split(/\r?\n/).map((line) => {
  const fields = line.split(",");
  if (fields.length !== 3) throw new Error("unexpected licence report row");
  return { name: fields[0], url: fields[1], license: fields[2] };
});
const sanitize = (text) => text.replace(/[\w.!#$%&'*+/=?^`{|}~-]+@[\w.-]+\.[A-Za-z]{2,}/g, "[address omitted]").trim();
function classify(text) {
  if (/SQLite Is Public Domain/.test(text)) return "LicenseRef-SQLite-Public-Domain";
  if (/Eclipse Public License/.test(text) && /Eclipse Distribution License/.test(text)) return "EPL-2.0 OR EDL-1.0";
  if (/Eclipse Distribution License|EDL-1\.0/.test(text)) return "EDL-1.0";
  if (/SPDX-License-Identifier: BSD-3-Clause AND MPL-2.0/.test(text)) return "BSD-3-Clause AND MPL-2.0";
  if (/covered by two different licenses: MIT and Apache/.test(text)) return "Apache-2.0 AND MIT";
  if (/Mozilla Public License.*2\.0|Mozilla Public\s*\n.*2\.0/.test(text)) return "MPL-2.0";
  if (/Apache License[\s,\n]+Version 2\.0|Apache License, Version 2\.0|SPDX-License-Identifier: Apache-2\.0/i.test(text)) return "Apache-2.0";
  if (/Permission is hereby granted, free of charge/i.test(text)) return "MIT";
  if (/permission to use, copy, modify, and distribute this software for any\s+purpose with or without fee/i.test(text)) return "ISC";
  if (/Permission to use, copy, modify, and distribute this software.*\n.*without fee.*\n.*without a written agreement/is.test(text) && /PostgreSQL|Postgres|California/.test(text)) return "PostgreSQL";
  if (/Redistribution and use in source and binary forms/i.test(text))
    return /Neither the name|names of its contributors|name of the author may|name of Google Inc/i.test(text) ? "BSD-3-Clause" : "BSD-2-Clause";
  if (/Boost Software License.*1\.0/.test(text)) return "BSL-1.0";
  if (/This is free and unencumbered software released into the public domain/.test(text)) return "Unlicense";
  if (/Eclipse Public License.*2\.0/.test(text)) return "EPL-2.0";
  return null;
}
function legalFiles(dir, recursive = false) {
  if (!existsSync(dir)) return [];
  const result = [];
  for (const item of readdirSync(dir, { withFileTypes: true })) {
    if (item.isDirectory() && recursive && ![".git", "testdata", "vendor"].includes(item.name)) result.push(...legalFiles(join(dir, item.name), true));
    if (item.isFile() && /^(licen[cs]e|copying|copyright|notice)([._-]|$)/i.test(item.name)) result.push(join(dir, item.name));
  }
  return result;
}
const copyrights = (text) => [...new Set(text.split(/\r?\n/).filter((s) => /copyright[^\r\n]*(?:[12][0-9]{3}|\([cC]\)|©)/i.test(s)).map(sanitize))];
function evidence(path, base, source) {
  const text = read(path);
  return { path: relative(base, path), sha256: hash(text), source,
    detectedLicense: classify(text), copyright: copyrights(text) };
}
const rows = [];
for (const item of [...required.values()].sort((a, b) => `${a.name}@${a.version}`.localeCompare(`${b.name}@${b.version}`, "en"))) {
  const local = item.name.startsWith("github.com/getarcaneapp/arcane/");
  const module = effective.get(item.name);
  const download = downloaded.get(`${item.name}@${item.version}`);
  const dir = local ? resolve(root, item.name.split("/").at(-2)) : download.Dir;
  const origin = local ? { URL: "https://github.com/getarcaneapp/arcane", Hash: revision, Subdir: relative(root, dir) } : download.Origin;
  const source = origin?.URL ?? `https://proxy.golang.org/${item.name.replace(/[A-Z]/g, (s) => "!" + s.toLowerCase())}/@v/${item.version}.zip`;
  const sourceRevision = origin?.Hash ?? item.version;
  const sourcePath = origin?.Subdir ?? "";
  const sourceRef = origin?.URL ? `${source}/tree/${sourceRevision}${sourcePath ? `/${sourcePath}` : ""}` : source;
  const licenceSource = local ? `${origin.URL}/tree/${revision}` : `https://proxy.golang.org/${item.name.replace(/[A-Z]/g, (s) => "!" + s.toLowerCase())}/@v/${item.version}.zip`;
  const rootFiles = local ? [join(root, "LICENSE")] : legalFiles(dir);
  const files = rootFiles.map((p) => evidence(p, local ? root : dir, licenceSource));
  let license = [...new Set(files.filter((f) => !/3RD-PARTY/.test(f.path)).flatMap((f) => (f.detectedLicense ?? "").split(" AND ")).filter(Boolean))].sort().join(" AND ");
  if (item.name === "modernc.org/libc") license = "BSD-3-Clause AND MIT";
  if (item.name.startsWith("go.getarcane.app/sys/")) {
    const url = `${origin.URL.replace("https://github.com/", "https://raw.githubusercontent.com/")}/${sourceRevision}/sys/LICENSE`;
    const response = await fetch(url, { signal: AbortSignal.timeout(30000) });
    if (!response.ok) throw new Error("cannot verify supplementary Kit licence");
    const content = await response.text();
    if (classify(content) !== "BSD-3-Clause") throw new Error("Kit licence changed");
    files.push({ path: "repository:sys/LICENSE", sha256: hash(content), source: url, detectedLicense: "BSD-3-Clause", copyright: copyrights(content) });
  }
  let resolution = "Module licence files; exact contents identified by SHA-256.";
  if (item.name === "github.com/mattn/go-localereader") {
    const p = join(dir, "README.md");
    if (!/## License\s+MIT/.test(read(p))) throw new Error("missing localereader licence evidence");
    files.push(evidence(p, dir, licenceSource)); license = "MIT";
    resolution = "Manual: README.md explicitly declares MIT; retain the declaration and author credit; no separate copyright statement is supplied.";
  }
  if (item.name === "github.com/eclipse/paho.golang") {
    license = "EPL-2.0 OR EDL-1.0";
    resolution = "Manual: LICENSE is EPL-2.0; edl-v10 is the alternative EDL-1.0 licence selected for redistribution.";
    for (const p of [join(dir, "edl-v10"), join(dir, "edl-v10.txt")].filter(existsSync)) files.push(evidence(p, dir, licenceSource));
  }
  const binaryPackages = graph.filter((p) => p.Module?.Path === item.name && (local ? item.version === revision : (p.Module.Replace?.Version ?? p.Module.Version) === item.version));
  const copyrightSources = [];
  if (!files.some((f) => f.copyright.length)) {
    const seen = new Set();
    const sourceFiles = binaryPackages.length
      ? binaryPackages.flatMap((p) => [...(p.GoFiles ?? []), ...(p.CgoFiles ?? [])].map((f) => join(p.Dir, f)))
      : readdirSync(dir).filter((f) => f.endsWith(".go")).map((f) => join(dir, f));
    for (const path of sourceFiles) {
      const text = read(path);
      const statements = copyrights(text.slice(0, 4096)).filter((s) => !seen.has(s));
      statements.forEach((s) => seen.add(s));
      if (statements.length) copyrightSources.push({ path: relative(dir, path), sha256: hash(text), copyright: statements });
    }
  }
  const matching = reports.filter((r) => binaryPackages.some((p) => p.ImportPath === r.name || p.ImportPath.startsWith(`${r.name}/`)));
  const detected = [...new Set(matching.map((r) => r.license).filter((s) => s && s !== "Unknown"))].sort();
  // Nested package licences can impose obligations absent from a module's root licence.
  const extra = legalFiles(dir, true).filter((p) => !rootFiles.includes(p));
  const nested = extra.map((p) => evidence(p, dir, licenceSource));
  const alternate = nested;
  if (!license) resolution = "Manual review required: no recognized root licence; blocked until resolved.";
  const disposition = license ? "accepted" : "blocked";
  const selectedVersion = module?.Main ? revision : module?.Replace?.Version ?? module?.Version ?? null;
  rows.push({ ...item, source, sourcePath, sourceRevision, correspondingSource: sourceRef,
    checksums: local ? { revision } : { module: download.Sum, goMod: download.GoModSum },
    selectedVersion, scope: binaryPackages.length ? ["runtime"] : ["build", "test"],
    relationship: item.declarations.some((d) => d.relationship === "direct") ? "direct" : "transitive",
    license: license || "NOASSERTION", selectedAlternative: item.name === "github.com/eclipse/paho.golang" ? "EDL-1.0" : null,
    copyright: [...new Set([...files, ...copyrightSources].flatMap((f) => f.copyright))], copyrightSources,
    copyrightEvidence: "Licence/NOTICE excerpts, with source headers where legal files lack attribution; retain full original notices when packaging. Empty means no statement in inspected texts, not public domain.", licenseFiles: files,
    nestedLicenseEvidence: alternate, reportedPackageLicenses: detected,
    modifications: "None; source assessment only, no dependency code imported.",
    redistributionObligations: license.includes("MPL-2.0") ? "Retain file notices and licence; make corresponding covered source available, including changes. Separate larger-work files keep their licences." : "Preserve applicable copyright, licence and NOTICE texts, including embedded third-party notices, in source and binary distributions; no endorsement.",
    resolution, disposition, sourceAvailable: true, binaryPackageCount: binaryPackages.length });
}
const binaryRows = rows.filter((r) => r.binaryPackageCount);
const unknown = rows.filter((r) => r.disposition === "blocked");
const lockfiles = ["go.work", "go.work.sum", ...manifestPaths, ...manifestPaths.map((p) => p.replace(".mod", ".sum"))]
  .filter((p) => existsSync(join(root, p))).map((path) => ({ path, sha256: hash(readFileSync(join(root, path))) }));
const goVersion = run("go", ["version"]).trim();
const standardLibrary = { name: "Go standard library", version: goVersion.split(" ")[2], license: "BSD-3-Clause",
  licenseFiles: [evidence(join(run("go", ["env", "GOROOT"]).trim(), "LICENSE"), run("go", ["env", "GOROOT"]).trim(), `https://go.googlesource.com/go/+/refs/tags/${goVersion.split(" ")[2]}/`)] };
const metadata = { schemaVersion: 1, ecosystem: "go", revision, date, lockfiles, standardLibrary,
  target: { packages: ["./backend/cmd/..."], modes: ["hub", "agent"], goos: "linux", goarch: "amd64", cgoEnabled: true, buildTags: [], agentBuildTags: ["exclude_frontend"], agentCgoEnabled: false },
  tool: { generator: "scripts/inventory-arcane.mjs", go: goVersion, licenseScanner: "github.com/google/go-licenses/v2 v2.0.1", reportSha256: hash(readFileSync(resolve(report))) },
  limitations: ["Pre-cut upstream graph; not a shipped Docklet Next binary.", "Non-binary declarations are conservatively classified build/test; this is not an exhaustive test graph.", "Platform-specific, optional build tags, web and container contents require separate release inventories."] };
mkdirSync(out, { recursive: true });
function writeInventory(name, data, packages) {
  // One JSON record per line keeps large generated datasets diffable without size-policy exemptions.
  const head = JSON.stringify(data, null, 2).slice(0, -2);
  writeFileSync(join(out, name), `${head},\n  "packages": [\n${packages.map((p) => "    " + JSON.stringify(p)).join(",\n")}\n  ]\n}\n`);
}
writeInventory("arcane-modules.json", { ...metadata, manifestCount: manifests.length, declaredEntries: manifests.reduce((n, m) => n + m.Require.length, 0), unsourceableModules: [], blockedModules: unknown.map((r) => `${r.name}@${r.version}`) }, rows);
writeInventory("arcane-binary.json", { ...metadata, moduleCount: binaryRows.length, packageCount: graph.filter((p) => p.Module).length }, binaryRows);
const packageRows = reports.map((r) => {
  const pkg = graph.find((p) => p.ImportPath === r.name) ?? graph.find((p) => p.ImportPath.startsWith(`${r.name}/`));
  const mod = binaryRows.find((m) => m.name === pkg?.Module?.Path);
  if (!pkg || !mod) throw new Error(`scanner package absent from graph: ${r.name}`);
  const manual = r.license === "Unknown";
  const corrected = r.name === mod.name && / AND | OR /.test(mod.license) && mod.license !== r.license ? mod.license : null;
  const legal = [...mod.licenseFiles, ...mod.nestedLicenseEvidence];
  const exact = legal.filter((e) => r.url.endsWith("/" + e.path));
  return { name: r.name, module: mod.name, version: mod.version, license: corrected ?? (manual ? mod.license : r.license),
    selectedAlternative: mod.selectedAlternative, method: manual ? "manual-module-evidence" : corrected ? "manual-correction" : "go-licenses",
    licenseFiles: exact.length ? exact : mod.licenseFiles,
    evidence: manual ? "arcane-binary.json#" + mod.name : r.url };
});
writeInventory("arcane-license-packages.json", { ...metadata, reportEntryCount: reports.length, manualEntryCount: reports.filter((p) => p.license === "Unknown").length, correctedEntryCount: packageRows.filter((p) => p.method === "manual-correction").length }, packageRows);
const packageGraph = graph.filter((p) => p.Module).map((p) => {
  const mod = binaryRows.find((r) => r.name === p.Module.Path);
  const candidates = packageRows.filter((r) => r.module === mod?.name && (p.ImportPath === r.name || p.ImportPath.startsWith(`${r.name}/`))).sort((a,b) => b.name.length - a.name.length);
  if (!mod || !candidates.length) throw new Error(`package without licence evidence: ${p.ImportPath}`);
  return { name: p.ImportPath, module: mod.name, version: mod.version, scope: "runtime", licenseGroup: candidates[0].name,
    targets: [hubGraph.some((x) => x.ImportPath === p.ImportPath) ? "hub" : null, agentGraph.some((x) => x.ImportPath === p.ImportPath) ? "agent" : null].filter(Boolean) };
}).sort((a,b) => a.name.localeCompare(b.name, "en"));
for (let i = 0; i < packageGraph.length; i += 500)
  writeInventory(`arcane-package-graph-${Math.floor(i / 500) + 1}.json`, { schemaVersion: 1, ecosystem: "go", revision, moduleInventory: "arcane-binary.json", licenseInventory: "arcane-license-packages.json" }, packageGraph.slice(i, i + 500));
writeFileSync(join(out, "arcane-modules.md"), `# Arcane module map\n\nGenerated by \`scripts/inventory-arcane.mjs\` from the pinned checkout.\nSee [adoption decision](../design/arcane-adoption.md) for scope and reproduction.\n\nEach row is an exact declared or selected module version. B/C/T identify the\nbackend/cli/types manifests; D is direct and I indirect. A dash is a selected\nbinary/workspace module without that exact require entry. Source links resolve\nto the effective source revision; local sibling requirements select the checkout.\n\n| Module | Declared / inventoried version | Required by | Selected version | Licence | Source |\n| --- | --- | --- | --- | --- | --- |\n${rows.map((r) => `| ${r.name} | ${r.version} | ${r.declarations.map((d) => ({ backend: "B", cli: "C", types: "T" })[d.manifest.split("/")[0]] + (d.relationship === "direct" ? ":D" : ":I")).join(", ") || "—"} | ${r.selectedVersion ?? "—"} | ${r.license} | [source](${r.correspondingSource}) |`).join("\n")}\n`);
console.log(JSON.stringify({ modules: rows.length, declaredEntries: manifests.reduce((n, m) => n + m.Require.length, 0), binaryModules: binaryRows.length, binaryPackages: graph.filter((p) => p.Module).length, scannerEntries: reports.length, blocked: unknown.map((r) => r.name) }));
