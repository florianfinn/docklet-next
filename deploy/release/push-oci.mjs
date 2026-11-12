// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification; M22 #446: a redirected blob HEAD counts as present and is never followed.

import { lookup } from "node:dns/promises";
import { createReadStream } from "node:fs";
import { isIP } from "node:net";
import { isMain, layout, options, releaseReference, sha256 } from "./common.mjs";

const BLOB_REDIRECTS = new Set([301, 302, 303, 307, 308]);
function bearer(value) {
  if (typeof value !== "string" || !/^[A-Za-z0-9._~+\/-]+=*$/.test(value)) throw new Error("invalid registry bearer credential");
  return `Bearer ${value}`;
}
function privateHost(host) {
  if (host === "localhost" || host === "[::1]") return true;
  if (isIP(host) === 4) {
    const [a, b] = host.split(".").map(Number);
    return a === 127 || a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254);
  }
  return /^\[(?:f[cd][a-f0-9]{2}:|fe[89ab][a-f0-9]:)/i.test(host);
}
export function registryURL(registry, scheme = "https", resolvedAddresses = []) {
  const url = new URL(`${scheme}://${registry}`);
  if (url.username || url.password || url.pathname !== "/" || !["http:", "https:"].includes(url.protocol) || (url.protocol === "http:" && !privateHost(url.hostname) && !(resolvedAddresses.length && resolvedAddresses.every(privateHost)))) throw new Error("registry requires HTTPS or a private/loopback HTTP address");
  return url.origin;
}
async function boundedBody(response, maximum) {
  if (Number(response.headers.get("content-length")) > maximum) { await response.body?.cancel(); throw new Error("registry response too large"); }
  const chunks = []; let size = 0;
  for await (const chunk of response.body) {
    size += chunk.length;
    if (size > maximum) throw new Error("registry response too large");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}
async function checkedOrigin(registry, scheme) {
  const url = new URL(`${scheme}://${registry}`);
  const addresses = scheme === "http" && !privateHost(url.hostname) ?
    (await lookup(url.hostname, { all: true })).map(result => result.address.includes(":") ? `[${result.address}]` : result.address) : [];
  return registryURL(registry, scheme, addresses);
}
export async function pushOCI({ directory, reference, scheme = "https", env = process.env }) {
  const ref = releaseReference(reference);
  const source = layout(directory);
  if (source.digest !== ref.digest) throw new Error("reference differs from OCI index digest");
  const origin = await checkedOrigin(ref.registry, scheme);
  const base = `${origin}/v2/${ref.path}`;
  let authorization = env.REGISTRY_BEARER_TOKEN ? bearer(env.REGISTRY_BEARER_TOKEN) :
    env.REGISTRY_USERNAME && env.REGISTRY_PASSWORD ? `Basic ${Buffer.from(`${env.REGISTRY_USERNAME}:${env.REGISTRY_PASSWORD}`).toString("base64")}` : "";
  async function request(url, init = {}, retry = true) {
    const target = new URL(url, origin);
    if (target.origin !== origin) throw new Error("registry upload location changed origin");
    const response = await fetch(target, { ...init, headers: { ...init.headers, ...(authorization ? { Authorization: authorization } : {}) },
      redirect: init.redirect ?? "error", signal: AbortSignal.timeout(120000), ...(init.body ? { duplex: "half" } : {}) });
    if (response.status === 401 && retry) {
      await response.body?.cancel();
      const challenge = response.headers.get("www-authenticate") ?? "";
      const realm = /realm="([^"]+)"/.exec(challenge)?.[1];
      if (!/^Bearer /i.test(challenge) || !realm) throw new Error("registry authentication failed");
      const exchangeURL = new URL(realm);
      await checkedOrigin(exchangeURL.host, exchangeURL.protocol.slice(0, -1));
      if (exchangeURL.username || exchangeURL.password) throw new Error("invalid token service URL");
      for (const name of ["service", "scope"]) {
        const value = new RegExp(`${name}="([^"]+)"`).exec(challenge)?.[1];
        if (value) exchangeURL.searchParams.set(name, value);
      }
      const basic = env.REGISTRY_USERNAME && env.REGISTRY_PASSWORD ? `Basic ${Buffer.from(`${env.REGISTRY_USERNAME}:${env.REGISTRY_PASSWORD}`).toString("base64")}` : "";
      const exchangeResponse = await fetch(exchangeURL, { headers: basic ? { Authorization: basic } : {}, redirect: "error", signal: AbortSignal.timeout(30000) });
      if (!exchangeResponse.ok) throw new Error("registry token exchange failed");
      const body = await boundedBody(exchangeResponse, 64 * 1024);
      let grant;
      try { grant = JSON.parse(body.toString("utf8")); } catch { throw new Error("invalid registry authentication response"); }
      if (!grant || typeof grant !== "object") throw new Error("invalid registry authentication response");
      if (typeof (grant.token ?? grant.access_token) !== "string") throw new Error("registry token missing");
      authorization = bearer(grant.token ?? grant.access_token);
      // Streaming uploads are never replayed. Authenticate via HEAD before opening a stream.
      if (init.body && typeof init.body !== "string" && !Buffer.isBuffer(init.body)) throw new Error("registry authentication expired during upload");
      return request(url, init, false);
    }
    return response;
  }
  async function requireStatus(response, statuses) {
    await response.body?.cancel();
    if (!statuses.includes(response.status)) throw new Error(`registry operation failed (HTTP ${response.status})`);
  }
  const accept = source.manifests.map(m => m.mediaType).filter((v, i, a) => a.indexOf(v) === i).join(", ");
  async function checkManifest(key, expected, absentAllowed = false) {
    const response = await request(`${base}/manifests/${key}`, { headers: { Accept: accept } });
    if (absentAllowed && response.status === 404) { await response.body?.cancel(); return false; }
    if (!response.ok) { await requireStatus(response, [200]); }
    const bytes = await boundedBody(response, 16 * 1024 * 1024);
    const header = response.headers.get("docker-content-digest");
    if (sha256(bytes) !== expected || (header && header !== expected)) throw new Error("remote manifest digest mismatch");
    return true;
  }
  // A release tag is immutable; an identical retry is safe.
  const exists = await checkManifest(ref.tag, source.digest, true);
  if (!exists) {
    for (const blob of source.blobs) {
      // Registries with redirecting blob storage answer an existing blob with a redirect. It counts as present and is
      // never followed, so credentials stay with the registry origin.
      const present = await request(`${base}/blobs/${blob.digest}`, { method: "HEAD", redirect: "manual" });
      if (present.status === 200 || BLOB_REDIRECTS.has(present.status)) { await present.body?.cancel(); continue; }
      await requireStatus(present, [404]);
      const start = await request(`${base}/blobs/uploads/`, { method: "POST" });
      const location = start.headers.get("location");
      await requireStatus(start, [202]);
      if (!location) throw new Error("registry upload location missing");
      const target = new URL(location, `${base}/blobs/uploads/`);
      target.searchParams.set("digest", blob.digest);
      const stream = createReadStream(blob.file);
      try {
        await requireStatus(await request(target.href, { method: "PUT", headers: { "Content-Type": "application/octet-stream", "Content-Length": String(blob.size) }, body: stream }), [201]);
      } finally { stream.destroy(); }
    }
    for (const object of source.manifests) {
      await requireStatus(await request(`${base}/manifests/${object.digest}`, { method: "PUT", headers: { "Content-Type": object.mediaType }, body: object.bytes }), [201]);
    }
    await requireStatus(await request(`${base}/manifests/${ref.tag}`, { method: "PUT", headers: { "Content-Type": source.mediaType }, body: source.bytes }), [201]);
  }
  await checkManifest(ref.tag, source.digest);
  for (const object of source.manifests) await checkManifest(object.digest, object.digest);
  return source.digest;
}
if (isMain(import.meta.url)) {
  const args = options(process.argv.slice(2), ["--layout", "--reference", "--scheme"]);
  if (!args["--layout"] || !args["--reference"]) throw new Error("expected --layout and --reference");
  console.log(await pushOCI({ directory: args["--layout"], reference: args["--reference"], scheme: args["--scheme"] }));
}
