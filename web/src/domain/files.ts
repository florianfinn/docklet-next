// File browser model shared by volumes and bind mounts.

export interface FileEntry {
  name: string;
  /** Absolute path inside the volume, "/" is the volume root. */
  path: string;
  type: "dir" | "file";
  size: number;
  modifiedAt: string;
  /** Text content that the in-browser editor may open. */
  editable: boolean;
}

export interface DirListing {
  volumeId: string;
  path: string;
  entries: FileEntry[];
}

export interface FileContent {
  path: string;
  content: string;
  editable: boolean;
  size: number;
}

export type FileNameError = "required" | "invalid" | "exists";

export function joinPath(dir: string, name: string): string {
  return `${dir === "/" ? "" : dir.replace(/\/+$/, "")}/${name}`;
}

export function parentPath(path: string): string {
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return `/${parts.join("/")}`;
}

export function baseName(path: string): string {
  return path.split("/").filter(Boolean).pop() ?? "/";
}

/** Breadcrumb segments: [{ name, path }] from the root. */
export function pathSegments(path: string): { name: string; path: string }[] {
  const parts = path.split("/").filter(Boolean);
  return parts.map((name, i) => ({ name, path: `/${parts.slice(0, i + 1).join("/")}` }));
}

export function validateFileName(name: string, siblings: readonly string[], current?: string): FileNameError | undefined {
  const v = name.trim();
  if (!v) return "required";
  if (v === "." || v === ".." || /[/\\\0]/.test(v) || v.length > 128) return "invalid";
  if (v !== current && siblings.includes(v)) return "exists";
  return undefined;
}

const TEXT_EXT = /\.(ya?ml|json|conf|cfg|ini|toml|txt|md|log|env|xml|properties|php|sh|py|js|ts|html|css|csv|service|lock)$/i;

/** Heuristic for editable text files; databases and media stay read-only. */
export function looksLikeText(name: string): boolean {
  return TEXT_EXT.test(name) || /^(\.env|README|Dockerfile|HEAD|config|PG_VERSION)$/i.test(name);
}
