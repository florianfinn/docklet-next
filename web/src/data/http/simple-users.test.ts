// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original real-HTTP assigned-stack and personal-key contract coverage.

import { createServer, request as httpRequest, type IncomingMessage, type ServerResponse } from "node:http";
import { afterEach, expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { createDemoAdapter } from "../demo/demo-adapter";
import { createHttpUsers } from "./users";
import { createCsrfClient } from "./csrf-client";
import type { UsersApi } from "../api";

// happy-dom's real-network Response clone loses the body; use Node's HTTP sender
// while exercising the complete adapter, CSRF and response decoder against a real receiver.
const wireFetch: typeof fetch = async (input, init) => new Promise<Response>((resolve, reject) => {
  const request = httpRequest(String(input), { method: init?.method ?? "GET", headers: Object.fromEntries(new Headers(init?.headers)) }, (response) => {
    const chunks: Buffer[] = [];
    response.on("data", (chunk: Buffer) => chunks.push(chunk));
    response.on("end", () => resolve(new Response(Buffer.concat(chunks).toString(), { status: response.statusCode, headers: response.headers as Record<string, string> })));
    response.on("error", reject);
  });
  request.on("error", reject);
  if (init?.body) request.write(String(init.body));
  request.end();
});

const closes: (() => Promise<void>)[] = [];
afterEach(async () => { await Promise.all(closes.splice(0).map((close) => close())); });
async function server(handler: (request: IncomingMessage, response: ServerResponse) => void) {
  const s = createServer((request, response) => {
    response.setHeader("Access-Control-Allow-Origin", request.headers.origin ?? "*");
    response.setHeader("Access-Control-Allow-Credentials", "true");
    response.setHeader("Access-Control-Allow-Headers", "X-CSRF-Token, Content-Type, Accept");
    response.setHeader("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS");
    response.setHeader("Access-Control-Expose-Headers", "X-CSRF-Token");
    if (request.method === "OPTIONS") { response.writeHead(204); response.end(); return; }
    handler(request, response);
  });
  await new Promise<void>((resolve) => s.listen(0, "127.0.0.1", resolve));
  closes.push(() => new Promise<void>((resolve, reject) => s.close((error) => error ? reject(error) : resolve())));
  const address = s.address();
  if (!address || typeof address === "string") throw new Error("test.address");
  return `http://127.0.0.1:${address.port}/api`;
}
const project = { id: "app", name: "Assigned", path: "/srv/stacks/app", isArchived: false, status: "running", serviceCount: 1, runningCount: 1, tags: [], createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z" };

async function fixture(level: "restricted" | "full") {
  const seen: string[] = [];
  const baseUrl = await server((request, response) => {
    const url = new URL(request.url!, "https://hub.example.com"); seen.push(url.pathname);
    const send = (data: unknown, list = false, status = 200) => { response.writeHead(status, { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") }); response.end(JSON.stringify({ success: status === 200, data, ...(list ? { pagination: { totalItems: Array.isArray(data) ? data.length : 0 } } : {}) })); };
    if (url.pathname === "/api/auth/me") return send({ id: "viewer", username: "viewer", requiresPasswordChange: false, isAdmin: false, assignments: [{ environmentId: "host", projectId: "app", level }] });
    if (url.pathname === "/api/environments") return send([{ id: "host", name: "Test host", color: "#336699", status: "online" }], true);
    if (url.pathname === "/api/capabilities") return send(["listHosts", "listStacks", "getStack"].map((operationId) => ({ operationId, resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") }, available: true })));
    if (url.pathname.endsWith("/projects")) return send([project], true);
    if (url.pathname.endsWith("/runtime")) return send({ ...project, runtimeServices: [{ name: "web", image: "example:1.0.0", status: "running", containerId: "owned" }] });
    if (url.pathname.endsWith("/updates")) return send(project);
    if (url.pathname.endsWith("/compose")) return send({ ...project, composeContent: "services: {}", envContent: "TZ=UTC" });
    if (url.pathname.endsWith("/projects/app")) return send(project);
    // The host has both foreign-stack and standalone containers. A user must never request them.
    if (url.pathname.endsWith("/containers")) return send(["foreign", "standalone"].map((id) => ({ id, names: [id], state: "running", status: "Up", image: "example:1.0.0", labels: {}, ports: [] })), true);
    return send(null, false, 404);
  });
  const fetcher = vi.fn(wireFetch);
  const adapter = createHttpAdapter({ baseUrl, fetch: fetcher });
  await adapter.getCurrentUser();
  return { adapter, seen };
}

it("W1 builds a user's list and runtime detail over real HTTP without host container inventory", async () => {
  const f = await fixture("restricted");
  const hosts = await f.adapter.listHosts();
  expect(hosts[0]?.name).toBe("Test host");
  expect((await f.adapter.listStacks()).map((s) => s.id)).toEqual(["host~app"]);
  const stack = await f.adapter.getStack("host~app");
  expect(stack.services.map((s) => s.containerId)).toEqual(["host~owned"]);
  expect(f.seen.some((p) => p.endsWith("/containers"))).toBe(false);
  expect(f.seen.some((p) => p.endsWith("/runtime"))).toBe(true);
  await expect(f.adapter.getStack("host~foreign")).rejects.toThrow("not-found");
});

it("keeps a user's reachable host when another assigned host fails", async () => {
  const baseUrl = await server((request, response) => {
    const url = new URL(request.url!, "https://hub.example.com");
    const send = (data: unknown, list = false, status = 200) => { response.writeHead(status, { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") }); response.end(JSON.stringify(status === 200 ? { success: true, data, ...(list ? { pagination: { totalItems: Array.isArray(data) ? data.length : 0 } } : {}) } : { code: "agent.unavailable" })); };
    if (url.pathname === "/api/auth/me") return send({ id: "viewer", username: "viewer", requiresPasswordChange: false, isAdmin: false, assignments: [{ environmentId: "host", projectId: "app", level: "full" }, { environmentId: "down", projectId: "other", level: "full" }] });
    if (url.pathname === "/api/environments") return send([{ id: "host", name: "Test host", color: "#336699", status: "online" }, { id: "down", name: "Down host", color: "#663399", status: "offline" }], true);
    if (url.pathname === "/api/capabilities") return send(["listHosts", "listStacks", "getStack"].map((operationId) => ({ operationId, resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") }, available: true })));
    if (url.pathname.startsWith("/api/environments/down/")) return send(null, false, 503);
    if (url.pathname.endsWith("/projects")) return send([project], true);
    return send(null, false, 404);
  });
  const adapter = createHttpAdapter({ baseUrl, fetch: vi.fn(wireFetch) });
  await adapter.getCurrentUser();
  expect((await adapter.listStacks()).map((s) => s.id)).toEqual(["host~app"]);
  expect((await adapter.listHosts()).find((h) => h.id === "down")?.inventory?.stale).toBe(true);
});

it.each(["restricted", "full"] as const)("W2 fetches Compose only for Full (%s)", async (level) => {
  const f = await fixture(level);
  await f.adapter.getStack("host~app");
  expect(f.seen.some((p) => p.endsWith("/compose"))).toBe(level === "full");
  expect(f.seen.some((p) => p.endsWith("/updates"))).toBe(level === "full");
});

it.each(["demo", "http"] as const)("implements the same user and key port on %s", async (kind) => {
  const demo = createDemoAdapter();
  let api: UsersApi = demo;
  const requests: { method: string; path: string }[] = [];
  if (kind === "http") {
    const baseUrl = await server((request, response) => {
      let raw = "";
      request.on("data", (chunk: string) => { raw += chunk; });
      request.on("end", () => { void (async () => {
        const url = new URL(request.url!, "https://hub.example.com"), path = url.pathname.replace("/api", ""), method = request.method!;
        requests.push({ method, path });
        response.setHeader("Content-Type", "application/json"); response.setHeader("X-CSRF-Token", "example".padEnd(43, "x"));
        const input = raw ? JSON.parse(raw) : {};
        let data: unknown;
        if (path === "/auth/mode") data = { mode: "local" };
        else if (path === "/users" && method === "GET") data = await demo.listUsers();
        else if (path === "/users") data = await demo.createUser(input);
        else if (path === "/auth/me/api-keys" && method === "GET") data = await demo.listMyApiKeys();
        else if (path === "/auth/me/api-keys") data = await demo.createMyApiKey(input);
        else if (path.startsWith("/auth/me/api-keys/")) data = await demo.deleteMyApiKey(decodeURIComponent(path.split("/").at(-1)!));
        else {
          const parts = path.split("/"), id = decodeURIComponent(parts[2]!), action = parts[3];
          if (action === "administrator") data = await demo.setUserAdministrator(id, input.isAdmin);
          else if (action === "app-assignments") data = await demo.setUserAppAssignments(id, input.assignments);
          else if (action === "disable") data = await demo.disableUser(id);
          else if (action === "enable") data = await demo.enableUser(id);
          else if (action === "reset-password") data = await demo.resetUserPassword(id, input.password);
          else if (method === "DELETE") data = await demo.deleteUser(id);
          else data = await demo.updateUser(id, input);
        }
        response.end(JSON.stringify({ success: true, data: data ?? { message: "OK" }, ...(path === "/users" && method === "GET" ? { pagination: { totalItems: (data as unknown[]).length } } : {}) }));
      })().catch(() => { response.statusCode = 409; response.end(JSON.stringify({ success: false, code: "last-admin" })); }); });
    });
    api = createHttpUsers(baseUrl, createCsrfClient(baseUrl, wireFetch));
  }
  const admin = (await api.listUsers())[0]!;
  await expect(api.disableUser(admin.id)).rejects.toBeDefined();
  await expect(api.deleteUser(admin.id)).rejects.toBeDefined();
  await expect(api.setUserAdministrator(admin.id, false)).rejects.toBeDefined();
  const user = await api.createUser({ username: "member@example.com", password: "example".repeat(3) });
  expect(user.requiresPasswordChange).toBe(true);
  await api.updateUser(user.id, { username: "member" });
  await api.setUserAppAssignments(user.id, [{ environmentId: "host", projectId: "app", level: "restricted" }, { environmentId: "host", projectId: "other", level: "full" }]);
  await api.setUserAdministrator(user.id, true);
  await api.disableUser(user.id); expect((await api.listUsers()).find((u) => u.id === user.id)?.disabledAt).toBeDefined();
  await api.enableUser(user.id); await api.resetUserPassword(user.id, "example-reset-123");
  expect((await api.listUsers()).find((u) => u.id === user.id)?.assignments.map((a) => a.level)).toEqual(["restricted", "full"]);
  const key = await api.createMyApiKey({ name: "Example", expiresAt: "2027-01-01T00:00:00Z" });
  expect(key.key.length).toBeGreaterThan(12);
  const keys = await api.listMyApiKeys(); expect(keys[0]?.expiresAt).toBe("2027-01-01T00:00:00Z"); expect("key" in keys[0]!).toBe(false);
  await api.deleteMyApiKey(key.id); expect(await api.listMyApiKeys()).toEqual([]);
  await api.deleteUser(user.id);
  if (kind === "http") expect(requests.some((r) => r.path.endsWith("/administrator") && r.method === "PUT")).toBe(true);
});
