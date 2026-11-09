// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { createRequire } from "node:module";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { isMain, options, releaseReference, releaseRepository, sha256, version } from "./common.mjs";
const { parse, stringify } = createRequire(new URL("../../web/package.json", import.meta.url))("yaml");
export function render(manifest, out) {
  const v = version(manifest.version);
  releaseRepository(manifest.releaseRepository);
  for (const role of ["hub", "agent"]) {
    const ref = releaseReference(manifest.images[role].reference);
    if (ref.tag !== v || ref.digest !== manifest.images[role].digest || `${ref.registry}/${ref.path}` !== `${manifest.releaseRepository}-${role}`) throw new Error("manifest image identity mismatch");
  }
  const template = readFileSync("deploy/hub/compose.yaml", "utf8");
  const compose = parse(template);
  delete compose.services.hub.build;
  compose.services.hub.image = `\${HUB_IMAGE:-${manifest.images.hub.reference}}`;
  compose.services.hub.environment.DOCKLET_AGENT_IMAGE = `\${DOCKLET_AGENT_IMAGE:-${manifest.images.agent.reference}}`;
  const header = template.match(/^(?:#[^\n]*\n)+/)?.[0] ?? "";
  const yaml = header + "# Generated release package: source build removed; image defaults digest-pinned.\n" + stringify(compose, { lineWidth: 0 });
  const variables = new Map([...yaml.matchAll(/\$\{([A-Z_][A-Z0-9_]*)(?:(:-|:\?)([^}]*))?\}/g)].map(([, key, mode, fallback]) => [key, { mode, fallback: fallback ?? "" }]));
  const comments = { HUB_IMAGE: "Optional hub image override; the default is pinned below.", DOCKLET_AGENT_IMAGE: "Optional agent image override; blank uses the pinned package default.", ENCRYPTION_KEY: "Required: generate a private 32-byte hexadecimal key. Keep it with your backups.", APP_URL: "Dedicated public domain root (sub-path hosting is unsupported).", PUID: "Numeric runtime user; startup prepares the data volume then drops privileges.", PGID: "Numeric runtime group.", TZ: "IANA time zone for schedules and display." };
  const env = ["# Docklet Next " + v, "# Copy to a private .env and pass --env-file consistently."];
  for (const [key, { mode, fallback }] of variables) {
    env.push(`# ${comments[key] ?? "Optional Compose setting; see the full repository .env.example reference."}`);
    env.push(key === "HUB_IMAGE" ? `# HUB_IMAGE=${fallback}` : `${key}=${mode === ":?" || key === "DOCKLET_AGENT_IMAGE" ? "" : fallback}`);
  }
  mkdirSync(out, { recursive: true });
  const files = { [`docklet-next-${v}-compose.yaml`]: yaml, [`docklet-next-${v}.env.example`]: env.join("\n") + "\n", "release-manifest.json": JSON.stringify(manifest, null, 2) + "\n" };
  for (const [name, path] of [["LICENSE", "LICENSE"], ["NOTICE", "NOTICE"], ["BSD-3-Clause-Arcane.txt", "LICENSES/BSD-3-Clause-Arcane.txt"]]) files[name] = readFileSync(path);
  for (const [path, content] of Object.entries(files)) writeFileSync(join(out, path), content);
  writeFileSync(join(out, "SHA256SUMS"), Object.entries(files).map(([path, content]) => `${sha256(content).slice(7)}  ${path}\n`).join(""));
  return files;
}
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--manifest", "--out"]);
  if (!args["--manifest"] || !args["--out"]) throw new Error("expected --manifest and --out");
  render(JSON.parse(readFileSync(args["--manifest"])), args["--out"]);
}
