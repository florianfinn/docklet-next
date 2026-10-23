// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 3d2d4f27b524b3422d010d6a69a22fe57225a78f
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original production boundary and regression coverage for issue #27.

import type { ResourceRef } from "@/data/capability";
import { vi } from "vitest";

import { resourceFixture } from "@/data/http/resources.test-support";
import { directWsCreate } from "@/data/http/workspace-smoke.test-support";

// Scoped storage descriptors match the producer's resource-kind admission.
function availableForRef(operation: string, ref: ResourceRef) {
  const [host, raw, extra] = ref.id.split("~");
  const scoped = !!host && !!raw && extra === undefined;
  if (["listFiles", "readFile", "writeFile", "createEntry", "renameEntry", "deleteEntry", "uploadFile", "downloadFile"].includes(operation)) return ref.kind === "volume" && scoped && !raw!.startsWith("bind:");
  if (["listVolumes", "getVolume"].includes(operation)) return ref.kind === "hub" || ref.kind === "host" && !!host && raw === undefined;
  if (["getProjectWorkspace", "readProjectFile", "downloadProjectFile", "updateProjectWorkspace"].includes(operation)) return ref.kind === "stack" && scoped;
  return true;
}

/** Reuse inventory contract fixtures and the sanitised real workspace envelope. */
export function connectedFixture() {
  const inventory = resourceFixture();
  inventory.state.hostIds = ["first"];
  const state = { signedIn: false, failed: "", calls: [] as string[] };
  const json = (data: unknown) => Response.json({ success: true, data }, { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
  const user = { id: "admin", username: "operator", requiresPasswordChange: false, locale: "de" };
  const fetcher = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const path = url.pathname;
    state.calls.push(path);
    if (path === "/api/auth/mode") return json({ mode: "local" });
    if (path === "/api/auth/me") return state.signedIn ? json(user) : new Response("{}", { status: 401 });
    if (path === "/api/auth/login") { state.signedIn = true; return json({ user }); }
    if (path === "/api/environments/0/settings") return Response.json([{ key: "setupCompleted", value: "true" }]);
    if (path === "/api/capabilities") return json([
      "listHosts", "listStacks", "getStack", "listContainers", "listImages", "listVolumes", "getVolume", "getTopology", "subscribeMetrics", "listFiles", "readFile", "writeFile", "createEntry", "renameEntry", "deleteEntry", "uploadFile", "downloadFile",
      // A server descriptor cannot enable an adapter mapping that does not exist.
      "runBackup", "setBackupPolicy", "restoreBackup", "checkUpdates", "installStackUpdates", "installContainerUpdate", "reconnectHost", "restartAgent", "restartService", "setContainerOrigin", "importSource", "updateHost", "restoreRestorePoint",
    ].map((operationId) => {
      const resourceRef = { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" };
      const available = availableForRef(operationId, resourceRef);
      return { operationId, available, resourceRef, disabledReasonCode: available ? undefined : "capability.unavailable" };
    }));
    if (state.failed && path.includes(state.failed)) return Response.json({ code: "http.error" }, { status: 500 });
    if (path.endsWith("/workspace")) return json(directWsCreate);
    if (path.endsWith("/workspace/file")) return json({ relativePath: "hello.txt", path: "/hello.txt", name: "hello.txt", content: "hello.txt", editable: true, size: 9, mimeType: "text/plain" });
    return inventory.fetcher(input);
  });
  return { state, fetcher, webSocket: inventory.webSocket, source: inventory.source };
}
