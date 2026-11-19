// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original connected first-run flow contract test; optional-module foundation (#472); M29 setup without the initial password step, completion opens the dashboard.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { toast } from "sonner";

import { AuthGate } from "@/app/AuthGate";
import { SetupGate } from "@/app/SetupGate";
import { renderRoutes } from "@/test/render";
import SettingsPage from "@/features/settings/SettingsPage";
import { restartSetup } from "@/features/setup/setup-draft";

import { createHttpAdapter } from "./http-adapter";

afterEach(() => { restartSetup(); localStorage.clear(); });

it.each(["/dashboard", "/settings/general", "failure"])("stores host defaults separately and localizes setup write errors (then=%s)", async (target) => {
  const fail = target === "failure";
  const errorToast = vi.spyOn(toast, "error");
  restartSetup();
  const settings: Record<string, string> = { setupCompleted: "false", hubName: "Before setup", hubAddress: "", domainUrl: "", linkBase: "auto", effectiveTimeZone: "Europe/Berlin", actionPresentation: "dialog", keepActionOutput: "true", moduleProxyEnabled: "false", defaultHostConnection: "direct", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true" };
  const writes: Record<string, string>[] = [];
  const fetcher = async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local", setupPending: false } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/me") return new Response(JSON.stringify({ success: true, data: { id: "admin", username: "admin", requiresPasswordChange: false, isAdmin: true, assignments: [] } }));
    if (path === "/api/environments/0/settings/public") return new Response(JSON.stringify([{ key: "moduleProxyEnabled", value: settings.moduleProxyEnabled }]));
    if (path === "/api/environments/0/settings") {
      if (init?.method === "PUT") {
        const values = JSON.parse(String(init.body)) as Record<string, string>;
        writes.push(values);
        if (fail) return new Response(JSON.stringify({ detail: "Not writable" }), { status: 400 });
        Object.assign(settings, values);
        return new Response(JSON.stringify({ success: true, data: [] }));
      }
      return new Response(JSON.stringify(Object.entries(settings).map(([key, value]) => ({ key, value, type: "text" }))));
    }
    throw new Error(`unexpected ${path}`);
  };
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => ({ close() {}, onopen: null, onmessage: null, onerror: null }) });
  const { router } = renderRoutes([{ path: "/setup", element: <SetupGate /> }, { element: <AuthGate />, children: [{ path: "/dashboard", element: <p>Dashboard target</p> }, { path: "/settings/:section", element: <SettingsPage /> }] }, { path: "/login", element: <p>Login</p> }], ["/setup"], adapter);
  const next = () => userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.type(await screen.findByLabelText("IP-Adresse"), "192.0.2.10:8080");
  const name = screen.getByLabelText("Name des Hubs");
  await userEvent.clear(name);
  await userEvent.type(name, "Setup Hub");
  await userEvent.type(screen.getByLabelText("Domain"), "https://docklet.example.com");
  await next();
  await screen.findByLabelText("Standard-Projektverzeichnis");
  await next();
  const moduleSwitch=await screen.findByRole("switch",{name:"Proxy"});
  expect(moduleSwitch.getAttribute("aria-checked")).toBe("false");
  if(target!=="/settings/general") await userEvent.click(moduleSwitch);
  await userEvent.click(screen.getByRole("button",{name:"Zurück"}));
  await next();
  expect(screen.getByRole("switch",{name:"Proxy"}).getAttribute("aria-checked")).toBe(String(target!=="/settings/general"));
  await next();
  await userEvent.click(await screen.findByRole("button", { name: "Einrichtung abschließen" }));
  if (fail) await waitFor(() => expect(errorToast).toHaveBeenCalledWith("Die Einrichtung konnte nicht gespeichert werden. Prüfe die Angaben und versuche es erneut."));
  else await waitFor(() => expect(settings.setupCompleted).toBe("true"));
  expect(writes[0]?.moduleProxyEnabled).toBe(String(target!=="/settings/general"));
  expect(writes[0]?.agentProjectsDirectory).toBe("/srv/projects");
  expect(writes[0]?.projectsDirectory).toBeUndefined();
  if (fail) expect(router.state.location.pathname).toBe("/setup");
  else {
    // Completion shows the preparing screen once, then opens the dashboard.
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"), { timeout: 5_000 });
    await screen.findByText("Dashboard target");
    if (target === "/settings/general") {
      await router.navigate("/settings/general");
      await waitFor(() => expect((screen.getByLabelText("Name des Hubs") as HTMLInputElement).value).toBe("Setup Hub"));
      expect((screen.getByLabelText("IP-Adresse") as HTMLInputElement).value).toBe("192.0.2.10:8080");
      expect((screen.getByLabelText("Domain") as HTMLInputElement).value).toBe("https://docklet.example.com");
      const domain = screen.getByLabelText("Domain");
      await userEvent.clear(domain);
      await userEvent.type(domain, "https://hub.example.com");
      await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
      await screen.findByText("Alles gespeichert");
      expect(settings.hubName).toBe("Setup Hub");
      expect(settings.hubAddress).toBe("192.0.2.10:8080");
      expect(settings.domainUrl).toBe("https://hub.example.com");
    }
  }
  errorToast.mockRestore();
  expect(writes).toHaveLength(target === "/settings/general" ? 2 : 1);
  expect(writes[0]?.timeZone).toBeUndefined();
});
