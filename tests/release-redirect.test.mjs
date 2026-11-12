// Source: docklet-next/issues/446
// Revision: 18969d4007896659a766e4fb409c5cd74d695476
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original redirect and resume coverage for the release uploader.

import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { pushOCI } from "../deploy/release/push-oci.mjs";
import { layout, sha256 } from "../deploy/release/common.mjs";
import { fixture } from "./release-fixture.mjs";

const listen = (server) => new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve(`http://127.0.0.1:${server.address().port}`)));
const close = async (server) => { server.closeAllConnections(); await new Promise((resolve) => server.close(resolve)); };
const body = async (req) => { const chunks = []; for await (const chunk of req) chunks.push(chunk); return Buffer.concat(chunks); };

// A registry double over real HTTP. Like Docker Distribution it refuses a manifest whose referenced objects are missing.
async function registry(t, options = {}) {
  const state = { blobs: new Map(), manifests: new Map(), types: new Map(), blobPuts: [], heads: [], targetRequests: 0, failUploadsAfter: Infinity, ...options };
  const target = createServer((req, res) => { state.targetRequests++; res.writeHead(200); res.end(); });
  const targetOrigin = await listen(target);
  const server = createServer(async (req, res) => {
    const url = new URL(req.url, "http://registry.invalid");
    const path = url.pathname;
    if (path.startsWith("/same-origin/")) { state.targetRequests++; res.writeHead(200); res.end(state.manifests.get(path.slice(13)) ?? ""); return; }
    if (path.includes("/manifests/")) {
      const key = path.split("/manifests/")[1];
      if (req.method === "PUT") {
        const bytes = await body(req), document = JSON.parse(bytes);
        const refs = [...(document.manifests ?? []), document.config, ...(document.layers ?? [])].filter(Boolean).map((child) => child.digest);
        if (refs.some((digest) => !state.blobs.has(digest) && !state.manifests.has(digest))) { res.writeHead(400); res.end('{"errors":[{"code":"MANIFEST_BLOB_UNKNOWN"}]}'); return; }
        state.manifests.set(key, bytes); state.types.set(key, req.headers["content-type"]); res.writeHead(201); res.end(); return;
      }
      if (state.redirectManifest === key) { res.writeHead(307, { Location: `/same-origin/${state.redirectTo}` }); res.end(); return; }
      if (!state.manifests.has(key)) { res.writeHead(404); res.end(); return; }
      const bytes = state.manifests.get(key);
      res.writeHead(200, { "Content-Type": state.types.get(key), "Docker-Content-Digest": sha256(bytes) }); res.end(bytes); return;
    }
    if (req.method === "HEAD" && path.includes("/blobs/")) {
      const digest = path.split("/blobs/")[1];
      state.heads.push(digest);
      if (state.headStatus) { res.writeHead(state.headStatus); res.end(); return; }
      if (state.redirectBlobs?.has(digest)) { res.writeHead(307, { Location: `${targetOrigin}/storage/${digest}` }); res.end(); return; }
      res.writeHead(state.blobs.has(digest) ? 200 : 404); res.end(); return;
    }
    if (req.method === "POST") {
      if (state.blobPuts.length >= state.failUploadsAfter) { res.writeHead(503); res.end(); return; }
      res.writeHead(202, { Location: "/upload/id" }); res.end(); return;
    }
    if (req.method === "PUT" && path === "/upload/id") {
      const bytes = await body(req), digest = url.searchParams.get("digest");
      assert.equal(sha256(bytes), digest);
      state.blobs.set(digest, bytes); state.blobPuts.push(digest); res.writeHead(201); res.end(); return;
    }
    res.writeHead(500); res.end();
  });
  const origin = await listen(server);
  const out = mkdtempSync(join(tmpdir(), "docklet-release-redirect-"));
  fixture(out);
  t.after(async () => { await close(server); await close(target); rmSync(out, { recursive: true, force: true }); });
  const source = layout(out);
  const push = () => pushOCI({ directory: out, reference: `${origin.slice(7)}/example/hub:1.0.0@${source.digest}`, scheme: "http",
    env: { REGISTRY_USERNAME: "test", REGISTRY_PASSWORD: "test-only" } });
  return { state, source, push };
}

test("a blob HEAD redirect counts as present and is never followed", async (t) => {
  const { state, source, push } = await registry(t);
  const [stored, missing] = source.blobs;
  state.blobs.set(stored.digest, Buffer.from("stored by an earlier upload"));
  state.redirectBlobs = new Set([stored.digest]);
  assert.equal(await push().catch((error) => error.message), source.digest);
  assert.deepEqual(state.blobPuts, [missing.digest]);
  assert.equal(state.targetRequests, 0);
});
test("an upload interrupted after a committed blob resumes through redirected HEADs", async (t) => {
  const { state, source, push } = await registry(t, { failUploadsAfter: 1 });
  await assert.rejects(push(), /HTTP 503/);
  assert.equal(state.blobPuts.length, 1);
  assert.equal(state.manifests.size, 0);
  const committed = state.blobPuts[0];
  state.failUploadsAfter = Infinity; state.redirectBlobs = new Set([committed]); state.blobPuts = [];
  assert.equal(await push().catch((error) => error.message), source.digest);
  assert.deepEqual(state.blobPuts, source.blobs.map((blob) => blob.digest).filter((digest) => digest !== committed));
  assert.equal(state.targetRequests, 0);
  assert.equal(sha256(state.manifests.get("1.0.0")), source.digest);
});
test("a blob HEAD failure stops before any manifest is written", async (t) => {
  const { state, push } = await registry(t, { headStatus: 500 });
  await assert.rejects(push(), /HTTP 500/);
  assert.equal(state.manifests.size, 0);
  assert.equal(state.blobPuts.length, 0);
});
test("a redirect claiming a blob the registry lacks fails at the manifest upload", async (t) => {
  const { state, source, push } = await registry(t);
  state.redirectBlobs = new Set([source.blobs[0].digest]);
  await assert.rejects(push(), /HTTP 400/);
  assert.equal(state.manifests.has("1.0.0"), false);
  assert.equal(state.targetRequests, 0);
});
test("a redirected manifest read is refused without following it", async (t) => {
  const { state, source, push } = await registry(t);
  const index = source.bytes;
  state.manifests.set("valid", index); state.types.set("valid", source.mediaType);
  state.redirectManifest = "1.0.0"; state.redirectTo = "valid";
  await assert.rejects(push());
  assert.equal(state.targetRequests, 0);
});
