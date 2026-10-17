// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original connected first-run flow contract test.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { toast } from "sonner";

import { AuthGate } from "@/app/AuthGate";
import { SetupGate } from "@/app/SetupGate";
import { renderRoutes } from "@/test/render";
import { restartSetup } from "@/features/setup/setup-draft";

import { createHttpAdapter } from "./http-adapter";

afterEach(() => { restartSetup(); localStorage.clear(); });

it.each(["/dashboard", "/settings/general", "failure"])("stores host defaults separately and localizes setup write errors (fail=%s)", async (target) => {
  const fail = target === "failure";
  const errorToast = vi.spyOn(toast, "error");
  restartSetup();
  const settings: Record<string, string> = { setupCompleted: "false" };
  let changedPassword = false;
  const writes: Record<string, string>[] = [];
  const fetcher = async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/me") return new Response(JSON.stringify({ success: true, data: { id: "admin", username: "admin", requiresPasswordChange: !changedPassword } }));
    if (path === "/api/auth/password") {
      const body = JSON.parse(String(init?.body)) as { currentPassword: string; newPassword: string };
      expect(body.currentPassword).toBe("old-password");
      expect(body.newPassword).toBe("New-password1!");
      changedPassword = true;
      return new Response(JSON.stringify({ success: true, data: { message: "changed" } }));
    }
    if (path === "/api/environments/0/settings") {
      if (init?.method === "PUT") {
        expect(changedPassword).toBe(true);
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
  const { router } = renderRoutes([{ path: "/setup", element: <SetupGate /> }, { element: <AuthGate />, children: [{ path: "/dashboard", element: <p>Dashboard target</p> }, { path: "/settings/general", element: <p>Settings target</p> }] }, { path: "/login", element: <p>Login</p> }], ["/setup"], adapter);
  const next = () => userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.type(await screen.findByLabelText("Interne Hub-Adresse"), "192.0.2.10:8080");
  await next();
  await screen.findByLabelText("Standard-Projektverzeichnis");
  await next();
  await userEvent.type(await screen.findByLabelText("Bisheriges Administratorpasswort"), "old-password");
  await userEvent.type(screen.getByLabelText("Neues Administratorpasswort"), "New-password1!");
  await userEvent.type(screen.getByLabelText("Neues Passwort bestätigen"), "New-password1!");
  await next();
  await userEvent.click(await screen.findByRole("button", { name: "Einrichtung abschließen" }));
  if (fail) await waitFor(() => expect(errorToast).toHaveBeenCalledWith("Die Einrichtung konnte nicht gespeichert werden. Prüfe die Angaben und versuche es erneut."));
  else await waitFor(() => expect(settings.setupCompleted).toBe("true"));
  expect(writes[0]?.agentProjectsDirectory).toBe("/home/docker");
  expect(writes[0]?.projectsDirectory).toBeUndefined();
  if (!fail) {
    await userEvent.click(await screen.findByRole("link", { name: target === "/dashboard" ? "Zum Dashboard" : "Hub-Einstellungen ansehen" }));
    await screen.findByText(target === "/dashboard" ? "Dashboard target" : "Settings target");
    expect(router.state.location.pathname).toBe(target);
  }
  errorToast.mockRestore();
  expect(changedPassword).toBe(true);
  expect(writes).toHaveLength(1);
  expect(writes[0]?.timeZone).toBe("Europe/Berlin");
  expect(JSON.stringify(writes).includes("old-password") || JSON.stringify(writes).includes("New-password1!")).toBe(false);
});
