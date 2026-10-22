// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original workspace error presentation for issue #182.

import { AdapterOutcomeError } from "@/data/capability";
import { WorkspaceConflictError, WorkspaceError } from "@/data/http/workspace";

export function workspaceErrorKey(error: unknown) {
  if (error instanceof WorkspaceConflictError) return "conflict";
  if (error instanceof WorkspaceError) return error.code;
  if (error instanceof AdapterOutcomeError && error.outcome.kind === "not-available") return "forbidden";
  if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "agent.update-required") return "agent-update";
  return "unavailable";
}
