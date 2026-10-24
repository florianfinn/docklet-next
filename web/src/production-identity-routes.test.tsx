// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original production runtime route coverage for M4 identity.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { RouteObject } from "react-router";
import { afterAll, afterEach, beforeAll, expect, it, vi } from "vitest";

vi.mock("@/runtime", () => vi.importActual("./runtime"));

import type { renderRoutes } from "@/test/render";
import type { createAdapter } from "./runtime";

let routes: RouteObject[];
let renderProductRoutes: typeof renderRoutes;
let productAdapter: typeof createAdapter;

// Cold router imports use a generous hang timeout so shared CI load does not fail setup.
beforeAll(async () => {
  vi.stubEnv("DEV", false);
  vi.resetModules();
  ({ routes } = await import("@/app/router"));
  ({ renderRoutes: renderProductRoutes } = await import("@/test/render"));
  ({ createAdapter: productAdapter } = await import("./runtime"));
}, 60_000);

afterAll(() => vi.unstubAllEnvs());
afterEach(() => { cleanup(); localStorage.clear(); });

it("runs local sign-in, first setup, connected settings and profile on production routes", async () => {
  let signedIn = false;
  let passwordChanged = false;
  const settings: Record<string, string> = {
    setupCompleted: "false", hubName: "Docklet", hubAddress: "", domainUrl: "", linkBase: "auto", effectiveTimeZone: "Europe/Berlin",
    keepActionOutput: "true", actionPresentation: "dialog",
  };
  const user = { id: "admin", username: "operator", requiresPasswordChange: true, locale: "de" };
  const writes: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/me") return signedIn ? new Response(JSON.stringify({ success: true, data: user })) : new Response("{}", { status: 401 });
    if (path === "/api/auth/login") {
      signedIn = true;
      writes.push("login");
      return new Response(JSON.stringify({ success: true, data: { user } }));
    }
    if (path === "/api/auth/password") {
      passwordChanged = true;
      user.requiresPasswordChange = false;
      writes.push("password");
      return new Response(JSON.stringify({ success: true, data: { message: "changed" } }));
    }
    if (path === "/api/auth/me/profile") {
      const input = JSON.parse(String(init?.body)) as { locale: string };
      user.locale = input.locale;
      writes.push("profile");
      return new Response(JSON.stringify({ success: true, data: user }));
    }
    if (path === "/api/environments/0/settings") {
      if (init?.method === "PUT") {
        Object.assign(settings, JSON.parse(String(init.body)) as Record<string, string>);
        writes.push("settings");
        return new Response(JSON.stringify({ success: true, data: [] }));
      }
      return new Response(JSON.stringify(Object.entries(settings).map(([key, value]) => ({ key, value, type: "text" }))));
    }
    return new Response(JSON.stringify({ code: "unavailable" }), { status: 503 });
  });
  const adapter = productAdapter({ fetch: fetcher as typeof fetch, eventSource: () => ({ close() {}, onopen: null, onmessage: null, onerror: null }) });
  const { router } = renderProductRoutes(routes, ["/dashboard"], adapter);
  await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
  await userEvent.type(await screen.findByLabelText("Benutzername"), "operator");
  await userEvent.type(screen.getByLabelText("Passwort"), "example-password");
  await userEvent.click(screen.getByRole("button", { name: "Anmelden" }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/setup"));
  await userEvent.type(await screen.findByLabelText("Interne Hub-Adresse"), "192.0.2.10:8080");
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await screen.findByLabelText("Standard-Projektverzeichnis");
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.type(await screen.findByLabelText("Bisheriges Administratorpasswort"), "example-password");
  await userEvent.type(screen.getByLabelText("Neues Administratorpasswort"), "New-password1!");
  await userEvent.type(screen.getByLabelText("Neues Passwort bestätigen"), "New-password1!");
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.click(await screen.findByRole("button", { name: "Einrichtung abschließen" }));
  await waitFor(() => expect(settings.setupCompleted).toBe("true"));
  expect(passwordChanged).toBe(true);
  await userEvent.click(await screen.findByRole("link", { name: "Zum Dashboard" }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
  expect(document.querySelector('[data-slot="sidebar"] a[href="/dashboard"]') !== null).toBe(true);
  await router.navigate("/settings/general");
  expect((await screen.findByLabelText("Name des Hubs")) !== null).toBe(true);
  await router.navigate("/settings/auth");
  expect((await screen.findByText(/AUTH_MODE/)) !== null).toBe(true);
  await router.navigate("/settings/activity");
  expect((await screen.findByRole("switch", { name: /Ausgaben von Aktionen speichern/ })) !== null).toBe(true);
  await router.navigate("/profile");
  expect((await screen.findByRole("heading", { name: "Profil" })) !== null).toBe(true);
  expect(writes).toEqual(["login", "password", "settings"]);
}, 30_000);
