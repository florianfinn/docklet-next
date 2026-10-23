// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4037e18b9c926793e2888952b4ec9c0805ecc62e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original whole-tree revision regression coverage for issue #183.

import { expect, it } from "vitest";
import { revisionProducer } from "./volume-revision.test-support";
import { createVolumeWorkspace } from "./volume-workspace";
import { WorkspaceConflictError } from "./workspace";

// Producer semantics: one whole-tree revision checked on every manifest PUT.
it("a file read after a tree change carries the tree revision current at read time", async () => {
  let rev = 1;
  const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }));
  const tree = () => ({ files: [{ name: "a.txt", path: "/a.txt", relativePath: "a.txt", isDirectory: false, isSymlink: false, editable: true, size: 1, modTime: "2026-10-22T12:00:00Z", mode: "-rw-r--r--" }], fileTreeRevision: `r${rev}`, fileTreeTruncated: false });
  let releaseTree: (() => void) | undefined;
  const treeBarrier = new Promise<void>((resolve) => { releaseTree = resolve; });
  let recovery = false;
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (init?.method === "PUT") {
      const manifest = JSON.parse(String((init.body as FormData).get("manifest")));
      if (manifest.fileTreeRevision !== `r${rev}`) return new Response(JSON.stringify({ status: 409 }), { status: 409 });
      rev++; return ok(tree());
    }
    if (url.pathname.endsWith("/file")) { releaseTree?.(); return ok({ relativePath: "a.txt", path: "/a.txt", name: "a.txt", content: `content@r${rev}`, editable: true, size: 1, mimeType: "text/plain" }); }
    if (recovery) { recovery = false; await treeBarrier; }
    return ok(tree());
  }) as typeof fetch;
  const ws = createVolumeWorkspace({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "x" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }) });
  await ws.listFiles("edge~data", "/");
  rev = 2; recovery = true; // another client or a container changes the volume
  // The UI's conflict recovery refetches listings and content concurrently.
  const [, reread] = await Promise.all([ws.listFiles("edge~data", "/"), ws.readFile("edge~data", "/a.txt")]);
  expect(reread.content).toBe("content@r2");
  let conflict = false;
  try { await ws.writeFile("edge~data", "/a.txt", "mine", { content: reread.content, revision: reread.revision! }); } catch (e) { conflict = e instanceof WorkspaceConflictError; }
  expect({ revision: reread.revision, conflict }).toEqual({ revision: "r2", conflict: false });
});

it("re-reads content when an external write happens during the read", async () => {
  const producer = revisionProducer();
  producer.hooks.file = () => { producer.hooks.file = undefined; producer.externalWrite(); };
  const file = await producer.workspace.readFile("edge~data", "/a.txt");
  expect({ content: file.content, revision: file.revision }).toEqual({ content: "external", revision: "r2" });
  await producer.workspace.writeFile("edge~data", "/a.txt", "mine", { content: file.content, revision: file.revision! });
  expect(producer.conflicts).toEqual([]);
});

it("refuses a baseline when the tree never stabilises", async () => {
  const producer = revisionProducer();
  producer.hooks.file = () => producer.externalWrite();
  await expect(producer.workspace.readFile("edge~data", "/a.txt")).rejects.toBeInstanceOf(WorkspaceConflictError);
  expect(producer.events.filter((event) => event.startsWith("file:"))).toHaveLength(3);
});

it("keeps implicit baselines current after own writes but conflicts on external changes", async () => {
  const producer = revisionProducer();
  await producer.workspace.readFile("edge~data", "/a.txt");
  await producer.workspace.readFile("edge~data", "/b.txt");
  await producer.workspace.writeFile("edge~data", "/a.txt", "mine A");
  await producer.workspace.writeFile("edge~data", "/b.txt", "mine B");
  producer.externalWrite();
  await expect(producer.workspace.writeFile("edge~data", "/a.txt", "again")).rejects.toBeInstanceOf(WorkspaceConflictError);
  expect(producer.contents.get("a.txt")).toBe("external");
});
