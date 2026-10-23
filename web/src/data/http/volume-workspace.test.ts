// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original live volume workspace adapter checks for issue #182.

import { describe, expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { WorkspaceConflictError } from "./workspace";
import { directWs, directWsCreate, edgeWs, edgeWsCreate, staleWorkspace } from "./workspace-smoke.test-support";

for (const [host, empty, created] of [["direct", directWs, directWsCreate], ["edge", edgeWs, edgeWsCreate]] as const) {
  describe(`${host} live volume workspace`, () => {
    function fixture(denied = "") {
      let tree = empty;
      const calls: { url: URL; init?: RequestInit }[] = [];
      const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(String(input), "https://hub.example.com");
        const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
        if (url.pathname.endsWith("/capabilities")) return ok(["listFiles", "readFile", "writeFile", "createEntry", "renameEntry", "deleteEntry", "uploadFile", "downloadFile"].map((operationId) => ({ operationId, available: operationId !== denied, resourceRef: { kind: "volume", id: `${host}~data` } })));
        calls.push({ url, init });
        if (init?.method === "PUT") {
          const form = init.body as FormData;
          const manifest = JSON.parse(String(form.get("manifest")));
          if (manifest.fileTreeRevision !== tree.fileTreeRevision) return new Response(JSON.stringify(staleWorkspace), { status: 409 });
          tree = created;
          return ok(tree);
        }
        if (url.pathname.endsWith("/download")) return new Response(new Uint8Array([0, 255, 2]));
        if (url.pathname.endsWith("/file")) return ok({ relativePath: "hello.txt", path: "/hello.txt", name: "hello.txt", content: "hello.txt", editable: true, size: 9, mimeType: "text/plain" });
        return ok(tree);
      });
      return { adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }), calls };
    }
    it("lists, creates, edits and downloads against the real producer envelopes", async () => {
      const { adapter, calls } = fixture(); const id = `${host}~data`;
      expect((await adapter.listFiles(id, "/")).entries).toEqual([]);
      await adapter.createEntry(id, "/", "hello.txt", "file", empty.fileTreeRevision);
      const listing = await adapter.listFiles(id, "/");
      expect(listing.entries[0]?.path).toBe("/hello.txt");
      expect(listing.revision).toBe(created.fileTreeRevision);
      const file = await adapter.readFile(id, "/hello.txt");
      await adapter.writeFile(id, file.path, "next", { revision: file.revision!, content: file.content });
      const form = calls.findLast((call) => call.init?.method === "PUT")!.init!.body as FormData;
      const manifest = JSON.parse(String(form.get("manifest")));
      expect(manifest.fileChanges[0]).toEqual({ operation: "update_file", relativePath: "hello.txt", uploadIndex: 0, baselineIndex: 1 });
      expect(await (form.getAll("files")[1] as File).text()).toBe("hello.txt");
      expect([...new Uint8Array(await (await adapter.downloadFile!(id, "/hello.txt")).arrayBuffer())]).toEqual([0, 255, 2]);
      expect(calls.every((call) => call.url.pathname.startsWith(`/api/environments/${host}/volumes/data/workspace`))).toBe(true);
    });
    it("surfaces stale baseline rather than silently adopting a newer tree", async () => {
      const { adapter } = fixture(); const id = `${host}~data`;
      await adapter.listFiles(id, "/");
      await adapter.createEntry(id, "/", "hello.txt", "file");
      await expect(adapter.deleteEntry(id, "/hello.txt", empty.fileTreeRevision)).rejects.toBeInstanceOf(WorkspaceConflictError);
    });
    for (const operation of ["listFiles", "readFile", "writeFile", "createEntry", "renameEntry", "deleteEntry", "uploadFile", "downloadFile"]) {
      it(`checks the scoped ${operation} capability before dispatch`, async () => {
        const { adapter, calls } = fixture(operation); const id = `${host}~data`;
        const methods: Record<string, () => Promise<unknown>> = {
          listFiles: () => adapter.listFiles(id, "/"), readFile: () => adapter.readFile(id, "/hello.txt"),
          writeFile: () => adapter.writeFile(id, "/hello.txt", "next", { revision: "r", content: "old" }),
          createEntry: () => adapter.createEntry(id, "/", "hello.txt", "file", "r"), renameEntry: () => adapter.renameEntry(id, "/hello.txt", "new.txt", "r"),
          deleteEntry: () => adapter.deleteEntry(id, "/hello.txt", "r"), uploadFile: () => adapter.uploadFile(id, "/", { name: "hello.txt", size: 1, blob: new Blob(["a"]) }, "r"),
          downloadFile: () => adapter.downloadFile!(id, "/hello.txt"),
        };
        await expect(methods[operation]!()).rejects.toMatchObject({ outcome: { kind: "not-available", operationId: operation, resourceRef: { kind: "volume", id } } });
        expect(calls.length).toBe(0);
      });
    }
  });
}
