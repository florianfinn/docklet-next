// Source: docklet-next/issues/340
// Revision: ac06d647bfce487c7a04398db744de24a4dd3de3
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original tests of the logout admission at physical retries, socket handshakes and public read errors (M17 acceptance M17-A2, review M17-AR3/AR4/AR5).

import { CancelledError } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { deferred } from "./activity-reader.test-support";
import { createHttpAdapter } from "./http-adapter";
import { envelope } from "./runtime-commands.test-support";
import { sessionFixture } from "./session-lifecycle.test-support";
import { Socket } from "./shell.test-support";

const settle = async () => { for (let i = 0; i < 100; i++) await Promise.resolve(); };
const infrastructure = (f: ReturnType<typeof sessionFixture>) => f.calls.filter((c) => !c.path.includes("/auth/")).length;
const failure = () => new Response(JSON.stringify({ code: "http.error" }), { status: 500 });
const widget = { kind: "plex", stale: false, ageSeconds: 0, plex: { streams: 1, transcodes: 0, sessions: [] } };
afterEach(() => { vi.useRealTimers(); });

function retryFixture() {
  const logout = deferred<Response>(), reached = deferred<void>();
  let reads = 0;
  let reject!: (error: unknown) => void;
  const f = sessionFixture((url) => {
    if (url.pathname.endsWith("/auth/logout")) return logout.promise;
    if (url.pathname.endsWith("/service-widgets/plex")) {
      if (++reads === 1) { reached.resolve(); return new Promise<Response>((_resolve, fail) => { reject = fail; }); }
      return new Response(JSON.stringify(widget));
    }
  });
  return { f, logout, reached, reads: () => reads, lose: () => reject(new TypeError("Example connection reset")) };
}

it("holds a transport retry during a logout and cancels it once signed out", async () => {
  const r = retryFixture();
  await r.f.adapter.getCurrentUser();
  const running = r.f.adapter.getServiceWidget("plex").catch((error: unknown) => error);
  await r.reached.promise;
  const pending = r.f.adapter.logout();
  await settle();
  r.lose();
  await settle();
  expect(r.reads()).toBe(1);
  r.logout.resolve(envelope({}));
  await pending;
  expect(await running instanceof CancelledError).toBe(true);
  expect(r.reads()).toBe(1);
});

it("lets a held transport retry run when the logout fails", async () => {
  const r = retryFixture();
  await r.f.adapter.getCurrentUser();
  const running = r.f.adapter.getServiceWidget("plex");
  await r.reached.promise;
  const pending = r.f.adapter.logout().catch(() => "failed");
  await settle();
  r.lose();
  await settle();
  expect(r.reads()).toBe(1);
  r.logout.resolve(failure());
  expect(await pending).toBe("failed");
  expect((await running).kind).toBe("plex");
  expect(r.reads()).toBe(2);
});

it("ends held and later reads after a sign-out as cancellation without a request", async () => {
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const pending = f.adapter.logout();
  await settle();
  const outcome = (read: Promise<unknown>) => read.then(() => "read", (error: unknown) => error instanceof CancelledError ? "cancelled" : "failed");
  const held = [f.adapter.getServiceWidget("plex"), f.adapter.listHosts(), f.adapter.getCapacity()].map(outcome);
  await settle();
  logout.resolve(envelope({}));
  await pending;
  expect(await Promise.all(held)).toEqual(["cancelled", "cancelled", "cancelled"]);
  const later = [f.adapter.getServiceWidget("plex"), f.adapter.listHosts(), f.adapter.getCapacity()].map(outcome);
  expect(await Promise.all(later)).toEqual(["cancelled", "cancelled", "cancelled"]);
  expect(infrastructure(f)).toBe(0);
  expect((await f.adapter.getAuthConfiguration()).mode).toBe("local");
});

