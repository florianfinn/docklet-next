// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original live onboarding adapter lifecycle tests.

import { describe, expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";

const base = { id: "example-host", name: "Example", apiUrl: "http://agent.example.com:3552", enabled: true, isEdge: false, status: "pending" };
const discovery = { detectedAt: "2026-10-17T12:00:00Z", agentImage: "registry.example.com/docklet/agent:1.2.3", candidates: [{ id: "public", url: "https://hub.example.com", reach: "unknown", source: "configured" }] };

function fixture(allow = true, metadata: unknown = discovery) {
  let exists = false;
  let current = { ...base };
  const writes: { method: string; path: string; body: unknown }[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const path = url.pathname;
    const method = init?.method ?? "GET";
    const json = (data: unknown, pagination?: object) => new Response(JSON.stringify({ success: true, data, ...(pagination ? { pagination } : {}) }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/capabilities") return json(["registerHost", "issueAgentToken", "removeHost", "listHosts", "discoverHub"].map(operationId => ({ operationId, available: allow, resourceRef: { kind: "hub", id: "0" } })));
    if (path === "/api/auth/mode") return json({ mode: "none" });
    if (method !== "GET") {
      expect(new Headers(init?.headers).has("X-CSRF-Token")).toBe(true);
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      writes.push({ method, path, body });
      if (method === "POST") {
        current = { ...base, apiUrl: String(body?.apiUrl), isEdge: body?.isEdge === true };
        exists = true;
        return json({ ...current, apiKey: "example" });
      }
      if (method === "PUT") return json({ ...current, apiKey: "test-only" });
      if (method === "DELETE") { exists = false; return json({ message: "Removed" }); }
    }
    if (path === "/api/environments") return json(exists ? [current] : [], { totalItems: exists ? 1 : 0 });
    if (path === "/api/hub/discovery") return json(metadata);
    throw new Error(`unexpected ${method} ${path}`);
  });
  return { adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }), writes, fetcher };
}

const input = { name: "Example", color: "#8b5cf6", connection: "direct" as const, address: "agent.example.com:3552", projectDir: "/srv/projects", installDir: "/srv/projects/agent" };

describe("host creation HTTP adapter", () => {
  it.each(["direct", "edge"] as const)("creates %s with an API key; token never enters host reads", async connection => {
    const f = fixture();
    const result = await f.adapter.registerHost({ ...input, connection });
    expect(result.token).toBe("example");
    expect(result.host.status).toBe("pending");
    expect(f.writes[0]).toMatchObject({ method: "POST", path: "/api/environments", body: { useApiKey: true, isEdge: connection === "edge", apiUrl: connection === "edge" ? "edge://pending" : "http://agent.example.com:3552" } });
    const [listed] = await f.adapter.listHosts();
    expect(listed).toEqual(result.host);
    for (const key of ["apiKey", "accessToken", "token"]) expect(listed).not.toHaveProperty(key);
    expect(await f.adapter.issueAgentToken(result.host.id)).toBe("test-only");
    expect(f.writes[1]).toMatchObject({ method: "PUT", path: "/api/environments/example-host", body: { regenerateApiKey: true } });
    expect(JSON.stringify(await f.adapter.listHosts()).includes("test-only")).toBe(false);
    await f.adapter.removeHost(result.host.id);
    expect(await f.adapter.listHosts()).toEqual([]);
    expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("deployment"))).toBe(false);
  });

  it("denies all onboarding calls when the server capability is unavailable", async () => {
    const f = fixture(false);
    await expect(f.adapter.registerHost(input)).rejects.toBeDefined();
    await expect(f.adapter.issueAgentToken("example-host")).rejects.toBeDefined();
    await expect(f.adapter.removeHost("example-host")).rejects.toBeDefined();
    await expect(f.adapter.discoverHub()).rejects.toBeDefined();
    expect(f.writes).toEqual([]);
  });

  it("maps configured-first discovery and preserves pinned image metadata", async () => {
    const f = fixture();
    expect(await f.adapter.discoverHub()).toMatchObject({ ...discovery, simulated: false });
  });

  it.each(["latest", "unsafe-url", "invalid-reach"])("rejects %s metadata", async mode => {
    const f = fixture(true, { ...discovery, ...(mode === "latest" ? { agentImage: "agent:latest" } : { candidates: [{ ...discovery.candidates[0], ...(mode === "unsafe-url" ? { url: "https://operator:example@hub.example.com" } : { reach: "lan" }) }] }) });
    await expect(f.adapter.discoverHub()).rejects.toBeDefined();
  });
});
