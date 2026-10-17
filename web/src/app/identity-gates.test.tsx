// Source: docklet-next/docs/design/identity.md
// Revision: 5b7a533b0682154acf719fc2ddd059aed9a443c2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original route-level identity state matrix and credential-error regression.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { createHttpAdapter } from "@/data/http/http-adapter";
import type { AuthMode } from "@/domain/settings";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";
import { ConnectedProfilePage } from "@/features/settings/ConnectedProfilePage";
import { restartSetup } from "@/features/setup/setup-draft";
import { renderRoutes } from "@/test/render";

import { AuthGate } from "./AuthGate";
import { SetupGate } from "./SetupGate";

afterEach(() => { cleanup(); restartSetup(); localStorage.clear(); });

function adapter(mode: AuthMode, completed: boolean, change: boolean, authenticated: boolean) {
  return createHttpAdapter({ fetch: async (input) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/mode") return json({ mode });
    if (path === "/api/auth/me") return authenticated ? json({ id: "admin", username: "operator", requiresPasswordChange: change }) : new Response("{}", { status: 401 });
    if (path === "/api/auth/password" || path === "/api/auth/login") return new Response("{}", { status: 401 });
    if (path === "/api/environments/0/settings") return new Response(JSON.stringify(Object.entries({ setupCompleted: String(completed), hubName: "Docklet", hubAddress: "192.0.2.10:8080", domainUrl: "", linkBase: "auto", effectiveTimeZone: "UTC", keepActionOutput: "true", actionPresentation: "dialog" }).map(([key, value]) => ({ key, value }))));
    return new Response("{}", { status: 503 });
  } });
}

const states = (["none", "local", "proxy"] as const).flatMap(mode => [false, true].flatMap(completed => [false, true].flatMap(change => [false, true].map(authenticated => ({ mode, completed, change, authenticated })))));
it.each(states)("terminates both entry routes for $mode completed=$completed change=$change authenticated=$authenticated", async ({ mode, completed, change, authenticated }) => {
  for (const entry of ["/dashboard", "/setup"]) {
    const { router, unmount } = renderRoutes([
      { path: "/login", element: <ProductLoginPage /> },
      { path: "/setup", element: <SetupGate /> },
      { element: <AuthGate />, children: [{ path: "/dashboard", element: <p>Dashboard terminal</p> }] },
    ], [entry], adapter(mode, completed, change, authenticated));
    let navigations = 0;
    let key = router.state.location.key;
    const stop = router.subscribe(state => {
      if (key !== state.location.key) { key = state.location.key; navigations++; }
      if (navigations > 6) router.dispose();
    });
    const needsPassword = mode === "local" && change;
    const expectedPath = !authenticated ? mode === "local" ? "/login" : entry : needsPassword || !completed ? "/setup" : "/dashboard";
    await waitFor(() => {
      expect(navigations).toBeLessThanOrEqual(2);
      expect(router.state.location.pathname).toBe(expectedPath);
      expect(document.querySelector('[aria-busy="true"]') === null).toBe(true);
      if (authenticated) {
        if (expectedPath === "/dashboard") expect(screen.queryByText("Dashboard terminal") !== null).toBe(true);
        else if (completed && needsPassword) expect(screen.queryByLabelText("Bisheriges Administratorpasswort") !== null).toBe(true);
        else expect(screen.queryByLabelText("Interne Hub-Adresse") !== null).toBe(true);
      } else if (mode === "local") expect(screen.queryByLabelText("Benutzername") !== null).toBe(true);
      else expect(document.body.textContent).toMatch(mode === "proxy" ? /Proxy/ : /nicht/);
    });
    stop(); router.dispose(); unmount();
  }
});

it("keeps a wrong current password inline on the protected profile", async () => {
  const { router } = renderRoutes([
    { path: "/login", element: <ProductLoginPage /> },
    { element: <AuthGate />, children: [{ path: "/profile", element: <ConnectedProfilePage /> }] },
  ], ["/profile"], adapter("local", true, false, true));
  await userEvent.type(await screen.findByLabelText("Bisheriges Passwort"), "wrong-password");
  await userEvent.type(screen.getByLabelText("Neues Passwort"), "New-password1!");
  await userEvent.type(screen.getByLabelText("Passwort wiederholen"), "New-password1!");
  await userEvent.click(screen.getByRole("button", { name: "Passwort ändern" }));
  expect((await screen.findByRole("alert")).textContent).toMatch(/Passwort/);
  expect(router.state.location.pathname).toBe("/profile");
});

it("keeps a wrong login inline without broadcasting session loss", async () => {
  let losses = 0;
  const reject = () => losses++;
  window.addEventListener("docklet:unauthorized", reject);
  try {
    const current = adapter("local", true, false, true);
    await expect(current.login("operator", "wrong-password")).rejects.toMatchObject({ outcome: { status: 401 } });
    expect(losses).toBe(0);
  } finally { window.removeEventListener("docklet:unauthorized", reject); }
});
