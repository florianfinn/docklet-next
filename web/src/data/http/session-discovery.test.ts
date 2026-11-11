// Source: docklet-next/issues/425
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original session presence hint discovery, 401 body completion and problem decoding proofs (#391, #425).

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CancelledError } from "@tanstack/react-query";

import { AdapterOutcomeError } from "../capability";
import { createHttpAdapter } from "./http-adapter";
import { problemCode } from "./problem-code";
import { clearSessionHint } from "./session-hint";

const token = "example".padEnd(43, "x");
const user = { id: "admin", username: "admin", requiresPasswordChange: false, isAdmin: true };
const clearCookies = () => { for (const part of document.cookie.split(";")) { const name = part.trim().split("=")[0]; if (name) document.cookie = `${name}=; Path=/; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT`; } };
beforeEach(clearCookies);
afterEach(clearCookies);

function hub(mode: "local" | "none" | "proxy", me: () => Response | Promise<Response> = () => Response.json({ success: true, data: user })) {
  const seen: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    seen.push(`${(init?.method ?? "GET").toUpperCase()} ${path}`);
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode } }), { headers: { "X-CSRF-Token": token } });
    if (path === "/api/auth/me") return me();
    if (path === "/api/auth/login") return Response.json({ success: true, data: { user } });
    throw new Error(`unexpected ${path}`);
  }) as typeof fetch;
  return { seen, adapter: createHttpAdapter({ fetch: fetcher }) };
}
const status = (error: unknown) => error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.status : undefined;

it("R2-b local mode without the hint resolves as a refused session without any session request", async () => {
  const { seen, adapter } = hub("local");
  const [mode, current] = await Promise.allSettled([adapter.getAuthMode(), adapter.getCurrentUser()]);
  expect(mode).toEqual({ status: "fulfilled", value: "local" });
  expect(current.status === "rejected" && status(current.reason)).toBe(401);
  // The mode query and session discovery share one public read.
  expect(seen).toEqual(["GET /api/auth/mode"]);
});

it.each(["none", "proxy"] as const)("R2-b %s mode probes the session without the hint", async (mode) => {
  const { seen, adapter } = hub(mode);
  expect((await adapter.getCurrentUser()).id).toBe("admin");
  expect(seen).toContain("GET /api/auth/me");
});

it("R2-b the hint makes a local-mode start probe the session", async () => {
  document.cookie = "docklet_session=1; Path=/";
  const { seen, adapter } = hub("local");
  expect((await adapter.getCurrentUser()).id).toBe("admin");
  expect(seen).toEqual(["GET /api/auth/me"]);
});

it("R2-b a hint that outlived its session gives one 401 and is deleted", async () => {
  document.cookie = "docklet_session=1; Path=/";
  const { seen, adapter } = hub("local", () => new Response(JSON.stringify({ code: "auth.unauthorized" }), { status: 401 }));
  await expect(adapter.getCurrentUser()).rejects.toSatisfy((error) => status(error) === 401);
  expect(document.cookie).not.toContain("docklet_session");
  await expect(adapter.getCurrentUser()).rejects.toSatisfy((error) => status(error) === 401);
  expect(seen.filter((line) => line === "GET /api/auth/me")).toHaveLength(1);
});

it("R2-b an anonymous start without a session read signs in with the CSRF token from the mode read", async () => {
  const { seen, adapter } = hub("local");
  await expect(adapter.getCurrentUser()).rejects.toSatisfy((error) => status(error) === 401);
  expect((await adapter.login("admin", "example-password")).id).toBe("admin");
  expect(seen).toEqual(["GET /api/auth/mode", "POST /api/auth/login"]);
});

it("R2-b after a sign-in in this document the session is probed even if no hint cookie arrived", async () => {
  const { seen, adapter } = hub("local");
  expect((await adapter.login("admin", "example-password")).id).toBe("admin");
  const current = await adapter.getCurrentUser().then((value) => value.id, (error: unknown) => status(error));
  expect(current).toBe("admin");
  expect(seen).toContain("GET /api/auth/me");
});

it("R2-b a discovery still waiting for the mode read when a login lands never reports the new session lost", async () => {
  let releaseMode!: () => void;
  const modeGate = new Promise<void>((resolve) => { releaseMode = resolve; });
  const seen: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    seen.push(path);
    if (path === "/api/auth/mode") { await modeGate; return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": token } }); }
    if (path === "/api/auth/login") return Response.json({ success: true, data: { user } });
    return Response.json({ success: true, data: user });
  }) as typeof fetch;
  const adapter = createHttpAdapter({ fetch: fetcher });
  const discovery = adapter.getCurrentUser();
  const login = adapter.login("admin", "example-password");
  releaseMode();
  expect((await login).id).toBe("admin");
  await expect(discovery).rejects.toBeInstanceOf(CancelledError);
});

it("R2-c a 401 body completes before the unauthorized event and its request is not aborted", async () => {
  document.cookie = "docklet_session=1; Path=/";
  let bodyDone = false;
  const observed: Array<{ path: string; bodyDone: boolean; aborted: boolean }> = [];
  let lastSignal: AbortSignal | undefined;
  let lastPath = "";
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": token } });
    lastSignal = init?.signal ?? undefined; lastPath = path; bodyDone = false;
    const bytes = new TextEncoder().encode(JSON.stringify({ code: "auth.unauthorized" }));
    return new Response(new ReadableStream({ pull(controller) { controller.enqueue(bytes); controller.close(); bodyDone = true; } }, { highWaterMark: 0 }), { status: 401 });
  }) as typeof fetch;
  const listener = () => observed.push({ path: lastPath, bodyDone, aborted: !!lastSignal?.aborted });
  window.addEventListener("docklet:unauthorized", listener);
  try {
    const adapter = createHttpAdapter({ fetch: fetcher });
    // An owned path (session discovery) and a non-owned read through the adapter's 401 wrapper.
    await expect(adapter.getCurrentUser()).rejects.toSatisfy((error) => status(error) === 401);
    await expect(adapter.getHubBackupStorage()).rejects.toBeDefined();
  } finally { window.removeEventListener("docklet:unauthorized", listener); }
  expect(observed.length).toBeGreaterThanOrEqual(2);
  expect(observed.every((entry) => entry.bodyDone && !entry.aborted)).toBe(true);
  expect(observed.map((entry) => entry.path)).toEqual(expect.arrayContaining(["/api/auth/me", "/api/backups/storage"]));
});

it.each([
  [{ code: "agent.unavailable" }, "agent.unavailable"],
  [{ success: false, code: "agent.unavailable", data: { error: "dial tcp" } }, "agent.unavailable"],
  [{ success: false, data: { code: "run.busy" } }, "run.busy"],
  [{ title: "Service Unavailable", status: 503, detail: "backup.maintenance" }, "backup.maintenance"],
  [{ title: "Bad Request", detail: "the request body is invalid" }, undefined],
  [null, undefined],
])("R2-d decodes %j as %s", (body, code) => {
  expect(problemCode(body)).toBe(code);
});

it("R2-b deleting the hint takes effect within the same instant", () => {
  document.cookie = "docklet_session=1; Path=/";
  const now = Date.now();
  const clock = vi.spyOn(Date, "now").mockReturnValue(now);
  try { clearSessionHint(); expect(document.cookie).not.toContain("docklet_session"); }
  finally { clock.mockRestore(); }
});
