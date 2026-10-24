// Source: docklet-next/issues/204
// Revision: def05d5f7606c94c26f8fdeb1b16c53c473a0c11
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original host-local partial topology fixtures.

import { resourceFixture } from "./resources.test-support";

export function barrier() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => { release = resolve; });
  return { promise, release };
}

export function topologyFixture(isEdge: boolean) {
  const f = resourceFixture();
  f.state.isEdge = isEdge;
  const base = f.state.networks[0]!;
  f.state.networks = [{ ...base, id: "good", name: "healthy", labels: {} }, { ...base, id: "missing", name: "unavailable", labels: {} }];
  const failures = new Set<string>();
  const holds = new Map<string, { entered: ReturnType<typeof barrier>; done: ReturnType<typeof barrier> }>();
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const hold = holds.get(url.pathname);
    if (hold) { hold.entered.release(); await hold.done.promise; }
    if (failures.has(url.pathname)) throw new TypeError("Synthetic transport failure");
    if (url.pathname === "/api/auth/me") return new Response(JSON.stringify({ success: true, data: { id: "user", username: "operator", role: "admin", locale: "en-US" } }));
    const response = await original(input);
    if (url.pathname === "/api/capabilities") {
      const body = await response.json();
      body.data.push(...["createNetwork", "removeNetwork", "connectNetwork", "disconnectNetwork"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
      return new Response(JSON.stringify(body), { headers: response.headers });
    }
    if (init?.method && init.method !== "GET") throw new Error("Unexpected write");
    return response;
  });
  function delay(path: string) {
    const hold = { entered: barrier(), done: barrier() };
    holds.set(path, hold);
    return hold;
  }
  return { ...f, failures, delay };
}
