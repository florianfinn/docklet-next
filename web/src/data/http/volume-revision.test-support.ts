// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4037e18b9c926793e2888952b4ec9c0805ecc62e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original whole-tree revision regression coverage for issue #183.

import { vi } from "vitest";
import type { WorkspaceGesture } from "@/data/api";
import { createWorkspaceTransport } from "./workspace";
import { createVolumeWorkspace } from "./volume-workspace";

/** Producer-shaped multipart transport, with an optional baseline content guard. */
export function revisionProducer(compareBaseline = true) {
  let version = 1;
  const contents = new Map([["a.txt", "old A"], ["b.txt", "old B"]]);
  const folders = new Set<string>();
  const stamps = new Map<string, number>();
  const events: string[] = [];
  const conflicts: string[] = [];
  const hooks: { tree?: () => Promise<void>; file?: () => void; updated?: () => void } = {};
  const revision = () => `r${version}`;
  const tree = () => ({ files: [...contents.keys(), ...folders].map((name) => ({ name: name.split("/").pop()!, path: `/${name}`, relativePath: name, isDirectory: folders.has(name), isSymlink: false, editable: !folders.has(name), size: contents.get(name)?.length ?? 0, modTime: `2026-10-22T12:00:${String(stamps.get(name) ?? 0).padStart(2, "0")}Z`, mode: "-rw-r--r--" })), fileTreeRevision: revision(), fileTreeTruncated: false });
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
        const path = change.relativePath;
        if (change.operation === "update_file" && compareBaseline && change.baselineIndex !== undefined && contents.get(path) !== await (form.getAll("files")[change.baselineIndex] as File).text()) {
          conflicts.push(manifest.fileTreeRevision);
          return new Response(JSON.stringify({ status: 409 }), { status: 409 });
        }
        if (change.operation === "create_file" && (contents.has(path) || folders.has(path))) return new Response("{}", { status: 400 });
        if (change.operation === "update_file" || change.operation === "create_file") {
          contents.set(path, await (form.getAll("files")[change.uploadIndex] as File).text()); stamps.set(path, version + 1);
        }
        if (change.operation === "create_folder") folders.add(path);
        if (change.operation === "delete") {
          for (const key of [...contents.keys()]) if (key === path || key.startsWith(`${path}/`)) contents.delete(key);
          for (const key of [...folders]) if (key === path || key.startsWith(`${path}/`)) folders.delete(key);
        }
        if (change.operation === "rename" || change.operation === "move") {
          const target = change.operation === "rename" ? [...path.split("/").slice(0, -1), change.newName].join("/") : [change.newParentPath, path.split("/").pop()].filter(Boolean).join("/");
          for (const [key, value] of [...contents]) if (key === path || key.startsWith(`${path}/`)) { contents.delete(key); contents.set(target + key.slice(path.length), value); stamps.set(target + key.slice(path.length), stamps.get(key) ?? 0); }
          for (const key of [...folders]) if (key === path || key.startsWith(`${path}/`)) { folders.delete(key); folders.add(target + key.slice(path.length)); }
        }
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
  const options = { baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability: async (operationId: string, resourceRef: import("@/data/capability").ResourceRef) => ({ operationId, resourceRef, available: true }) };
  const workspace = createVolumeWorkspace(options);
  const transport = createWorkspaceTransport(options);
  return { workspace, apply: (changes: WorkspaceGesture[]) => transport.update("volume", "edge~data", revision(), changes), contents, folders, events, conflicts, hooks, revision, tree, externalFolder(path: string) { folders.add(path); version++; }, externalWrite(path = "a.txt", text = "external") { contents.set(path, text); version++; stamps.set(path, version); } };
}
