// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original workspace implementation and contract coverage for issue #182.

import type { DockletAdapter } from "@/data/adapter";
import type { WorkspaceBaseline, WorkspaceGesture, WorkspaceTree } from "@/data/api";
import { joinPath, parentPath } from "@/domain/files";
import { createWorkspaceTransport, WorkspaceError, workspacePath, type WorkspaceOptions } from "./workspace";

type VolumeWorkspace = Pick<DockletAdapter, "listFiles" | "readFile" | "writeFile" | "createEntry" | "renameEntry" | "deleteEntry" | "uploadFile" | "downloadFile">;

export function createVolumeWorkspace(options: WorkspaceOptions): VolumeWorkspace {
  const transport = createWorkspaceTransport(options);
  const trees = new Map<string, WorkspaceTree>();
  const baselines = new Map<string, WorkspaceBaseline>();
  let owner = options.generation?.();
  function current() {
    if (owner !== options.generation?.()) { trees.clear(); baselines.clear(); owner = options.generation?.(); }
  }
  async function tree(id: string) {
    current();
    const result = await transport.list("volume", id);
    trees.set(id, result);
    return result;
  }
  async function mutate(id: string, operation: string, change: WorkspaceGesture, revision?: string) {
    current();
    await transport.require(operation, "volume", id);
    const expected = revision ?? trees.get(id)?.fileTreeRevision;
    if (!expected) throw new WorkspaceError("invalid");
    const result = await transport.update("volume", id, expected, [change]);
    trees.set(id, result);
    return result;
  }
  return {
    async listFiles(id, path) {
      await transport.require("listFiles", "volume", id);
      const result = await tree(id);
      const absolute = `/${workspacePath(path)}`;
      return { volumeId: id, path: absolute, revision: result.fileTreeRevision, truncated: result.fileTreeTruncated, entries: (result.files ?? []).filter((file) => parentPath(`/${file.relativePath}`) === absolute).map((file) => ({ name: file.name, path: `/${file.relativePath}`, type: file.isDirectory && !file.isSymlink ? "dir" : "file", size: file.size, modifiedAt: file.modTime, editable: file.editable && !file.isSymlink, readOnlyReason: file.readOnlyReason })) };
    },
    async readFile(id, path) {
      current();
      await transport.require("readFile", "volume", id);
      const snapshot = trees.get(id) ?? await tree(id);
      const file = await transport.read("volume", id, path);
      const baseline = { content: file.content ?? "", revision: snapshot.fileTreeRevision };
      baselines.set(`${id}:${workspacePath(path)}`, baseline);
      return { path: `/${file.relativePath}`, content: file.content ?? "", editable: file.editable, size: file.size, readOnlyReason: file.readOnlyReason, revision: baseline.revision };
    },
    async writeFile(id, path, content, baseline) {
      current();
      const saved = baseline ?? baselines.get(`${id}:${workspacePath(path)}`);
      if (!saved) throw new WorkspaceError("invalid");
      const result = await mutate(id, "writeFile", { kind: "update", path, content, baseline: saved.content }, saved.revision);
      return { revision: result.fileTreeRevision };
    },
    async createEntry(id, dir, name, type, revision) { await mutate(id, "createEntry", type === "dir" ? { kind: "create-folder", path: joinPath(dir, name) } : { kind: "create-file", path: joinPath(dir, name), content: "" }, revision); },
    async renameEntry(id, path, name, revision) { await mutate(id, "renameEntry", { kind: "rename", path, name }, revision); },
    async deleteEntry(id, path, revision) { await mutate(id, "deleteEntry", { kind: "delete", path }, revision); },
    async uploadFile(id, dir, file, revision) { await mutate(id, "uploadFile", { kind: "upload", path: joinPath(dir, file.name), file }, revision); },
    async downloadFile(id, path) { await transport.require("downloadFile", "volume", id); return transport.download("volume", id, path); },
  };
}
