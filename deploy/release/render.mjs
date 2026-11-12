// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification; M22 #446: CA source asset and rendered image-default pin guard.

import { createHash } from "node:crypto";
import { createRequire } from "node:module";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { basename, join } from "node:path";
import { isMain, options, releaseReference, releaseRepository, sha256, version } from "./common.mjs";
const { parse, stringify } = createRequire(new URL("../../web/package.json", import.meta.url))("yaml");
// The CA bundle's source archive and every licence its contents need travel with each release (MPL-2.0 §3.2(a)).
export function caSourceFiles(assessed = JSON.parse(readFileSync("deploy/release/ca-bundle.json", "utf8"))) {
  const { file, sha512, licenses } = assessed.source ?? {};
  if (typeof file !== "string" || !existsSync(file)) throw new Error("CA source archive missing");
  const archive = readFileSync(file);
  if (createHash("sha512").update(archive).digest("hex") !== sha512) throw new Error("CA source archive SHA-512 mismatch");
  const files = { [basename(file)]: archive };
  for (const path of [...assessed.notices, ...licenses]) files[basename(path)] = readFileSync(path);
  return files;
}
// Compose interpolations at every "${" start, nested ones included: ${NAME}, ${NAME-x}, ${NAME:-x}, ${NAME?x},
// ${NAME:?x}, ${NAME+x}, ${NAME:+x}. "$$" escapes are not special-cased, so the scan is stricter than Compose.
export function interpolations(value) {
  const found = [];
  for (let start = value.indexOf("${"); start >= 0; start = value.indexOf("${", start + 2)) {
    const name = /^[A-Za-z_][A-Za-z0-9_]*/.exec(value.slice(start + 2))?.[0];
    if (!name) throw new Error("invalid Compose interpolation");
    const from = start + 2 + name.length + /^(?::?[-?+])?/.exec(value.slice(start + 2 + name.length))[0].length;
    let depth = 1, end = from;
    for (; end < value.length && depth; end++) {
      if (value.startsWith("${", end)) { depth++; end++; } else if (value[end] === "}") depth--;
    }
    if (depth) throw new Error("unbalanced Compose interpolation");
    const operator = value.slice(start + 2 + name.length, from);
    if (!operator && end - 1 !== from) throw new Error("unsupported Compose interpolation");
    found.push({ name, operator, argument: value.slice(from, end - 1), whole: start === 0 && end === value.length });
  }
  return found;
}
const isDefault = (item) => item.operator === "-" || item.operator === ":-";
// Every image default in the rendered Compose (service images and any *_IMAGE fallback at any depth) must be a package pin.
export function checkImageDefaults(compose, manifest) {
  const pins = new Set(["hub", "agent"].map((role) => manifest.images[role].reference));
  const defaults = [];
  for (const [name, service] of Object.entries(compose.services ?? {})) {
    const image = service?.image;
    if (typeof image !== "string") throw new Error(`service ${name} has no image default`);
    const items = interpolations(image);
    if (!items.length) { defaults.push(image); continue; }
    const [item] = items;
    if (items.length !== 1 || !item.whole || !isDefault(item) || item.argument.includes("$"))
      throw new Error(`service ${name} image has no usable default`);
    defaults.push(item.argument);
  }
  const visit = (value) => {
    if (typeof value === "string") {
      for (const item of interpolations(value)) if (item.name.endsWith("_IMAGE") && isDefault(item)) defaults.push(item.argument);
    }
    else if (value && typeof value === "object") for (const item of Object.values(value)) visit(item);
  };
  visit(compose);
  for (const value of defaults) if (!pins.has(value)) throw new Error("image default is not a pinned package reference");
  return defaults;
}
export function render(manifest, out, { template = readFileSync("deploy/hub/compose.yaml", "utf8") } = {}) {
  const v = version(manifest.version);
  releaseRepository(manifest.releaseRepository);
  for (const role of ["hub", "agent"]) {
    const ref = releaseReference(manifest.images[role].reference);
    if (ref.tag !== v || ref.digest !== manifest.images[role].digest || `${ref.registry}/${ref.path}` !== `${manifest.releaseRepository}-${role}`) throw new Error("manifest image identity mismatch");
  }
  const compose = parse(template);
  delete compose.services.hub.build;
  compose.services.hub.image = `\${HUB_IMAGE:-${manifest.images.hub.reference}}`;
  compose.services.hub.environment.DOCKLET_AGENT_IMAGE = `\${DOCKLET_AGENT_IMAGE:-${manifest.images.agent.reference}}`;
  const header = template.match(/^(?:#[^\n]*\n)+/)?.[0] ?? "";
  const yaml = header + "# Generated release package: source build removed; image defaults digest-pinned.\n" + stringify(compose, { lineWidth: 0 });
  checkImageDefaults(parse(yaml), manifest);
  const caFiles = caSourceFiles();
  const modes = { "-": ":-", ":-": ":-", "?": ":?", ":?": ":?" };
  const variables = new Map(interpolations(yaml).map(({ name, operator, argument }) => [name, { mode: modes[operator], fallback: modes[operator] ? argument : "" }]));
  const comments = { HUB_IMAGE: "Optional hub image override; the default is pinned below.", DOCKLET_AGENT_IMAGE: "Optional agent image override; blank uses the pinned package default.", ENCRYPTION_KEY: "Required: generate a private 32-byte hexadecimal key. Keep it with your backups.", APP_URL: "Dedicated public domain root (sub-path hosting is unsupported).", PUID: "Numeric runtime user; startup prepares the data volume then drops privileges.", PGID: "Numeric runtime group.", TZ: "IANA time zone for schedules and display." };
  const env = ["# Docklet Next " + v, "# Copy to a private .env and pass --env-file consistently."];
  for (const [key, { mode, fallback }] of variables) {
    env.push(`# ${comments[key] ?? "Optional Compose setting; see the full repository .env.example reference."}`);
    env.push(key === "HUB_IMAGE" ? `# HUB_IMAGE=${fallback}` : `${key}=${mode === ":?" || key === "DOCKLET_AGENT_IMAGE" ? "" : fallback}`);
  }
  mkdirSync(out, { recursive: true });
  const files = { [`docklet-next-${v}-compose.yaml`]: yaml, [`docklet-next-${v}.env.example`]: env.join("\n") + "\n", "release-manifest.json": JSON.stringify(manifest, null, 2) + "\n" };
  for (const [name, path] of [["LICENSE", "LICENSE"], ["NOTICE", "NOTICE"], ["BSD-3-Clause-Arcane.txt", "LICENSES/BSD-3-Clause-Arcane.txt"]]) files[name] = readFileSync(path);
  Object.assign(files, caFiles);
  for (const [path, content] of Object.entries(files)) writeFileSync(join(out, path), content);
  writeFileSync(join(out, "SHA256SUMS"), Object.entries(files).map(([path, content]) => `${sha256(content).slice(7)}  ${path}\n`).join(""));
  return files;
}
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--manifest", "--out"]);
  if (!args["--manifest"] || !args["--out"]) throw new Error("expected --manifest and --out");
  render(JSON.parse(readFileSync(args["--manifest"])), args["--out"]);
}
