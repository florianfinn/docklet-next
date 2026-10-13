import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { routes } from "@/app/router";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { SETTINGS } from "@/data/demo/fixtures/settings";
import { labOpenStore } from "@/features/design-lab/lab-open-store";
import { FlowsSection } from "@/features/design-lab/ControlsSection";
import { renderRoutes, renderWithApp } from "@/test/render";

import LoginPage from "./LoginPage";
import { resolveVariant, simulateLogin, validateLogin } from "./login-model";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});
afterEach(() => {
  vi.useRealTimers();
  simulationStore.reset();
  labOpenStore.set(false);
  localStorage.clear();
  sessionStorage.clear();
});

const auth = SETTINGS.auth;
const loginError = () =>
  waitFor(() => {
    const el = document.querySelector<HTMLElement>('[data-slot="login-error"]');
    if (!el) throw new Error("no login error");
    return el;
  });

describe("login model", () => {
  it("requires both fields and resolves the variant from the settings unless the demo fixes it", () => {
    expect([validateLogin({ username: " ", password: "" }), validateLogin({ username: "admin", password: "x" })]).toEqual([{ username: "required", password: "required" }, {}]);
    expect([resolveVariant("settings", auth), resolveVariant("settings", { ...auth, mode: "proxy" }), resolveVariant("none", auth), resolveVariant("settings", undefined)]).toEqual(["local", "proxy", "none", "local"]);
  });

  it("simulates outcomes after a delay without comparing a stored password; abort cancels", async () => {
    vi.useFakeTimers();
    const run = (username: string, forced: "none" | "invalid" | "unavailable" | "locked" = "none", variant: "local" | "proxy" = "local") => {
      const p = simulateLogin(variant, { username, password: "irgendwas" }, { auth, forced, delayMs: 100 });
      void vi.advanceTimersByTimeAsync(100);
      return p;
    };
    expect(await run("admin")).toEqual({ ok: true, user: "admin" });
    expect(await run("root")).toEqual({ ok: false, reason: "invalid" });
    expect(await run("admin", "locked")).toEqual({ ok: false, reason: "locked" });
    expect(await run("", "none", "proxy")).toEqual({ ok: true, user: "" });
    const controller = new AbortController();
    const pending = simulateLogin("local", { username: "admin", password: "x" }, { auth, forced: "none", delayMs: 100, signal: controller.signal });
    controller.abort();
    await expect(pending).rejects.toThrow("aborted");
  });
});

