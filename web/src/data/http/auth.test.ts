// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Sign-in mode and CSRF HTTP contract tests.

import { describe, expect, it, vi } from "vitest";

import { createHttpAdapter } from "./http-adapter";

describe("HTTP sign-in policy", () => {
  it.each(["none", "local", "proxy"] as const)("uses /auth/mode for %s, never authLocalEnabled", async (mode) => {
    const seen: string[] = [];
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(String(input), "https://hub.example.com").pathname;
      seen.push(path);
      if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
      if (path === "/api/environments/0/settings") return new Response(JSON.stringify([{ key: "authLocalEnabled", value: mode === "local" ? "false" : "true", type: "boolean" }]));
      throw new Error(`unexpected ${path}`);
    }) as typeof fetch;
    const adapter = createHttpAdapter({ fetch: fetcher });
    expect(await adapter.getAuthMode()).toBe(mode);
    expect(seen).toEqual(["/api/auth/mode"]);
  });

  it("sends the first login with the CSRF token obtained from mode", async () => {
    const seen: { path: string; token: string | null }[] = [];
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "https://hub.example.com").pathname;
      seen.push({ path, token: new Headers(init?.headers).get("X-CSRF-Token") });
      if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
      return new Response(JSON.stringify({ success: true, data: { user: { id: "admin", username: "admin", requiresPasswordChange: true } } }));
    }) as typeof fetch;
    const adapter = createHttpAdapter({ fetch: fetcher });
    expect((await adapter.login("admin", "example-password")).requiresPasswordChange).toBe(true);
    expect(seen).toEqual([{ path: "/api/auth/mode", token: null }, { path: "/api/auth/login", token: "example".padEnd(43, "x") }]);
  });
});
