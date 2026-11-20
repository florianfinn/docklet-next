// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original first-run claim, pending-gate and wizard hydration tests through the real HTTP adapter.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { AuthGate } from "@/app/AuthGate";
import { SetupGate } from "@/app/SetupGate";
import { createHttpAdapter } from "@/data/http/http-adapter";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";
import { renderRoutes } from "@/test/render";

import { freshSetupDraft, hydrateFromSettings, restartSetup } from "./setup-draft";

afterEach(() => { cleanup(); restartSetup(); localStorage.clear(); });

interface Hub {
  pending: boolean;
  forced?: boolean;
  mode?: "local" | "proxy" | "none";
  claims: unknown[];
  refuse?: { status: number; code: string };
  /** Holds the gates' post-claim mode refetch so the claim form stays mounted. */
  hold?: Promise<void>;
}

function hubAdapter(hub: Hub) {
  return createHttpAdapter({ fetch: async (input, init) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    const problem = (status: number, code: string) => new Response(JSON.stringify({ status, code, detail: code }), { status, headers: { "Content-Type": "application/problem+json" } });
    const admin = { id: "admin", username: "owner", requiresPasswordChange: false, isAdmin: true, assignments: [] };
    if (path === "/api/auth/mode" && !hub.pending && hub.hold) await hub.hold;
    if (path === "/api/auth/mode") return json({ mode: hub.mode ?? "local", setupPending: hub.pending, forced: hub.forced ?? false, ...(hub.pending ? { proxyHeader: "Remote-User", trustedProxies: "" } : {}) });
    if (path === "/api/setup/identity") {
      hub.claims.push(JSON.parse(String(init?.body)));
      if (hub.refuse) return problem(hub.refuse.status, hub.refuse.code);
      hub.pending = false;
      return json({ success: true, status: "authenticated", user: admin });
    }
    if (path === "/api/auth/me") return hub.pending ? problem(401, "setup.pending") : json(admin);
    if (path === "/api/environments/0/settings" || path === "/api/environments/0/settings/public") return new Response(JSON.stringify(Object.entries({ setupCompleted: "false", hubName: "Docklet", hubAddress: "", domainUrl: "", linkBase: "auto", effectiveTimeZone: "UTC", keepActionOutput: "true", actionPresentation: "dialog", defaultHostConnection: "direct", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true" }).map(([key, value]) => ({ key, value }))));
    return new Response("{}", { status: 503 });
  } });
}

const routes = [
  { path: "/login", element: <ProductLoginPage /> },
  { path: "/setup", element: <SetupGate /> },
  { element: <AuthGate />, children: [{ path: "/dashboard", element: <p>Dashboard terminal</p> }] },
];

const entries = (["local", "proxy", "none"] as const).flatMap((mode) => ["/dashboard", "/login", "/setup"].map((entry) => ({ mode, entry })));
it.each(entries)("a pending $mode hub opens the claim from $entry without a sign-in screen", async ({ mode, entry }) => {
  const { router } = renderRoutes(routes, [entry], hubAdapter({ pending: true, mode, forced: mode !== "local", claims: [] }));
  expect(await screen.findByLabelText("Benutzername des Administrators") !== null).toBe(true);
  expect(screen.queryByLabelText("Setup-Code") === null).toBe(true);
  expect(router.state.location.pathname).toBe("/setup");
  expect(screen.queryByLabelText("Benutzername") === null).toBe(true);
});

it("claims with mode and administrator, without a setup code, then continues in the wizard", async () => {
  const hub: Hub = { pending: true, claims: [], refuse: { status: 409, code: "auth.password-required" } };
  renderRoutes(routes, ["/setup"], hubAdapter(hub));
  await userEvent.type(await screen.findByLabelText("Benutzername des Administrators"), "owner");
  await userEvent.type(screen.getByLabelText("Neues Passwort"), "Example-Setup-Password1!");
  await userEvent.type(screen.getByLabelText("Passwort wiederholen"), "Example-Setup-Password1!");
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  expect((await screen.findByRole("alert")) !== null).toBe(true);

  hub.refuse = undefined;
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  await waitFor(() => expect(screen.queryByLabelText("Benutzername des Administrators") === null).toBe(true));
  expect(hub.claims.at(-1)).toEqual({ mode: "local", username: "owner", password: "Example-Setup-Password1!" });
  // The wizard continues with the hub step; the access step is complete.
  expect(await screen.findByLabelText("IP-Adresse") !== null).toBe(true);
});

it("shows no validation error while a successful claim hands over to the wizard", async () => {
  let release!: () => void;
  const hub: Hub = { pending: true, claims: [], hold: new Promise<void>((r) => { release = r; }) };
  renderRoutes(routes, ["/setup"], hubAdapter(hub));
  await userEvent.type(await screen.findByLabelText("Benutzername des Administrators"), "owner");
  await userEvent.type(screen.getByLabelText("Neues Passwort"), "Example-Setup-Password1!");
  await userEvent.type(screen.getByLabelText("Passwort wiederholen"), "Example-Setup-Password1!");
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  await waitFor(() => expect([hub.claims.length, (screen.getByLabelText("Neues Passwort") as HTMLInputElement).value]).toEqual([1, ""]));
  expect([screen.queryByText("Passwort eingeben") === null, screen.queryByText(/Passwörter stimmen nicht/) === null]).toEqual([true, true]);
  release();
  expect(await screen.findByLabelText("IP-Adresse") !== null).toBe(true);
});

it("sends no password outside local mode and needs trusted proxies for proxy mode", async () => {
  const hub: Hub = { pending: true, claims: [] };
  renderRoutes(routes, ["/setup"], hubAdapter(hub));
  await userEvent.type(await screen.findByLabelText("Benutzername des Administrators"), "owner");
  await userEvent.click(screen.getByRole("radio", { name: /Reverse Proxy/ }));
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  expect(screen.getByText("IP-Adressen oder CIDRs des Proxys eingeben") !== null).toBe(true);
  expect(hub.claims).toHaveLength(0);
  await userEvent.click(screen.getByRole("radio", { name: /Kein Login/ }));
  expect(screen.queryByLabelText("Neues Passwort") === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  await waitFor(() => expect(hub.claims).toHaveLength(1));
  expect(hub.claims[0]).toEqual({ mode: "none", username: "owner" });
});

it("shows a forced mode locked and claims with it", async () => {
  const hub: Hub = { pending: true, forced: true, mode: "none", claims: [] };
  renderRoutes(routes, ["/setup"], hubAdapter(hub));
  expect((await screen.findByText(/AUTH_MODE legt die Anmeldung/)).textContent).toContain("Kein Login");
  expect(screen.queryByRole("radio", { name: /Lokaler Admin/ }) === null).toBe(true);
  await userEvent.type(screen.getByLabelText("Benutzername des Administrators"), "owner");
  await userEvent.click(screen.getByRole("button", { name: "Administrator anlegen" }));
  await waitFor(() => expect(hub.claims).toHaveLength(1));
  expect(hub.claims[0]).toMatchObject({ mode: "none" });
});

it("connected wizards start from saved values; addresses stay open to discovery only while none is saved", () => {
  const draft = freshSetupDraft();
  const saved = { general: { ...draft.general, hubName: "Lab", hubAddress: "192.0.2.10:3552", domainUrl: "https://hub.example.com" }, hostDefaults: { ...draft.hostDefaults, projectDir: "/srv/docker" } };
  const upgraded = hydrateFromSettings(draft, saved, { proxy: true });
  expect([upgraded.general.hubName, upgraded.general.hubAddress, upgraded.hostDefaults.projectDir, upgraded.modules.proxy, upgraded.hubAddressOrigin?.kind]).toEqual(["Lab", "192.0.2.10:3552", "/srv/docker", true, "manual"]);
  const blank = hydrateFromSettings(draft, { general: { ...draft.general, hubName: "Other" }, hostDefaults: { ...draft.hostDefaults, projectDir: "/srv/docker" } }, { proxy: true });
  expect([blank.general.hubName, blank.hostDefaults.projectDir, blank.modules.proxy, blank.hubAddressOrigin, blank.domainEdited, blank.hydrated]).toEqual(["Other", "/srv/docker", true, null, false, true]);
  expect(hydrateFromSettings(upgraded, saved)).toBe(upgraded);
});
