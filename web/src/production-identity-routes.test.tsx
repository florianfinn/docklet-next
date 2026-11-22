// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original production runtime route coverage for M4 identity; optional-module foundation (#472); M29 first-run claim instead of the initial password change.

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

it("claims a pending hub, runs first setup, connected settings and profile on production routes", async () => {
  let pending = true;
  const settings: Record<string, string> = {
    moduleProxyEnabled: "false", setupCompleted: "false", hubName: "Docklet", hubAddress: "", domainUrl: "", linkBase: "auto", effectiveTimeZone: "Europe/Berlin",
    keepActionOutput: "true", actionPresentation: "dialog", defaultHostConnection: "direct", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true",
  };
  const user = { id: "admin", username: "operator", requiresPasswordChange: false, isAdmin: true, assignments: [], locale: "de" };
  const writes: string[] = [];
  const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/me/api-keys") return json([]);
    if (path === "/api/auth/mode") return json({ mode: "local", setupPending: pending, forced: false });
    if (path === "/api/auth/sign-in") return json({ mode: "local", setupPending: false, forced: false, proxyHeader: "Remote-User", trustedProxies: "" });
    if (path === "/api/auth/me") return pending ? new Response(JSON.stringify({ status: 401, code: "setup.pending" }), { status: 401, headers: { "Content-Type": "application/problem+json" } }) : json(user);
    if (path === "/api/setup/identity") {
      pending = false;
      writes.push("claim");
      return json({ success: true, status: "authenticated", user });
    }
    if (path === "/api/auth/me/profile") {
      const input = JSON.parse(String(init?.body)) as { locale: string };
      user.locale = input.locale;
      writes.push("profile");
      return json(user);
    }
    if (path === "/api/environments/0/settings/public") return new Response(JSON.stringify([{key:"moduleProxyEnabled",value:settings.moduleProxyEnabled}]));
    if (path === "/api/environments/0/settings") {
      if (init?.method === "PUT") {
        Object.assign(settings, JSON.parse(String(init.body)) as Record<string, string>);
        writes.push("settings");
        return json([]);
      }
      return new Response(JSON.stringify(Object.entries(settings).map(([key, value]) => ({ key, value, type: "text" }))));
    }
    return new Response(JSON.stringify({ code: "unavailable" }), { status: 503 });
  });
  const adapter = productAdapter({ fetch: fetcher as typeof fetch, eventSource: () => ({ close() {}, onopen: null, onmessage: null, onerror: null }) });
  const { router } = renderProductRoutes(routes, ["/dashboard"], adapter);
  await waitFor(() => expect(router.state.location.pathname).toBe("/setup"));
  await userEvent.type(await screen.findByLabelText("Benutzername des Administrators"), "operator");
  await userEvent.type(screen.getByLabelText("Neues Passwort"), "New-password1!");
  await userEvent.type(screen.getByLabelText("Passwort wiederholen"), "New-password1!");
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  await userEvent.type(await screen.findByLabelText("IP-Adresse"), "192.0.2.10:8080");
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await screen.findByLabelText("Standard-Projektverzeichnis");
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  expect((await screen.findByRole("switch",{name:"Proxy"})).getAttribute("aria-checked")).toBe("false");
  await userEvent.click(screen.getByRole("button",{name:"Weiter"}));
  await userEvent.click(await screen.findByRole("button", { name: "Überspringen" }));
  await userEvent.click(await screen.findByRole("button", { name: "Einrichtung abschließen" }));
  await waitFor(() => expect(settings.setupCompleted).toBe("true"));
  await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"), { timeout: 5_000 });
  await waitFor(() => expect(document.querySelector('[data-slot="sidebar"] a[href="/dashboard"]') !== null).toBe(true), { timeout: 5_000 });
  await router.navigate("/settings/general");
  expect((await screen.findByLabelText("Name des Hubs")) !== null).toBe(true);
  await router.navigate("/settings/auth");
  expect((await screen.findByRole("button", { name: "Anmeldeart übernehmen" })) !== null).toBe(true);
  await router.navigate("/settings/activity");
  expect((await screen.findByRole("switch", { name: /Ausgaben von Aktionen speichern/ })) !== null).toBe(true);
  await router.navigate("/profile");
  expect((await screen.findByRole("heading", { name: "Profil" })) !== null).toBe(true);
  expect(writes).toEqual(["claim", "settings"]);
}, 30_000);
