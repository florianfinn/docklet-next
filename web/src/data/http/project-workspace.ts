// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original project workspace transport wiring for issue #182.

import type { ProjectWorkspaceApi } from "@/data/api";
import { createWorkspaceTransport, type WorkspaceOptions } from "./workspace";

export function createProjectWorkspace(options: WorkspaceOptions): Required<ProjectWorkspaceApi> {
  const transport = createWorkspaceTransport(options);
  return {
    async getProjectWorkspace(id) { await transport.require("getProjectWorkspace", "stack", id); return transport.list("stack", id); },
    async readProjectFile(id, path) { await transport.require("readProjectFile", "stack", id); return transport.read("stack", id, path); },
    async downloadProjectFile(id, path) { await transport.require("downloadProjectFile", "stack", id); return transport.download("stack", id, path); },
    async updateProjectWorkspace(id, revision, changes) { await transport.require("updateProjectWorkspace", "stack", id); return transport.update("stack", id, revision, changes); },
  };
}
