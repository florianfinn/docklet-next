// Generate adoption evidence from a pristine Arcane checkout; no product code is copied.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { hash, classify, legalFiles, inspectLegal, inspectCompiled, assertLicense } from "./arcane-license-policy.mjs";
import { readFileSync, writeFileSync, mkdirSync, existsSync, readdirSync } from "node:fs";
import { resolve, relative, dirname, join } from "node:path";

const [checkout, report, output] = process.argv.slice(2);
if (!checkout || !report || !output) throw new Error("usage: inventory-arcane.mjs CHECKOUT LICENSE_REPORT OUTPUT");
const root = resolve(checkout), out = resolve(output);
const revision = "095cb778daba1034e2a39961199e712a75d4ba29";
const date = "2026-10-13";
const run = (cmd, args, cwd = root, overrides = {}) => execFileSync(cmd, args, {
  cwd, encoding: "utf8", maxBuffer: 100 * 1024 * 1024,
  env: { ...process.env, GOOS: "linux", GOARCH: "amd64", CGO_ENABLED: "1", GOFLAGS: "", GOPROXY: "off", GOSUMDB: "off", GOTOOLCHAIN: "local", ...overrides },
});
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
const hubGraph = objects(run("go", ["list", "-deps", "-json=ImportPath,Dir,GoFiles,CgoFiles,CFiles,HFiles,SFiles,Imports,Module,Error,DepsErrors", "./backend/cmd/..."]));
const agentGraph = objects(run("go", ["list", "-deps", "-tags=exclude_frontend", "-json=ImportPath,Dir,GoFiles,CgoFiles,CFiles,HFiles,SFiles,Imports,Module,Error,DepsErrors", "./backend/cmd/..." ], root, { CGO_ENABLED: "0" }));
const combined = new Map();
for (const p of [...hubGraph, ...agentGraph]) {
  const previous = combined.get(p.ImportPath);
  const merged = { ...p };
  for (const key of ["GoFiles", "CgoFiles", "CFiles", "HFiles", "SFiles", "Imports"])
    merged[key] = [...new Set([...(previous?.[key] ?? []), ...(p[key] ?? [])])];
  combined.set(p.ImportPath, merged);
}
const graph = [...combined.values()];
// Prove the SQLite-only route in each target, rather than assuming it from names.
for (const target of [hubGraph, agentGraph]) {
  const packages = new Map(target.map((p) => [p.ImportPath, p]));
  const pending = target.filter((p) => p.ImportPath.startsWith("github.com/getarcaneapp/arcane/backend/v2/cmd")).map((p) => p.ImportPath);
  const seen = new Set();
  while (pending.length) {
    const name = pending.pop(), p = packages.get(name);
    if (!p || seen.has(name) || p.Module?.Path === "modernc.org/sqlite") continue;
    seen.add(name);
    if (p.Module?.Path === "modernc.org/libc") throw new Error("libc is reachable without SQLite");
    pending.push(...(p.Imports ?? []));
  }
}
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
// Scanner package/license pairs are committed; network-resolved URLs are not inputs.
const reports = JSON.parse(read(resolve(report)));
if (reports.some((r) => !r.name || !r.license || Object.keys(r).length !== 2)) throw new Error("invalid scanner input");
const sanitize = (text) => text.replace(/[\w.!#$%&'*+/=?^`{|}~-]+@[\w.-]+\.[A-Za-z]{2,}/g, "[address omitted]").trim();
const copyrights = (text) => [...new Set(text.split(/\r?\n/).filter((s) => /copyright[^\r\n]*(?:[12][0-9]{3}|\([cC]\)|©)/i.test(s)).map(sanitize))];
function evidence(path, base, source, item = { name: "Go standard library", version: "go1.27.2" }) {
  const text = read(path);
  return { path: relative(base, path), sha256: hash(text), source,
    ...inspectLegal(item.name, item.version, relative(base, path), text), copyright: copyrights(text) };
}
function moduleTree(dir) {
  const object = (kind, bytes) => createHash("sha1").update(`${kind} ${bytes.length}\0`).update(bytes).digest();
  const entries = readdirSync(dir, { withFileTypes: true }).sort((a, b) => Buffer.compare(Buffer.from(a.name + (a.isDirectory() ? "/" : "")), Buffer.from(b.name + (b.isDirectory() ? "/" : ""))));
  return object("tree", Buffer.concat(entries.map((e) => {
    if (!e.isFile() && !e.isDirectory()) throw new Error("unsupported module entry");
    return Buffer.concat([Buffer.from(`${e.isDirectory() ? "40000" : "100644"} ${e.name}\0`), e.isDirectory() ? moduleTree(join(dir, e.name)) : object("blob", readFileSync(join(dir, e.name)))]);
  })));
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
  const files = rootFiles.map((p) => evidence(p, local ? root : dir, licenceSource, item));
  let license;
  if (item.name.startsWith("go.getarcane.app/sys/")) {
    const url = `${origin.URL.replace("https://github.com/", "https://raw.githubusercontent.com/")}/${sourceRevision}/sys/LICENSE`;
    // The supplementary sys/LICENSE equals the pinned Arcane root BSD text.
    const content = read(join(root, "LICENSE"));
    if (hash(content) !== "9618cc2d4716242ffc576d177e3c325da67c27cd973d449b1bdb31ecefde57c5") throw new Error("supplementary licence changed");
    if (classify(content) !== "BSD-3-Clause") throw new Error("Kit licence changed");
    files.push({ path: "repository:sys/LICENSE", sha256: hash(content), source: url, detectedLicense: "BSD-3-Clause", copyright: copyrights(content) });
  }
  let resolution = "Module licence files; exact contents identified by SHA-256.";
  if (item.name === "github.com/mattn/go-localereader") {
    const p = join(dir, "README.md");
    if (!/## License\s+MIT/.test(read(p))) throw new Error("missing localereader licence evidence");
    files.push(evidence(p, dir, licenceSource, item)); license = "MIT";
    resolution = "Manual: README.md explicitly declares MIT; retain the declaration and author credit; no separate copyright statement is supplied.";
  }
  if (item.name === "github.com/eclipse/paho.golang") {
    license = "EPL-2.0 OR BSD-3-Clause";
    resolution = "Manual: elect BSD-3-Clause (Eclipse Distribution License 1.0 / EDL-1.0) from EPL-2.0 OR BSD-3-Clause; preserve edl-v10 and contributor source notices.";
    for (const p of [join(dir, "edl-v10"), join(dir, "edl-v10.txt")].filter(existsSync)) files.push(evidence(p, dir, licenceSource, item));
  }
  const binaryPackages = graph.filter((p) => p.Module?.Path === item.name && (local ? item.version === revision : (p.Module.Replace?.Version ?? p.Module.Version) === item.version));
  const copyrightSources = [];
  if (!files.some((f) => f.copyright.length) || item.name === "github.com/eclipse/paho.golang") {
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
  const nested = extra.map((p) => evidence(p, dir, licenceSource, item));
  const codeLicenses = [...files, ...nested].filter((f) => f.review?.scope !== "non-code").flatMap((f) => (f.detectedLicense ?? "").split(" AND "));
  license = item.name === "github.com/eclipse/paho.golang" ? "EPL-2.0 OR BSD-3-Clause" : [...new Set(codeLicenses.filter(Boolean))].sort().join(" AND ");
  try { assertLicense(license, item.name === "github.com/eclipse/paho.golang" ? "BSD-3-Clause" : null); } catch { throw new Error(`unapproved module licence: ${item.name}: ${license}`); }
  const compiledLicenseNotices = [];
  const sourceFiles = [...new Set(binaryPackages.flatMap((p) => ["GoFiles", "CgoFiles", "CFiles", "HFiles", "SFiles"].flatMap((key) => (p[key] ?? []).map((f) => join(p.Dir, f)))))];
  for (const path of sourceFiles) {
    const content = read(path);
    const result = inspectCompiled(item.name, item.version, relative(dir, path), content);
    if (!result) continue;
    const { review, comments } = result;
    compiledLicenseNotices.push({ path: relative(dir, path), sha256: hash(content), license: review.license,
      reason: review.reason, source: licenceSource, targets: ["hub", "agent"], arrivesOnlyVia: "modernc.org/sqlite", copyright: copyrights(comments) });
  }
  if (item.name === "modernc.org/libc") resolution = "Root BSD-3-Clause plus MIT in LICENSE-3RD-PARTY.md, now detected without a hard-coded module override. Two compiled glibc header-derived files retain LGPL-2.1-or-later notices under the reviewed LGPL-2.1 section 5 assessment; see compiledLicenseNotices. SQLite-only, to be removed in #7.";
  if (item.name === "github.com/cyphar/filepath-securejoin") resolution += " Only BSD-3-Clause root/internal/consts files are compiled; MPL-2.0 pathrs-lite is outside both measured graphs. Module expression includes all source licences conservatively.";
  const disposition = "accepted";
  const selectedVersion = module?.Main ? revision : module?.Replace?.Version ?? module?.Version ?? null;
  rows.push({ ...item, source, sourcePath, sourceRevision, correspondingSource: sourceRef,
    checksums: local ? { revision } : { module: download.Sum, goMod: download.GoModSum },
    selectedVersion, scope: binaryPackages.length ? ["runtime"] : item.declarations.some((d) => d.manifest === "cli/go.mod") ? ["runtime (cli)"] : ["build", "test"],
    scopeEvidence: binaryPackages.length ? "Measured hub/agent graph." : item.declarations.some((d) => d.manifest === "cli/go.mod") ? "CLI declaration; conservatively include runtime obligations, standalone CLI graph not measured." : "Declaration outside measured command graphs.",
    ...(local ? { sourceTree: { kind: "upstream-git-subtree", sha1: run("git", ["rev-parse", `${revision}:${relative(root, dir)}`]).trim() } } : item.name.startsWith("go.getarcane.app/") ? { sourceTree: { kind: "module-zip-git-tree", sha1: moduleTree(dir).toString("hex"), fileMode: "100644", excludes: "Nested modules and files excluded by the Go module ZIP format; not the upstream repository subtree." } } : {}),
    relationship: item.declarations.some((d) => d.relationship === "direct") ? "direct" : "transitive",
    license: license || "NOASSERTION", selectedAlternative: item.name === "github.com/eclipse/paho.golang" ? "BSD-3-Clause" : null,
    copyright: [...new Set([...files, ...copyrightSources].flatMap((f) => f.copyright))], copyrightSources,
    copyrightEvidence: "Licence/NOTICE excerpts, with source headers where legal files lack attribution; retain full original notices when packaging. Empty means no statement in inspected texts, not public domain.", licenseFiles: files,
    nestedLicenseEvidence: nested, compiledLicenseNotices, reportedPackageLicenses: detected,
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
  tool: { generator: "scripts/inventory-arcane.mjs", go: goVersion, licenseScanner: "github.com/google/go-licenses/v2 v2.0.1", reportSha256: hash(readFileSync(resolve(report))), reviewPolicySha256: hash(readFileSync(new URL("./arcane-license-reviews.json", import.meta.url))) },
  limitations: ["Pre-cut upstream graph; not a shipped Docklet Next binary.", "CLI-only declarations include runtime (cli) obligations; standalone CLI and exhaustive test/build graphs are not measured.", "Platform-specific, optional build tags, web and container contents require separate release inventories."] };
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
  const groupPath = r.name.slice(mod.name.length).replace(/^\//, "");
  const exact = legal.filter((e) => dirname(e.path) === (groupPath || "."));
  const chosen = exact.length ? exact : mod.licenseFiles;
  const groupLicense = corrected ?? (manual ? mod.license : r.license);
  assertLicense(groupLicense, mod.selectedAlternative);
  return { name: r.name, module: mod.name, version: mod.version, license: groupLicense,
    selectedAlternative: mod.selectedAlternative, method: manual ? "manual-module-evidence" : corrected ? "manual-correction" : "go-licenses",
    licenseFiles: chosen,
    evidence: chosen.map((e) => ({ source: e.source, path: e.path, sha256: e.sha256 })) };
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