it("opens no log socket during a logout and connects the channel once the logout failed", async () => {
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const first = f.adapter.subscribeContainerLogs("0~example", () => {});
  await settle();
  expect(f.sockets).toHaveLength(1);
  first();
  const pending = f.adapter.logout().catch(() => undefined);
  await settle();
  // The capability is cached, so only the handshake itself can reach the hub.
  const off = f.adapter.subscribeContainerLogs("0~example", () => {});
  await settle();
  expect(f.sockets).toHaveLength(1);
  logout.resolve(failure());
  await pending;
  await settle();
  expect(f.sockets).toHaveLength(2);
  off();
});

it("replaces no lost log or metrics socket while a logout runs or after it signed out", async () => {
  vi.useFakeTimers();
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const offLogs = f.adapter.subscribeContainerLogs("0~example", () => {});
  const offMetrics = f.adapter.subscribeMetrics(() => {});
  await settle();
  expect(f.sockets).toHaveLength(2);
  const pending = f.adapter.logout();
  await settle();
  for (const { socket } of f.sockets) socket.disconnect();
  await settle();
  await vi.advanceTimersByTimeAsync(6_000);
  expect(f.sockets).toHaveLength(2);
  logout.resolve(envelope({}));
  await pending;
  await vi.advanceTimersByTimeAsync(6_000);
  expect(f.sockets).toHaveLength(2);
  offLogs(); offMetrics();
});

it("reconnects a metrics socket lost during a failed logout", async () => {
  vi.useFakeTimers();
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const off = f.adapter.subscribeMetrics(() => {});
  await settle();
  expect(f.sockets).toHaveLength(1);
  const pending = f.adapter.logout().catch(() => undefined);
  await settle();
  f.sockets[0]!.socket.disconnect();
  await vi.advanceTimersByTimeAsync(5_000);
  expect(f.sockets).toHaveLength(1);
  logout.resolve(failure());
  await pending;
  await vi.advanceTimersByTimeAsync(5_000);
  expect(f.sockets).toHaveLength(2);
  off();
});

it("opens no shell whose capability read completes during a logout", async () => {
  const capability = deferred<Response>(), reached = deferred<void>(), logout = deferred<Response>();
  const sockets: Socket[] = [];
  const adapter = createHttpAdapter({
    fetch: (async (input: RequestInfo | URL) => {
      const path = new URL(String(input), "https://hub.example.com").pathname;
      if (path.endsWith("/auth/me")) return envelope({ id: "example-user", username: "operator", requiresPasswordChange: false, isAdmin: true });
      if (path.endsWith("/auth/mode")) return envelope({ mode: "local" });
      if (path.endsWith("/auth/logout")) return logout.promise;
      if (path.endsWith("/capabilities")) { reached.resolve(); return capability.promise; }
      throw new TypeError("Example unexpected request");
    }) as typeof fetch,
    shellWebSocket: () => { const socket = new Socket(); sockets.push(socket); return socket; },
  });
  await adapter.getCurrentUser();
  const shell = adapter.openContainerShell("0~example", { shell: "auto" }, () => {});
  await reached.promise;
  const pending = adapter.logout();
  await settle();
  capability.resolve(envelope([{ operationId: "openContainerShell", resourceRef: { kind: "container", id: "0~example" }, available: true }]));
  await settle();
  expect(sockets).toHaveLength(0);
  logout.resolve(envelope({}));
  await pending;
  await settle();
  expect(sockets).toHaveLength(0);
  shell.close();
});

it("signs out when a read is still bootstrapping the CSRF token as the logout starts", async () => {
  const f = sessionFixture();
  const read = f.adapter.listHosts().then(() => "listed", () => "ended");
  const out = f.adapter.logout().then(() => "out", () => "failed");
  await settle();
  expect(await Promise.race([out, settle().then(() => "stuck")])).toBe("out");
  // Its later requests start during the logout, so the read ends with the session instead of waiting forever.
  expect(await read).toBe("ended");
});
