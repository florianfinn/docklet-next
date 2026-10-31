// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W2 generated-DTO transport and typed conflict checks.

import { expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createContainerOrigin } from "./container-origin";

it("W2 sends current dockerId on PUT and resets through DELETE, refreshing after success", async () => {
  const changed = vi.fn();
  const fetcher = vi.fn(async (_url, _init) => new Response(JSON.stringify({ origin: "unraid", originSource: "manual" })));
  const api = createContainerOrigin({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), generation: () => 0, changed });
  await api.setContainerOrigin("example~current /?#", "unraid");
  expect(fetcher.mock.calls[0]![0]).toBe("/api/environments/example/containers/current%20%2F%3F%23/origin");
  const init = fetcher.mock.calls[0]![1] as RequestInit;
  expect(init.method).toBe("PUT");
  const expected: components["schemas"]["ContainerOriginAssignment"] = { origin: "unraid", dockerId: "current /?#" };
  expect(JSON.parse(init.body as string)).toEqual(expected);
  await api.setContainerOrigin("example~current /?#", "unknown");
  expect(fetcher.mock.calls[1]![1]).toMatchObject({ method: "DELETE" });
  expect((fetcher.mock.calls[1]![1] as RequestInit).body).toBeUndefined();
  expect(changed).toHaveBeenCalledTimes(2);
});

it("W2 retains the real origin Problem code and fences late identity responses", async () => {
  let version = 0;
  const problem: components["schemas"]["ContainerProblem"] = { code: "container.origin-locked", origin: "unknown", status: 409 };
  const fetcher = vi.fn(async () => new Response(JSON.stringify(problem), { status: 409 }));
  const changed = vi.fn();
  const api = createContainerOrigin({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), generation: () => version, changed });
  await expect(api.setContainerOrigin("0~current", "standalone")).rejects.toMatchObject({ outcome: { code: "container.origin-locked", status: 409 } });
  fetcher.mockImplementation(async () => { version++; return new Response("{}"); });
  await expect(api.setContainerOrigin("0~current", "unraid")).rejects.toMatchObject({ message: "CancelledError" });
  expect(changed).not.toHaveBeenCalled();
});
