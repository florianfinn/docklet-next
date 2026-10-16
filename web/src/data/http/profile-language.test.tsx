// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original persisted administrator locale route test.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { AuthGate } from "@/app/AuthGate";
import { localeStore, setAppLocale } from "@/app/i18n/locale-store";
import ProfilePage from "@/features/settings/ProfilePage";
import { renderRoutes } from "@/test/render";

import { createHttpAdapter } from "./http-adapter";

afterEach(() => { localeStore.reset(); localStorage.clear(); });

it("loads the administrator language from the core and saves a change with CSRF", async () => {
  const current = { id: "admin", username: "operator", requiresPasswordChange: false, locale: "en-US" };
  const writes: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/me") return new Response(JSON.stringify({ success: true, data: current }));
    if (path === "/api/auth/me/profile") {
      expect(init?.method).toBe("PUT");
      expect(new Headers(init?.headers).get("X-CSRF-Token")).toBe("example".padEnd(43, "x"));
      const body = JSON.parse(String(init?.body)) as { locale: string };
      writes.push(body.locale);
      current.locale = body.locale;
      return new Response(JSON.stringify({ success: true, data: current }));
    }
    if (path === "/api/environments/0/settings") return new Response(JSON.stringify([
      { key: "setupCompleted", value: "true", type: "boolean" },
      { key: "timeZone", value: "Europe/Berlin", type: "text" },
    ]));
    throw new Error(`unexpected ${path}`);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch });
  const routes = [{ element: <AuthGate />, children: [{ path: "/profile", element: <ProfilePage /> }] }, { path: "/login", element: <p>Login</p> }];
  const first = renderRoutes(routes, ["/profile"], adapter);
  expect((await screen.findByRole("heading", { name: "Profile" })) !== null).toBe(true);
  expect(localeStore.get()).toBe("en-US");
  await userEvent.click(screen.getByRole("radio", { name: "Deutsch" }));
  await waitFor(() => expect(writes).toEqual(["de"]));
  expect((await screen.findByRole("heading", { name: "Profil" })) !== null).toBe(true);
  first.unmount();

  setAppLocale("en-US");
  renderRoutes(routes, ["/profile"], createHttpAdapter({ fetch: fetcher as typeof fetch }));
  await waitFor(() => expect(localeStore.get()).toBe("de"));
  expect((await screen.findByRole("heading", { name: "Profil" })) !== null).toBe(true);
});
