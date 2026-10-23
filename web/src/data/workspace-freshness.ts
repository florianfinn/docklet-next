// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 17796cbc61cdb65fd8ef7236ba301a2fe0286008
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original path and entry freshness helpers for issue #183.

import type { WorkspaceGesture, WorkspaceTree } from "./api";
import { joinPath, parentPath, baseName } from "@/domain/files";

export const underPath = (path: string, root: string) => path === root || path.startsWith(`${root}/`);
export function touchedPaths(change: WorkspaceGesture): string[] {
  const paths = change.kind === "rename" ? [change.path, joinPath(parentPath(change.path), change.name)] : change.kind === "move" ? [change.path, joinPath(change.parent, baseName(change.path))] : [change.path];
  return paths.map((path) => `/${path.replace(/^\/+/, "")}`);
}
export function entryIdentity(tree: WorkspaceTree, path: string): string | undefined {
  const entry = tree.files?.find((file) => file.relativePath === path.replace(/^\/+/, ""));
  return entry && JSON.stringify([entry.size, entry.modTime, entry.mode, entry.isDirectory, entry.isSymlink]);
}
