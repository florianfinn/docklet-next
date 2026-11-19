// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original Settings › Sign-in runtime change tests through the real HTTP adapter.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { createHttpAdapter } from "@/data/http/http-adapter";
import { renderWithApp } from "@/test/render";

import { ConnectedAuthSection } from "./ConnectedAuthSection";

afterEach(() => { cleanup(); localStorage.clear(); });

function adapter(policy: { mode: "local" | "proxy" | "none"; forced?: boolean }, puts: unknown[], refuse?: string) {
  return createHttpAdapter({ fetch: async (input, init) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    const admin = { id: "admin", username: "owner", requiresPasswordChange: false, isAdmin: true, assignments: [] };
    if (path === "/api/auth/mode") return json({ mode: policy.mode, setupPending: false, forced: policy.forced ?? false });
    if (path === "/api/auth/sign-in" && init?.method === "PUT") {
      puts.push(JSON.parse(String(init.body)));
      if (refuse) return new Response(JSON.stringify({ status: 409, code: refuse }), { status: 409, headers: { "Content-Type": "application/problem+json" } });
      policy.mode = (puts.at(-1) as { mode: typeof policy.mode }).mode;
      return json({ success: true, status: "authenticated", user: admin });
    }
    if (path === "/api/auth/sign-in") return json({ mode: policy.mode, setupPending: false, forced: policy.forced ?? false, proxyHeader: "Remote-User", trustedProxies: "" });
    if (path === "/api/auth/me") return json(admin);
    if (path === "/api/users") return json([]);
    return new Response("{}", { status: 503 });
  } });
}

it("changes the stored sign-in mode and asks for a password when switching to local", async () => {
  const puts: unknown[] = [];
  renderWithApp(<ConnectedAuthSection />, adapter({ mode: "none" }, puts));
  const apply = await screen.findByRole("button", { name: "Anmeldeart übernehmen" });
  expect(apply.hasAttribute("disabled")).toBe(true);
  await userEvent.click(screen.getByRole("radio", { name: /Lokaler Admin/ }));
  await userEvent.type(screen.getByLabelText("Passwort für dein Konto"), "Example-Setup-Password1!");
  await userEvent.click(screen.getByRole("button", { name: "Anmeldeart übernehmen" }));
  await waitFor(() => expect(puts).toEqual([{ mode: "local", password: "Example-Setup-Password1!" }]));
});

it("explains a refused proxy switch and keeps the form", async () => {
  const puts: unknown[] = [];
  renderWithApp(<ConnectedAuthSection />, adapter({ mode: "local" }, puts, "auth.proxy-unverified"));
  await userEvent.click(await screen.findByRole("radio", { name: /Reverse Proxy/ }));
  await userEvent.type(screen.getByLabelText("Vertrauenswürdige Proxy-Adressen"), "192.0.2.10");
  await userEvent.click(screen.getByRole("button", { name: "Anmeldeart übernehmen" }));
  expect(await screen.findByText(/nicht über den vertrauenswürdigen Proxy/) !== null).toBe(true);
  expect(puts).toEqual([{ mode: "proxy", proxyHeader: "Remote-User", trustedProxies: "192.0.2.10" }]);
});

it("shows a forced mode locked without a form", async () => {
  renderWithApp(<ConnectedAuthSection />, adapter({ mode: "proxy", forced: true }, []));
  expect((await screen.findByText(/AUTH_MODE legt die Anmeldung/)).textContent).toContain("Reverse Proxy");
  expect(screen.queryByRole("button", { name: "Anmeldeart übernehmen" }) === null).toBe(true);
});
