// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4037e18b9c926793e2888952b4ec9c0805ecc62e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original whole-tree revision regression coverage for issue #183.

import { vi } from "vitest";
import { createVolumeWorkspace } from "./volume-workspace";

/** Whole-tree revision checks mirror the volume producer, without content comparison. */
export function revisionProducer() {
  let version = 1;
  const contents = new Map([["a.txt", "old A"], ["b.txt", "old B"]]);
  const folders = new Set<string>();
  const events: string[] = [];
  const conflicts: string[] = [];
  const hooks: { tree?: () => Promise<void>; file?: () => void; updated?: () => void } = {};
  const revision = () => `r${version}`;
  const tree = () => ({ files: [...contents.keys(), ...folders].map((name) => ({ name, path: `/${name}`, relativePath: name, isDirectory: folders.has(name), isSymlink: false, editable: !folders.has(name), size: contents.get(name)?.length ?? 0, modTime: "2026-10-22T12:00:00Z", mode: "-rw-r--r--" })), fileTreeRevision: revision(), fileTreeTruncated: false });
  const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }));
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (init?.method === "PUT") {
      const form = init.body as FormData;
      const manifest = JSON.parse(String(form.get("manifest")));
      events.push(`put:${manifest.fileTreeRevision}`);
      if (manifest.fileTreeRevision !== revision()) {
        conflicts.push(manifest.fileTreeRevision);
        return new Response(JSON.stringify({ status: 409 }), { status: 409 });
      }
      for (const change of manifest.fileChanges) {
        if (change.operation === "update_file" || change.operation === "create_file") contents.set(change.relativePath, await (form.getAll("files")[change.uploadIndex] as File).text());
        if (change.operation === "create_folder") folders.add(change.relativePath);
      }
      version++;
      const response = ok(tree());
      hooks.updated?.();
      return response;
    }
    if (url.pathname.endsWith("/file")) {
      const path = url.searchParams.get("relativePath")!;
      events.push(`file:${revision()}`);
      const response = ok({ relativePath: path, path: `/${path}`, name: path, content: contents.get(path), editable: true, size: contents.get(path)?.length, mimeType: "text/plain" });
      hooks.file?.();
      return response;
    }
    events.push(`tree:${revision()}`);
    await hooks.tree?.();
    return ok(tree());
  });
  const workspace = createVolumeWorkspace({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }) });
  return { workspace, contents, events, conflicts, hooks, revision, externalWrite(path = "a.txt", text = "external") { contents.set(path, text); version++; } };
}
