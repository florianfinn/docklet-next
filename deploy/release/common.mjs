// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import { isIP } from "node:net";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync, statSync } from "node:fs";
import { resolve, join } from "node:path";
import { pathToFileURL } from "node:url";

export const repository = "ghcr.io/florianfinn/docklet-next";
export const sha256 = bytes => `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
export const isMain = url => process.argv[1] && url === pathToFileURL(resolve(process.argv[1])).href;
export function run(program, args, options = {}) {
  const result = spawnSync(program, args, { encoding: "utf8", ...options });
  if (result.status !== 0) throw new Error(`${program} failed (exit ${result.status ?? "signal"})`);
  return result.stdout?.trim();
}
export function version(value) {
  const match = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/.exec(value ?? "");
  if (!match || match[4]?.split(".").some(id => /^0[0-9]+$/.test(id))) throw new Error("expected release SemVer X.Y.Z[-pre]");
  return value;
}
// Parse the registry, repository, tag and digest separately; defaults require all four.
export function releaseReference(value) {
  if (typeof value !== "string") throw new Error("invalid release reference");
  const parts = /^(?<name>[^@]+)@(?<digest>sha256:[a-f0-9]{64})$/.exec(value)?.groups;
  if (!parts) throw new Error("release reference requires sha256 digest");
  const tagged = /^(?<registry>[^/]+)\/(?<path>.+):(?<tag>[^:]+)$/.exec(parts.name)?.groups;
  if (!tagged) throw new Error("release reference requires registry/path and version tag");
  const host = /^(\[[a-fA-F0-9:]+\]|[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)*)(?::([0-9]{1,5}))?$/.exec(tagged.registry);
  const bracketed = host?.[1].startsWith("[");
  if (!host || (bracketed && isIP(host[1].slice(1, -1)) !== 6) ||
      (!bracketed && !host[1].includes(".") && host[1] !== "localhost" && !host[2]) ||
      (host[2] && (+host[2] < 1 || +host[2] > 65535)) || parts.name.length > 255 ||
      !tagged.path.split("/").every(p => /^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*$/.test(p)))
    throw new Error("invalid distribution reference name");
  version(tagged.tag);
  return { ...tagged, digest: parts.digest };
}
export function releaseRepository(value = repository) {
  releaseReference(`${value}-agent:1.0.0@sha256:${"0".repeat(64)}`);
  return value;
}
export function options(args, names) {
  const result = {};
  for (let i = 0; i < args.length; i += 2) {
    if (!names.includes(args[i]) || !args[i + 1] || args[i + 1].startsWith("--") || result[args[i]]) throw new Error("invalid release arguments");
    result[args[i]] = args[i + 1];
  }
  return result;
}
const indexTypes = new Set(["application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"]);
const manifestTypes = new Set(["application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json"]);
export function layout(directory) {
  const root = resolve(directory);
  const objects = new Map(), manifests = [], blobs = [];
  let total = 0;
  function visit(bytes, descriptor, file) {
    const digest = sha256(bytes);
    if (descriptor && (digest !== descriptor.digest || bytes.length !== descriptor.size)) throw new Error("OCI descriptor digest/size mismatch");
    if (objects.has(digest)) return;
    if (objects.size >= 4096) throw new Error("OCI object limit exceeded");
    const mediaType = descriptor?.mediaType ?? JSON.parse(bytes).mediaType ?? "application/vnd.oci.image.index.v1+json";
    const object = { digest, mediaType, bytes, file, size: bytes.length };
    objects.set(digest, object);
    if (!indexTypes.has(mediaType) && !manifestTypes.has(mediaType)) throw new Error("unsupported OCI manifest type");
    const document = JSON.parse(bytes);
    if (document.schemaVersion !== 2) throw new Error("unsupported OCI schema");
    for (const child of document.manifests ?? []) {
      if (child.platform && (child.platform.os !== "linux" || child.platform.architecture !== "amd64")) throw new Error("release platform must be linux/amd64");
      const path = blobPath(child);
      if (child.size > 16 * 1024 * 1024) throw new Error("OCI manifest too large");
      visit(readFileSync(path), child, path);
    }
    for (const child of [document.config, ...(document.layers ?? [])].filter(Boolean)) {
      const path = blobPath(child);
      if (objects.has(child.digest)) continue;
      const size = statSync(path).size;
      total += size;
      if (size !== child.size || size > 1024 ** 3 || total > 8 * 1024 ** 3 || objects.size >= 4096) throw new Error("OCI blob bounds exceeded");
      // Hash one bounded blob at a time; upload streams from the verified file.
      if (sha256(readFileSync(path)) !== child.digest) throw new Error("OCI blob digest mismatch");
      const blob = { ...child, file: path };
      objects.set(child.digest, blob); blobs.push(blob);
    }
    manifests.push(object);
  }
  function blobPath(descriptor) {
    if (!/^sha256:[a-f0-9]{64}$/.test(descriptor.digest) || !Number.isSafeInteger(descriptor.size) || descriptor.size < 0) throw new Error("invalid OCI descriptor");
    return join(root, "blobs", "sha256", descriptor.digest.slice(7));
  }
  if (JSON.parse(readFileSync(join(root, "oci-layout"))).imageLayoutVersion !== "1.0.0") throw new Error("invalid OCI layout");
  const bytes = readFileSync(join(root, "index.json"));
  if (bytes.length > 16 * 1024 * 1024) throw new Error("OCI index too large");
  visit(bytes);
  if (!manifests.some(m => manifestTypes.has(m.mediaType))) throw new Error("OCI layout has no platform manifest");
  return { digest: sha256(bytes), bytes, manifests, blobs, mediaType: JSON.parse(bytes).mediaType ?? "application/vnd.oci.image.index.v1+json" };
}
export function buildStamp({ version: releaseVersion, revision, epoch, agentImage = "", releaseRepository: repo = repository }) {
  version(releaseVersion); releaseRepository(repo);
  if (!/^[a-f0-9]{40}$/.test(revision) || !/^\d+$/.test(String(epoch)) || !Number.isFinite(new Date(Number(epoch) * 1000).valueOf())) throw new Error("invalid source identity");
  if (agentImage) releaseReference(agentImage);
  const values = { Version: releaseVersion, Revision: revision, BuildTime: new Date(Number(epoch) * 1000).toISOString().replace(".000Z", "Z"), DefaultAgentImage: agentImage, ReleaseRepository: repo };
  return Object.entries(values).map(([key, value]) => `-X github.com/florianfinn/docklet-next/backend/internal/config.${key}=${value}`).join(" ");
}
