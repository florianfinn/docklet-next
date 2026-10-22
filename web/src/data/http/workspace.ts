// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original workspace implementation and contract coverage for issue #182.

import type { WorkspaceGesture, WorkspaceTree, WorkspaceContent } from "@/data/api";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "@/data/capability";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";

export class WorkspaceConflictError extends Error {
  constructor() { super("workspace.conflict"); this.name = "WorkspaceConflictError"; }
}

export class WorkspaceError extends Error {
  readonly code: "forbidden" | "not-found" | "too-large" | "invalid" | "helper-incompatible" | "helper-unavailable" | "unavailable" | "unknown";
  readonly status?: number;
  constructor(code: "forbidden" | "not-found" | "too-large" | "invalid" | "helper-incompatible" | "helper-unavailable" | "unavailable" | "unknown", status?: number) {
    super(`workspace.${code}`); this.name = "WorkspaceError"; this.code = code; this.status = status;
  }
}

/** The producer accepts leading slashes but rejects traversal beyond the root. */
export function workspacePath(path: string): string {
  if (path.includes("\0") || path.includes("\\")) throw new WorkspaceError("forbidden");
  const parts: string[] = [];
  for (const part of path.split("/")) {
    if (!part || part === ".") continue;
    if (part === "..") {
      if (!parts.length) throw new WorkspaceError("forbidden");
      parts.pop();
    } else parts.push(part);
  }
  return parts.join("/");
}

export function workspaceManifest(revision: string, gestures: WorkspaceGesture[]): FormData {
  if (!revision.trim() || gestures.length < 1 || gestures.length > 500) throw new WorkspaceError("invalid");
  const form = new FormData();
  let index = 0;
  const upload = (blob: Blob, name: string) => { form.append("files", blob, name); return index++; };
  const fileChanges = gestures.map((gesture) => {
    const relativePath = workspacePath(gesture.path);
    if (!relativePath) throw new WorkspaceError("forbidden");
    switch (gesture.kind) {
      case "create-file": return { operation: "create_file", relativePath, uploadIndex: upload(new Blob([gesture.content]), "content") };
      case "create-folder": return { operation: "create_folder", relativePath };
      case "update": return { operation: "update_file", relativePath, uploadIndex: upload(new Blob([gesture.content]), "content"), baselineIndex: upload(new Blob([gesture.baseline]), "baseline") };
      case "upload": {
        if (!gesture.file.blob && gesture.file.content === undefined) throw new WorkspaceError("invalid");
        return { operation: "create_file", relativePath, uploadIndex: upload(gesture.file.blob ?? new Blob([gesture.file.content!]), gesture.file.name) };
      }
      case "rename": return { operation: "rename", relativePath, newName: gesture.name };
      case "move": return { operation: "move", relativePath, newParentPath: workspacePath(gesture.parent) };
      case "delete": return { operation: "delete", relativePath, recursive: true };
    }
  });
  // ReadWorkspaceUploads indexes repeated `files` parts, never their filenames.
  form.append("manifest", JSON.stringify({ fileTreeRevision: revision, fileChanges }));
  return form;
}

export interface WorkspaceOptions {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation?: () => number;
}

export function createWorkspaceTransport({ baseUrl, client, capability, generation = () => 0 }: WorkspaceOptions) {
  function route(kind: "volume" | "stack", id: string) {
    const [host, raw] = splitInventoryId(id);
    if (kind === "volume" && raw.startsWith("bind:")) throw new WorkspaceError("forbidden");
    return `${baseUrl}/environments/${encodeURIComponent(host)}/${kind === "volume" ? "volumes" : "projects"}/${encodeURIComponent(raw)}/workspace`;
  }
  async function require(operation: string, kind: "volume" | "stack", id: string) {
    const ref = { kind, id };
    const descriptor = await capability(operation, ref);
    if (!descriptor.available) throw unavailable(operation, ref, descriptor.disabledReasonCode);
  }
  async function response(kind: "volume" | "stack", id: string, suffix = "", body?: FormData): Promise<Response> {
    const owner = generation();
    let result: Response;
    try { result = await client.fetch(`${route(kind, id)}${suffix}`, { method: body ? "PUT" : "GET", credentials: "same-origin", cache: "no-store", headers: { Accept: suffix.includes("download") ? "application/octet-stream" : "application/json" }, body }); }
    catch (error) { if (error instanceof AdapterOutcomeError || error instanceof WorkspaceError) throw error; throw new WorkspaceError(body ? "unknown" : "unavailable"); }
    if (generation() !== owner) { await result.body?.cancel(); throw new WorkspaceError("unavailable"); }
    if (!result.ok) {
      let code: string | undefined;
      try { code = (await result.json() as { code?: string }).code; } catch { /* HTTP status remains authoritative. */ }
      if (code?.startsWith("agent.")) throw new AdapterOutcomeError({ kind: "error", code, status: result.status });
      if (result.status === 409) throw new WorkspaceConflictError();
      if (code === "volume.helper-incompatible") throw new WorkspaceError("helper-incompatible", result.status);
      if (code === "volume.helper-unavailable") throw new WorkspaceError("helper-unavailable", result.status);
      throw new WorkspaceError(result.status === 403 ? "forbidden" : result.status === 404 ? "not-found" : result.status === 413 ? "too-large" : result.status === 400 || result.status === 422 ? "invalid" : "unavailable", result.status);
    }
    return result;
  }
  async function json<T>(kind: "volume" | "stack", id: string, suffix = "", body?: FormData): Promise<T> {
    const owner = generation();
    const result = await response(kind, id, suffix, body);
    let envelope: { success: boolean; data: T };
    try { envelope = await result.json(); } catch { throw new WorkspaceError(body ? "unknown" : "unavailable"); }
    if (owner !== generation()) throw new WorkspaceError("unavailable");
    if (envelope.success !== true || !envelope.data) throw new WorkspaceError(body ? "unknown" : "unavailable");
    return envelope.data;
  }
  return {
    require,
    async list(kind: "volume" | "stack", id: string): Promise<WorkspaceTree> {
      const tree = await json<WorkspaceTree>(kind, id);
      if (typeof tree.fileTreeRevision !== "string" || !tree.fileTreeRevision || tree.files !== null && !Array.isArray(tree.files)) throw new WorkspaceError("unavailable");
      return tree;
    },
    read: (kind: "volume" | "stack", id: string, path: string) => json<WorkspaceContent>(kind, id, `/file?${new URLSearchParams({ relativePath: workspacePath(path) })}`),
    async download(kind: "volume" | "stack", id: string, path: string) {
      return (await response(kind, id, `/file/download?${new URLSearchParams({ relativePath: workspacePath(path) })}`)).blob();
    },
    update: (kind: "volume" | "stack", id: string, revision: string, changes: WorkspaceGesture[]) => json<WorkspaceTree>(kind, id, "", workspaceManifest(revision, changes)),
  };
}
