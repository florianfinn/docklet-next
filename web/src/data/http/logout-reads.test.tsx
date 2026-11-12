// Source: docklet-next/issues/340
// Revision: 4155c8fe73dc2c562ddd41075b20e0304f879f5b
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original tests of reads and streams around a logout through the real HTTP adapter (M17 acceptance M17-A2, review M17-AR1/AR2).

import { useQueryClient } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { LogoutDialog } from "@/app/shell/LogoutDialog";
import { AdapterProvider } from "@/data/AdapterProvider";
import { queryKeys } from "@/data/query-keys";
import { ServiceWidgetBody } from "@/features/dashboard/services/ServiceWidgets";

import { deferred } from "./activity-reader.test-support";
import { envelope } from "./runtime-commands.test-support";
import { sessionFixture } from "./session-lifecycle.test-support";

const settle = async () => { for (let i = 0; i < 100; i++) await Promise.resolve(); };
const infrastructure = (f: ReturnType<typeof sessionFixture>) => f.calls.filter((c) => !c.path.includes("/auth/")).length;
afterEach(() => { cleanup(); vi.useRealTimers(); });

it("keeps a widget refresh that runs into a logout silent and its data in place", async () => {
  const held = deferred<Response>(), reached = deferred<void>(), logout = deferred<Response>();
  let reads = 0;
  let aborted = false;
  const f = sessionFixture((url, init) => {
    if (url.pathname.endsWith("/auth/logout")) return logout.promise;
    if (url.pathname.endsWith("/service-widgets/plex")) {
      if (++reads === 1) return new Response(JSON.stringify({ kind: "plex", stale: false, ageSeconds: 0, plex: { streams: 1, transcodes: 0, sessions: [] } }));
      reached.resolve();
      init.signal?.addEventListener("abort", () => { aborted = true; }, { once: true });
      return held.promise;
    }
  });
  await f.adapter.getCurrentUser();
  let client!: ReturnType<typeof useQueryClient>;
  function Capture() { client = useQueryClient(); return null; }
  const view = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter><AdapterProvider adapter={f.adapter}><Capture /><ServiceWidgetBody kind="plex" summary={{ kind: "plex", configured: true, enabled: true, openUrl: "" }} admin={true} /><LogoutDialog open={true} onOpenChange={() => {}} /></AdapterProvider></MemoryRouter></IntlProvider>);
  await waitFor(() => expect(view.container.querySelector('[data-widget-state="data"]') !== null).toBe(true));
  void client.invalidateQueries({ queryKey: queryKeys.serviceWidget("plex") });
  await reached.promise;
  fireEvent.click(screen.getByRole("button", { name: catalogs["en-US"].shell.user.logoutConfirm }));
  await settle();
  // The running refresh is not cancelled by the logout; it finishes with its answer.
  held.resolve(new Response(JSON.stringify({ kind: "plex", stale: false, ageSeconds: 0, plex: { streams: 2, transcodes: 0, sessions: [] } })));
  await settle();
  expect(aborted).toBe(false);
  expect(view.container.querySelector('[data-widget-state="stale"]') !== null).toBe(false);
  expect((view.container.textContent ?? "").includes(catalogs["en-US"].dashboard.services.readFailed)).toBe(false);
  logout.resolve(envelope({}));
  await settle();
  view.unmount();
});

it("holds reads started during a logout and cancels them once it signed out", async () => {
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const before = infrastructure(f);
  const pending = f.adapter.logout();
  await settle();
  const during = f.adapter.listHosts().then(() => "listed", () => "cancelled");
  await settle();
  expect(infrastructure(f)).toBe(before);
  logout.resolve(envelope({}));
  await pending;
  expect(await during).toBe("cancelled");
  expect(await f.adapter.listHosts().then(() => "listed", () => "cancelled")).toBe("cancelled");
  expect(infrastructure(f)).toBe(before);
  await f.adapter.login("operator", "p".repeat(12));
  expect((await f.adapter.listHosts()).map((h) => h.id)).toEqual(["0"]);
});

it("lets held reads run when the logout fails", async () => {
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const pending = f.adapter.logout().then(() => "out", () => "failed");
  await settle();
  const during = f.adapter.listHosts();
  logout.resolve(new Response(JSON.stringify({ code: "http.error" }), { status: 500 }));
  expect(await pending).toBe("failed");
  expect((await during).map((h) => h.id)).toEqual(["0"]);
});

it("treats a session lost during the logout as signed out and starts no infrastructure read", async () => {
  const reached = deferred<void>();
  const f = sessionFixture((url, init) => {
    if (url.pathname.endsWith("/auth/logout")) {
      reached.resolve();
      return new Promise<Response>((_resolve, reject) => init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true }));
    }
  });
  await f.adapter.getCurrentUser();
  const result = f.adapter.logout().then(() => "out", () => "failed");
  await reached.promise;
  // The server revoked the session before its logout response arrived; an identity check sees it.
  f.state.authenticated = false;
  await f.adapter.getCurrentUser().catch(() => undefined);
  expect(await result).toBe("out");
  const before = infrastructure(f);
  await f.adapter.listHosts().catch(() => undefined);
  expect(infrastructure(f)).toBe(before);
});

it("closes the event stream during a logout and opens no replacement until it fails", async () => {
  vi.useFakeTimers();
  const logout = deferred<Response>();
  const f = sessionFixture((url) => (url.pathname.endsWith("/auth/logout") ? logout.promise : undefined));
  await f.adapter.getCurrentUser();
  const off = f.adapter.subscribeChanges(() => {});
  expect(f.sources).toHaveLength(1);
  const pending = f.adapter.logout().catch(() => undefined);
  await settle();
  expect(f.sources[0]!.close.mock.calls.length > 0).toBe(true);
  await vi.advanceTimersByTimeAsync(5_000);
  expect(f.sources).toHaveLength(1);
  logout.resolve(new Response(JSON.stringify({ code: "http.error" }), { status: 500 }));
  await pending;
  await settle();
  expect(f.sources).toHaveLength(2);
  off();
});
