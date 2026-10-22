// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original workspace implementation and contract coverage for issue #182.

import { describe, expect, it, vi } from "vitest";
import type { WorkspaceGesture } from "@/data/api";
import { AdapterOutcomeError } from "@/data/capability";
import { createWorkspaceTransport, workspaceManifest, WorkspaceConflictError, WorkspaceError } from "./workspace";

// Compared with Get*Workspace/File handlers, types/workspace and
// ReadWorkspaceUploads: envelope, relativePath query and repeated files order.
export const workspaceTree = { files: [{ relativePath: "nested/Grüße file.txt", path: "/nested/Grüße file.txt", name: "Grüße file.txt", size: 3, editable: true, isDirectory: false, isSymlink: false, modTime: "2026-10-22T12:00:00Z" }], fileTreeRevision: "tree-one", fileTreeTruncated: false };
const gestures: WorkspaceGesture[] = [
  { kind: "create-folder", path: "/nested/new" },
  { kind: "create-file", path: "/nested/new/a.txt", content: "new" },
  { kind: "update", path: "/nested/Grüße file.txt", content: "changed", baseline: "old" },
  { kind: "upload", path: "/nested/file.bin", file: { name: "file.bin", size: 3, blob: new Blob([new Uint8Array([0, 255, 1])]) } },
  { kind: "rename", path: "/nested/a.txt", name: "Grüße file.txt" },
  { kind: "move", path: "/nested/b.txt", parent: "/other folder" },
  { kind: "delete", path: "/nested/old" },
];

export function workspaceFixture() {
  const response = { status: 200, code: "", data: workspaceTree as unknown, allowed: true };
  const fetcher = vi.fn(async () => new Response(JSON.stringify(response.status < 300 ? { success: true, data: response.data } : { code: response.code || undefined, detail: "Workspace request failed" }), { status: response.status }));
  const capability = vi.fn(async (operationId: string, resourceRef: { kind: string; id: string }) => ({ operationId, resourceRef, available: response.allowed }));
  const transport = createWorkspaceTransport({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability });
  return { response, fetcher, capability, transport };
}

describe("shared workspace manifest contract", () => {
  for (const kind of ["volume", "stack"] as const) {
    it(`maps every nested ${kind} gesture with revision and producer upload indices`, async () => {
      const { transport, fetcher } = workspaceFixture();
      await transport.update(kind, "edge~data", "tree-one", gestures);
      const init = (fetcher.mock.calls[0] as unknown as [string, RequestInit])[1];
      const form = init.body as FormData;
      const manifest = JSON.parse(String(form.get("manifest")));
      expect(manifest.fileTreeRevision).toBe("tree-one");
      expect(manifest.fileChanges).toEqual([
        { operation: "create_folder", relativePath: "nested/new" },
        { operation: "create_file", relativePath: "nested/new/a.txt", uploadIndex: 0 },
        { operation: "update_file", relativePath: "nested/Grüße file.txt", uploadIndex: 1, baselineIndex: 2 },
        { operation: "create_file", relativePath: "nested/file.bin", uploadIndex: 3 },
        { operation: "rename", relativePath: "nested/a.txt", newName: "Grüße file.txt" },
        { operation: "move", relativePath: "nested/b.txt", newParentPath: "other folder" },
        { operation: "delete", relativePath: "nested/old", recursive: true },
      ]);
      const files = form.getAll("files") as File[];
      expect(await Promise.all(files.slice(0, 3).map((file) => file.text()))).toEqual(["new", "changed", "old"]);
      expect([...new Uint8Array(await files[3]!.arrayBuffer())]).toEqual([0, 255, 1]);
      expect(init.headers).toEqual({ Accept: "application/json" });
    });
  }
  it("rejects missing revisions and escaping paths before dispatch", () => {
    expect(() => workspaceManifest("", gestures)).toThrow(WorkspaceError);
    expect(() => workspaceManifest("r", [{ kind: "delete", path: "../../outside" }])).toThrow(WorkspaceError);
    expect(() => workspaceManifest("r", [{ kind: "create-folder", path: "/" }])).toThrow(WorkspaceError);
  });
});

describe("workspace response mapping", () => {
  for (const kind of ["volume", "stack"] as const) {
    for (const host of ["direct", "edge"]) {
      it(`reads ${kind} content, tree and download through the hub for ${host}`, async () => {
        const { transport, response, fetcher } = workspaceFixture();
        expect(await transport.list(kind, `${host}~data`)).toEqual(workspaceTree);
        for (const reason of [undefined, "binary", "too_large", "symlink", "special"]) {
          response.data = { path: "/private-host-path", relativePath: "nested/Grüße file.txt", name: "Grüße file.txt", content: reason ? undefined : "old", editable: !reason, size: reason === "too_large" ? 1e10 : 3, mimeType: "text/plain", readOnlyReason: reason };
          const file = await transport.read(kind, `${host}~data`, "/nested/Grüße file.txt");
          expect(file.editable).toBe(!reason);
          expect(file.readOnlyReason).toBe(reason);
          expect(file.content).toBe(reason ? undefined : "old");
        }
        await transport.download(kind, `${host}~data`, "/nested/Grüße file.txt");
        expect((fetcher.mock.calls as unknown as [string, RequestInit][]).every(([url]) => url.startsWith(`/api/environments/${host}/${kind === "stack" ? "projects" : "volumes"}/data/workspace`))).toBe(true);
        expect((fetcher.mock.calls.at(-1) as unknown as [string])[0]).toContain("/file/download?relativePath=nested%2FGr%C3%BC%C3%9Fe+file.txt");
      });
    }
  }
  for (const [status, code, expected] of [[403, "", "forbidden"], [404, "", "not-found"], [413, "", "too-large"], [422, "volume.helper-incompatible", "helper-incompatible"], [503, "volume.helper-incompatible", "helper-incompatible"], [503, "volume.helper-unavailable", "helper-unavailable"]] as const) {
    it(`maps ${status} ${code}`, async () => {
      const { transport, response } = workspaceFixture();
      Object.assign(response, { status, code });
      await expect(transport.list("volume", "edge~data")).rejects.toMatchObject({ code: expected, status });
    });
  }
  it("maps untyped Huma 409 to a typed revision conflict", async () => {
    const { transport, response } = workspaceFixture(); response.status = 409;
    await expect(transport.update("stack", "direct~data", "tree-one", gestures)).rejects.toBeInstanceOf(WorkspaceConflictError);
  });
  it("preserves agent update-required", async () => {
    const { transport, response } = workspaceFixture(); response.status = 409; response.code = "agent.update-required";
    await expect(transport.list("volume", "edge~data")).rejects.toBeInstanceOf(AdapterOutcomeError);
  });
});
