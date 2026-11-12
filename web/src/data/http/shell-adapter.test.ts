// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original HTTP adapter shell capability and identity integration.

import { expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { Socket } from "./shell.test-support";
const response = (data: unknown) => new Response(JSON.stringify({ success: true, data }));
it("connects using the scoped shell capability and closes on identity replacement", async () => {
  const state = { user: "example-user" }, sockets: Socket[] = [];
  const adapter = createHttpAdapter({ fetch: vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) return response([{ operationId: "openContainerShell", available: true, resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") } }]);
    if (url.pathname.endsWith("/auth/me")) return response({ id: state.user, username: "operator", requiresPasswordChange: false });
    return response(null);
  }), shellWebSocket: () => { const socket = new Socket(); sockets.push(socket); return socket; } });
  await adapter.getCurrentUser(); const push = vi.fn(); const session = adapter.openContainerShell("example-host~example", { shell: "sh" }, push);
  await vi.waitFor(() => expect(sockets.length).toBe(1)); sockets[0]!.open();
  expect(adapter.mappedOperations?.has("openContainerShell")).toBe(true);
  state.user = "example-replacement"; await adapter.getCurrentUser();
  expect(sockets[0]!.closed).toBe(true); expect(push.mock.lastCall?.[0]).toEqual({ type: "exit", code: null, reason: "" }); session.close();
});
