// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real-adapter host maintenance request and problem tests. M14 host maintenance and availability.

import { expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { HOST_COLORS, defaultHostColor } from "@/domain/hosts";
import { createHttpAdapter } from "./http-adapter";
import { mapHost } from "./hosts";

const host: components["schemas"]["EnvironmentEnvironment"] = { id: "example", name: "Example", color: null, apiUrl: "http://[2001:db8::10]:3553", enabled: true, isEdge: false, status: "online" };
function fixture(code?: string) {
  const writes: { path: string; method: string; body: unknown }[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), method = init?.method ?? "GET";
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (url.pathname.endsWith("/auth/mode")) return json({ mode: "none" });
    if (url.pathname.endsWith("/capabilities")) return json(["updateHost"].map(operationId => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") } })));
    expect(url.pathname).toBe("/api/environments/example"); expect(method).toBe("PUT");
    expect(new Headers(init?.headers).has("Idempotency-Key")).toBe(false);
    const body = JSON.parse(String(init?.body)) as { name: string; color: string | null };
    writes.push({ path: url.pathname, method, body });
    if (code) return new Response(JSON.stringify({ code, status: 409, title: "Host operation refused" }), { status: 409, headers: { "Content-Type": "application/problem+json" } });
    return json({ ...host, ...body });
  });
  return { adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }), writes };
}

it("H1 maps backend colour and stable palette defaults instead of grey", () => {
  expect(mapHost(host).color).toBe(defaultHostColor(host.id));
  expect(HOST_COLORS.includes(mapHost(host).color)).toBe(true);
  expect(mapHost({ ...host, color: "#12ab34" }).color).toBe("#12ab34");
  expect(defaultHostColor(host.id)).toBe(defaultHostColor(host.id));
});
it("H1 sends a narrow host edit and an explicit null reset", async () => {
  const f = fixture();
  expect(await f.adapter.updateHost("example", { name: " Renamed ", color: "#12AB34" })).toMatchObject({ id: "example", name: "Renamed", color: "#12ab34" });
  expect(f.writes[0]).toEqual({ path: "/api/environments/example", method: "PUT", body: { name: "Renamed", color: "#12ab34" } });
  expect((await f.adapter.updateHost("example", { name: "Renamed", color: null })).color).toBe(defaultHostColor("example"));
  expect(f.writes[1]?.body).toEqual({ name: "Renamed", color: null });
});
it.each(["host.name-invalid", "host.name-taken"])("H1 preserves %s from the real problem shape", async code => {
  const f = fixture(code);
  await expect(f.adapter.updateHost("example", { name: "Renamed", color: null })).rejects.toMatchObject({ outcome: { kind: "error", code, status: 409 } });
});