describe("login page", () => {
  const render = () =>
    renderRoutes(
      [
        { path: "/login", element: <LoginPage /> },
        { path: "/dashboard", element: <p>Dashboard erreicht</p> },
      ],
      ["/login"],
    );

  it("validates, shows the simulated error, keeps the button busy while checking and navigates on success", async () => {
    const { router } = render();
    expect(await screen.findByRole("heading", { level: 1, name: "Bei Docklet anmelden" })).not.toBe(null);
    expect(screen.getByText("Demo · keine echte Anmeldung") !== null).toBe(true);
    await userEvent.click(await screen.findByRole("button", { name: "Anmelden" }));
    expect(screen.getAllByText("Pflichtfeld").length).toBe(2);

    await userEvent.type(screen.getByLabelText("Benutzername"), "root");
    await userEvent.type(screen.getByLabelText("Passwort"), "geheim-123");
    await userEvent.click(screen.getByRole("button", { name: "Anmelden" }));
    const busy = screen.getByRole("button", { name: "Melde an …" });
    expect([busy.getAttribute("aria-busy"), document.activeElement === busy]).toEqual(["true", true]);
    expect((await loginError()).textContent).toBe("Benutzername oder Passwort stimmen nicht.");
    // Typing again clears the error.
    await userEvent.type(screen.getByLabelText("Benutzername"), "x");
    expect(document.querySelector('[data-slot="login-error"]')).toBe(null);

    await userEvent.clear(screen.getByLabelText("Benutzername"));
    await userEvent.type(screen.getByLabelText("Benutzername"), "admin");
    await userEvent.click(screen.getByRole("button", { name: "Anmelden" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"), { timeout: 3000 });
    // Nothing secret survives in browser storage.
    expect(JSON.stringify({ ...localStorage }).includes("geheim-123") || JSON.stringify({ ...sessionStorage }).includes("geheim-123")).toBe(false);
  });

  it("offers forced errors and the proxy variant through the demo controls", async () => {
    render();
    const controls = await screen.findByRole("region", { name: "Demo-Steuerung" });
    await userEvent.click(within(controls).getByRole("radio", { name: "Nicht erreichbar" }));
    await userEvent.type(await screen.findByLabelText("Benutzername"), "admin");
    await userEvent.type(screen.getByLabelText("Passwort"), "x");
    await userEvent.click(screen.getByRole("button", { name: "Anmelden" }));
    expect((await loginError()).textContent).toBe("Der Hub ist gerade nicht erreichbar. Versuche es gleich noch einmal.");

    await userEvent.click(within(controls).getByRole("radio", { name: "Proxy" }));
    expect(screen.queryByLabelText("Passwort")).toBe(null);
    expect(screen.getByText(/Header Remote-User/) !== null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Über den Proxy fortfahren (simuliert)" }));
    expect((await loginError()).textContent).toBe("Der Proxy hat keinen Benutzer-Header gesendet oder ist nicht als vertrauenswürdig eingetragen.");

    // "Ohne" is no page: the demo offers only variants with a form.
    expect(within(controls).queryByRole("radio", { name: "Ohne" })).toBe(null);
  });

  it("goes straight to the dashboard when the hub runs without login, without a warning or continue button", async () => {
    const adapter = createDemoAdapter();
    await adapter.saveSettings("auth", { ...SETTINGS.auth, mode: "none" });
    const { router } = renderRoutes(
      [
        { path: "/login", element: <LoginPage /> },
        { path: "/dashboard", element: <p>Dashboard erreicht</p> },
      ],
      ["/login"],
      adapter,
    );
    expect(await screen.findByText("Dashboard erreicht")).not.toBe(null);
    expect([router.state.location.pathname, screen.queryByText(/keine Anmeldung eingerichtet/), screen.queryByRole("link", { name: "Weiter zum Dashboard" })]).toEqual(["/dashboard", null, null]);
  });

  it("keeps the local and proxy demo reachable through ?variant= while the hub runs without login", async () => {
    const adapter = createDemoAdapter();
    await adapter.saveSettings("auth", { ...SETTINGS.auth, mode: "none" });
    renderRoutes([{ path: "/login", element: <LoginPage /> }], ["/login?variant=local"], adapter);
    expect(await screen.findByLabelText("Benutzername")).not.toBe(null);
    await userEvent.click(within(screen.getByRole("region", { name: "Demo-Steuerung" })).getByRole("radio", { name: "Proxy" }));
    expect(screen.getByRole("button", { name: "Über den Proxy fortfahren (simuliert)" }) !== null).toBe(true);
  });

  it("explains a settings load error and still lets the demo choose a variant", async () => {
    simulationStore.set((s) => ({ ...s, failArea: "settings" }));
    render();
    expect(await screen.findByText(/Die Anmeldeart konnte nicht geladen werden/, undefined, { timeout: 4000 })).not.toBe(null);
    await userEvent.click(within(screen.getByRole("region", { name: "Demo-Steuerung" })).getByRole("radio", { name: "Lokal" }));
    expect(screen.queryByText(/Die Anmeldeart konnte nicht geladen werden/)).toBe(null);
    expect(screen.getByLabelText("Benutzername") !== null).toBe(true);
  });

  it("is a full-screen route outside the shell, opened from the design lab flows", async () => {
    expect(routes.some((r) => r.path === "login")).toBe(true);
    labOpenStore.set(true);
    const { router } = renderWithApp(<FlowsSection />);
    await userEvent.click(screen.getByRole("button", { name: "Anmeldung (Demo) öffnen" }));
    expect(router.state.location.pathname).toBe("/login");
    expect(labOpenStore.get()).toBe(false);
  });
});
