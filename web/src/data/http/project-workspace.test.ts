// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original project workspace public adapter checks for issue #182.

import { describe, expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { directWsCreate } from "./workspace-smoke.test-support";

for (const host of ["direct", "edge"]) {
  describe(`${host} project workspace mapping`, () => {
    function fixture(denied = "") {
      const calls: { url: URL; init?: RequestInit }[] = [];
      const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(String(input), "https://hub.example.com");
        const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
        if (url.pathname.endsWith("/capabilities")) return ok(["getProjectWorkspace", "readProjectFile", "downloadProjectFile", "updateProjectWorkspace"].map((operationId) => ({ operationId, available: operationId !== denied, resourceRef: { kind: "stack", id: `${host}~example` } })));
        calls.push({ url, init });
        if (url.pathname.endsWith("/download")) return new Response(new Uint8Array([0, 255]));
        if (url.pathname.endsWith("/file")) return ok({ relativePath: "hello.txt", path: "/projects/example/hello.txt", name: "hello.txt", editable: true, content: "hello.txt", size: 9, mimeType: "text/plain" });
        return ok(directWsCreate);
      });
      return { adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }), calls };
    }
    it("reads extra files through workspace-file and writes a multipart baseline", async () => {
      const { adapter, calls } = fixture(); const id = `${host}~example`;
      const tree = await adapter.getProjectWorkspace!(id);
      const file = await adapter.readProjectFile!(id, "hello.txt");
      expect(file.content).toBe("hello.txt");
      await adapter.updateProjectWorkspace!(id, tree.fileTreeRevision, [{ kind: "update", path: "hello.txt", content: "new", baseline: file.content! }]);
      expect(calls[1]!.url.searchParams.get("relativePath")).toBe("hello.txt");
      const form = calls[2]!.init!.body as FormData;
      expect(JSON.parse(String(form.get("manifest")))).toEqual({ fileTreeRevision: tree.fileTreeRevision, fileChanges: [{ operation: "update_file", relativePath: "hello.txt", uploadIndex: 0, baselineIndex: 1 }] });
      expect(await (form.getAll("files")[1] as File).text()).toBe("hello.txt");
      expect([...new Uint8Array(await (await adapter.downloadProjectFile!(id, "hello.txt")).arrayBuffer())]).toEqual([0, 255]);
      expect(calls.every((call) => call.url.pathname.startsWith(`/api/environments/${host}/projects/example/workspace`))).toBe(true);
      expect(calls.some((call) => call.url.pathname.endsWith("/compose"))).toBe(false);
    });
    for (const operation of ["getProjectWorkspace", "readProjectFile", "downloadProjectFile", "updateProjectWorkspace"]) {
      it(`gates ${operation} with a stack ref before dispatch`, async () => {
        const { adapter, calls } = fixture(operation); const id = `${host}~example`;
        const methods: Record<string, () => Promise<unknown>> = { getProjectWorkspace: () => adapter.getProjectWorkspace!(id), readProjectFile: () => adapter.readProjectFile!(id, "hello.txt"), downloadProjectFile: () => adapter.downloadProjectFile!(id, "hello.txt"), updateProjectWorkspace: () => adapter.updateProjectWorkspace!(id, "r", [{ kind: "delete", path: "hello.txt" }]) };
        await expect(methods[operation]!()).rejects.toMatchObject({ outcome: { kind: "not-available", operationId: operation, resourceRef: { kind: "stack", id } } });
        expect(calls).toEqual([]);
      });
    }
  });
}
