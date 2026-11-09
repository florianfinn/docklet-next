// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original offline release packaging and verification.

import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdtempSync, rmSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { pushOCI, registryURL } from "../deploy/release/push-oci.mjs";
import { layout, sha256 } from "../deploy/release/common.mjs";
import { fixture } from "./release-fixture.mjs";

async function registryTest({ mismatch = false, bearer = false, basic = false, dockerIndex = false, platformMismatch = false } = {}) {
  const out = mkdtempSync(join(tmpdir(), "docklet-release-upload-"));
  fixture(out);
  if (dockerIndex) { const path = join(out, "index.json"); const root = JSON.parse(readFileSync(path)); root.mediaType = "application/vnd.docker.distribution.manifest.list.v2+json"; writeFileSync(path, JSON.stringify(root)); }
  const source = layout(out), manifests = new Map(), uploads = [], types = new Map(), blobs = new Map();
  let tokenExchanges = 0, origin;
  const server = createServer(async (req, res) => {
    try {
      const path = new URL(req.url, origin).pathname;
      if (path === "/token") {
        tokenExchanges++;
        if (basic) assert.equal(req.headers.authorization, "Basic " + Buffer.from("test:test-only").toString("base64"));
        res.setHeader("Content-Type", "application/json"); res.end('{"token":"test-only"}'); return;
      }
      if (bearer && req.headers.authorization !== "Bearer test-only") {
        res.writeHead(401, { "www-authenticate": `Bearer realm="${origin}/token",service="fixture",scope="repository:example/hub:pull,push"` }); res.end(); return;
      }
      if (basic && !bearer) assert.equal(req.headers.authorization, "Basic " + Buffer.from("test:test-only").toString("base64"));
      if (path.includes("/manifests/")) {
        const key = path.split("/manifests/")[1];
        if (req.method === "PUT") {
          const chunks = []; for await (const chunk of req) chunks.push(chunk);
          const bytes = Buffer.concat(chunks);
          if (key.startsWith("sha256:")) assert.equal(sha256(bytes), key);
          manifests.set(key, bytes); types.set(key, req.headers["content-type"]); uploads.push(key);
          res.writeHead(201); res.end(); return;
        }
        if (!manifests.has(key)) { res.writeHead(404); res.end(); return; }
        const bytes = manifests.get(key);
        const wrong = mismatch || (platformMismatch && key === source.manifests[0].digest);
        const body = wrong ? Buffer.from("different bytes") : bytes;
        res.writeHead(200, { "Content-Type": types.get(key), "Docker-Content-Digest": sha256(body) }); res.end(body); return;
      }
      if (req.method === "HEAD") { res.writeHead(blobs.has(path) ? 200 : 404); res.end(); return; }
      if (req.method === "POST") { res.writeHead(202, { Location: "/upload/id" }); res.end(); return; }
      if (req.method === "PUT" && path === "/upload/id") {
        const chunks = []; for await (const chunk of req) chunks.push(chunk);
        const bytes = Buffer.concat(chunks), digest = new URL(req.url, origin).searchParams.get("digest");
        assert.equal(sha256(bytes), digest); blobs.set(`/v2/example/hub/blobs/${digest}`, bytes); res.writeHead(201); res.end(); return;
      }
      res.writeHead(500); res.end();
    } catch (err) { res.writeHead(500); res.end(String(err)); }
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
  const reference = `${origin.slice(7)}/example/hub:1.0.0@${source.digest}`;
  try {
    const env = basic ? { REGISTRY_USERNAME: "test", REGISTRY_PASSWORD: "test-only" } : {};
    const result = pushOCI({ directory: out, reference, scheme: "http", env });
    if (mismatch || platformMismatch) await assert.rejects(result, /remote manifest digest mismatch/);
    else {
      assert.equal(await result, source.digest);
      assert.equal(uploads.at(-1), "1.0.0");
      assert.deepEqual(manifests.get("1.0.0"), readFileSync(join(out, "index.json")));
      assert.equal(types.get("1.0.0"), source.mediaType);
      assert.deepEqual(uploads.slice(0, -1), source.manifests.map(m => m.digest));
      assert.equal(blobs.size, source.blobs.length);
      assert.equal(await pushOCI({ directory: out, reference, scheme: "http", env }), source.digest, "identical retry verifies rather than overwrites");
    }
    if (bearer) assert.ok(tokenExchanges > 0, "bearer challenge exchange exercised");
  } finally {
    server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); rmSync(out, { recursive: true, force: true });
  }
}
test("OCI upload preserves index/child bytes, media types and tag digest through real registry HTTP transport", () => registryTest());
test("OCI upload preserves an original Docker index media type", () => registryTest({ dockerIndex: true }));
test("OCI upload rejects different remote digest after a successful upload", () => registryTest({ mismatch: true }));
test("OCI upload rejects a changed remote platform manifest", () => registryTest({ platformMismatch: true }));
test("OCI upload uses basic and challenged bearer authentication without Docker", async () => { await registryTest({ basic: true }); await registryTest({ bearer: true, basic: true }); });
test("registry transport permits private HTTP and refuses public cleartext", () => {
  assert.equal(registryURL("127.0.0.1:5000", "http"), "http://127.0.0.1:5000");
  assert.throws(() => registryURL("registry.example.com", "http"), /HTTPS/);
});
test("offline OCI validation rejects corrupted descriptors and payloads", () => {
  const out = mkdtempSync(join(tmpdir(), "docklet-release-corrupt-"));
  try {
    fixture(out); const source = layout(out);
    writeFileSync(source.blobs[0].file, "modified");
    assert.throws(() => layout(out), /bounds|digest/);
  } finally { rmSync(out, { recursive: true, force: true }); }
});
