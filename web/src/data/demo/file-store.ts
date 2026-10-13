import { baseName, joinPath, looksLikeText, parentPath, type DirListing, type FileContent, type FileEntry } from "@/domain/files";

import type { FileSeed } from "./fixtures/volume-files";
import { FIXTURE_NOW } from "./fixtures/time";

interface Node {
  type: "dir" | "file";
  size: number;
  mtime: string;
  content?: string;
}

export class FileStoreError extends Error {
  readonly code: "not-found" | "exists" | "not-editable" | "invalid-name";
  constructor(code: FileStoreError["code"]) {
    super(code);
    this.code = code;
    this.name = "FileStoreError";
  }
}

/** In-memory file trees per storage entry, seeded lazily from fixtures. */
export class FileStore {
  private readonly trees = new Map<string, Map<string, Node>>();

  has(volumeId: string): boolean {
    return this.trees.has(volumeId);
  }

  ensure(volumeId: string, seed: () => FileSeed[]): Map<string, Node> {
    let tree = this.trees.get(volumeId);
    if (tree) return tree;
    tree = new Map([["/", { type: "dir", size: 0, mtime: new Date(FIXTURE_NOW).toISOString() }]]);
    for (const s of seed()) {
      const isDir = s.path.endsWith("/");
      const path = isDir ? s.path.slice(0, -1) : s.path;
      const mtime = new Date(FIXTURE_NOW - (s.ageDays ?? 3) * 86_400_000).toISOString();
      this.mkdirs(tree, isDir ? path : parentPath(path), mtime);
      if (!isDir) tree.set(path, { type: "file", size: s.content !== undefined ? new TextEncoder().encode(s.content).length : Math.round(s.size ?? 0), mtime, content: s.content });
    }
    this.trees.set(volumeId, tree);
    return tree;
  }

  clone(from: string, to: string): void {
    const src = this.trees.get(from);
    if (src) this.trees.set(to, new Map([...src].map(([k, v]) => [k, { ...v }])));
  }

  drop(volumeId: string): void {
    this.trees.delete(volumeId);
  }

  size(volumeId: string): number {
    let n = 0;
    for (const node of this.trees.get(volumeId)?.values() ?? []) n += node.type === "file" ? node.size : 0;
    return n;
  }

  list(volumeId: string, path: string): DirListing {
    const tree = this.tree(volumeId);
    const dir = tree.get(path);
    if (!dir || dir.type !== "dir") throw new FileStoreError("not-found");
    const entries: FileEntry[] = [];
    for (const [p, node] of tree) {
      if (p === "/" || parentPath(p) !== path) continue;
      const name = baseName(p);
      entries.push({ name, path: p, type: node.type, size: node.type === "dir" ? this.dirSize(tree, p) : node.size, modifiedAt: node.mtime, editable: node.type === "file" && node.content !== undefined && looksLikeText(name) });
    }
    entries.sort((a, b) => (a.type === b.type ? a.name.localeCompare(b.name, "de") : a.type === "dir" ? -1 : 1));
    return { volumeId, path, entries };
  }

  read(volumeId: string, path: string): FileContent {
    const node = this.tree(volumeId).get(path);
    if (!node || node.type !== "file") throw new FileStoreError("not-found");
    const editable = node.content !== undefined && looksLikeText(baseName(path));
    return { path, content: node.content ?? "", editable, size: node.size };
  }

  write(volumeId: string, path: string, content: string): void {
    const tree = this.tree(volumeId);
    const node = tree.get(path);
    if (!node || node.type !== "file") throw new FileStoreError("not-found");
    if (node.content === undefined) throw new FileStoreError("not-editable");
    tree.set(path, { ...node, content, size: new TextEncoder().encode(content).length, mtime: new Date().toISOString() });
  }

  create(volumeId: string, dir: string, name: string, type: "dir" | "file", content = "", size?: number): void {
    const tree = this.tree(volumeId);
    if (tree.get(dir)?.type !== "dir") throw new FileStoreError("not-found");
    if (!name.trim() || /[/\\\0]/.test(name)) throw new FileStoreError("invalid-name");
    const path = joinPath(dir, name);
    if (tree.has(path)) throw new FileStoreError("exists");
    const now = new Date().toISOString();
    tree.set(path, type === "dir" ? { type, size: 0, mtime: now } : { type, size: size ?? new TextEncoder().encode(content).length, mtime: now, content: size === undefined ? content : undefined });
  }

  rename(volumeId: string, path: string, name: string): void {
    const tree = this.tree(volumeId);
    if (!tree.has(path) || path === "/") throw new FileStoreError("not-found");
    const next = joinPath(parentPath(path), name);
    if (next === path) return;
    if (tree.has(next)) throw new FileStoreError("exists");
    for (const [p, node] of [...tree]) {
      if (p === path || p.startsWith(`${path}/`)) {
        tree.delete(p);
        tree.set(next + p.slice(path.length), p === path ? { ...node, mtime: new Date().toISOString() } : node);
      }
    }
  }

  remove(volumeId: string, path: string): void {
    const tree = this.tree(volumeId);
    if (!tree.has(path) || path === "/") throw new FileStoreError("not-found");
    for (const p of [...tree.keys()]) if (p === path || p.startsWith(`${path}/`)) tree.delete(p);
  }

  private tree(volumeId: string): Map<string, Node> {
    const tree = this.trees.get(volumeId);
    if (!tree) throw new FileStoreError("not-found");
    return tree;
  }

  private dirSize(tree: Map<string, Node>, path: string): number {
    let n = 0;
    for (const [p, node] of tree) if (node.type === "file" && p.startsWith(`${path}/`)) n += node.size;
    return n;
  }

  private mkdirs(tree: Map<string, Node>, path: string, mtime: string): void {
    const parts = path.split("/").filter(Boolean);
    for (let i = 1; i <= parts.length; i++) {
      const p = `/${parts.slice(0, i).join("/")}`;
      if (!tree.has(p)) tree.set(p, { type: "dir", size: 0, mtime });
    }
  }
}
