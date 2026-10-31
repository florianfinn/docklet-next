// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original request and typed connection result matrix. M14 host maintenance and availability.

import { expect, it, vi } from "vitest";
import { createHostChecks } from "./host-checks";

it.each(["online", "standby-opened", "host.unreachable", "agent.invalid-token", "agent.update-required", "agent.ui-configuration-disabled"])("H2 transports %s without changing identity", async code => {
  const success = code === "online" || code === "standby-opened";
  const fetcher = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    expect(String(url)).toBe("/api/environments/example/test");
    expect(init?.method).toBe("POST"); expect(init?.body).toBeUndefined();
    expect(new Headers(init?.headers).has("Idempotency-Key")).toBe(false);
    return new Response(JSON.stringify(success ? { success: true, data: { status: code } } : { code, status: 502, title: "Host operation refused" }), { status: success ? 200 : 502 });
  });
  const changed = vi.fn();
  const capability = vi.fn(async (operationId: string, resourceRef: { kind: string; id: string }) => ({ operationId, resourceRef, available: true }));
  const checks = createHostChecks({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability, generation: () => 0, changed });
  for (const operation of ["connectHost", "reconnectHost"] as const) {
    if (success) expect(await checks[operation]("example")).toEqual({ status: code });
    else await expect(checks[operation]("example")).rejects.toMatchObject({ outcome: { kind: "error", code, status: 502 } });
    expect(capability).toHaveBeenLastCalledWith(operation, { kind: "host", id: "example" });
  }
  expect(changed).toHaveBeenCalledTimes(2);
});
