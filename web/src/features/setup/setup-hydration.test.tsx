// Source: docklet-next/issues/517
// Revision: 9ee12a697e657c533f58d87b38b32a2458600f63
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original connected wizard initialization tests through the real HTTP adapter and page.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { createHttpAdapter } from "@/data/http/http-adapter";
import { renderRoutes } from "@/test/render";

import SetupPage from "./SetupPage";
import { restartSetup, setupStore } from "./setup-draft";

afterEach(() => { cleanup(); restartSetup(); localStorage.clear(); });

type Read = "ok" | "fail" | Promise<void>;
const domainOnly = { detectedAt: "2026-11-19T12:00:00Z", agentImage: "registry.example.com/docklet/agent:1.2.3", containerDetection: "unknown",
  candidates: [{ id: "domain", url: "https://hub.example.com", kind: "domain", source: "configured", reach: "unknown" }] };

/** A claimed hub with an enabled proxy module; reads can be held or fail until `heal()`. */
function hub(opts: { settings?: Read; modules?: Read; blank?: boolean; discovery?: Promise<unknown> } = {}) {
  const writes: Record<string, string>[] = [];
  let reads = 0;
  const state = { settings: opts.settings ?? "ok", modules: opts.modules ?? "ok" };
  const values = {
    setupCompleted: "false", hubName: "Saved Hub", hubAddress: "192.0.2.10:3552", domainUrl: "https://saved.example.com", linkBase: "auto", effectiveTimeZone: "UTC",
    defaultHostConnection: "direct", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true",
    keepActionOutput: "true", actionPresentation: "dialog", moduleProxyEnabled: "true", ...(opts.blank ? { hubAddress: "", domainUrl: "" } : {}),
  };
  const list = () => new Response(JSON.stringify(Object.entries(values).map(([key, value]) => ({ key, value }))));
  const read = async (r: Read) => { reads++; if (r === "fail") return new Response("{}", { status: 503 }); if (r !== "ok") await r; return list(); };
  const adapter = createHttpAdapter({ fetch: async (input, init) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x"), "Content-Type": "application/json" } });
    if (path === "/api/capabilities") return json([{ operationId: "discoverHub", available: true, resourceRef: { kind: "hub", id: "0" } }]);
    if (path === "/api/hub/discovery" && opts.discovery) return json(await opts.discovery);
    if (path === "/api/auth/mode") return json({ mode: "none", setupPending: false });
    if (path === "/api/auth/me") return json({ id: "admin", username: "owner", isAdmin: true, requiresPasswordChange: false, assignments: [] });
    if (path === "/api/environments/0/settings/public") return read(state.modules);
    if (path === "/api/environments/0/settings") {
      if (init?.method === "PUT") { writes.push(JSON.parse(String(init.body)) as Record<string, string>); return json([]); }
      return read(state.settings);
    }
    return new Response("{}", { status: 503 });
  } });
  return { adapter, writes, reads: () => reads, heal: () => { state.settings = "ok"; state.modules = "ok"; } };
}

const page = (h: ReturnType<typeof hub>) => renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"], h.adapter);
async function finish(h: ReturnType<typeof hub>) {
  for (let i = 0; i < 3; i++) await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.click(screen.getByRole("button", { name: "Einrichtung abschließen" }));
  await waitFor(() => expect(h.writes).toHaveLength(1));
  return h.writes[0] ?? {};
}

it("holds the form until the saved settings arrive, then shows them for editing", async () => {
  let release!: () => void;
  const h = hub({ settings: new Promise<void>((r) => { release = r; }) });
  page(h);
  await waitFor(() => expect(h.reads()).toBeGreaterThanOrEqual(2));
  expect([screen.queryByLabelText("Domain") === null, screen.queryByRole("button", { name: "Weiter" }) === null]).toEqual([true, true]);
  expect(screen.queryByText(/Gespeicherte Einstellungen des Hubs werden geladen/) !== null).toBe(true);
  release();
  const domain = await screen.findByLabelText("Domain") as HTMLInputElement;
  expect(domain.value).toBe("https://saved.example.com");
  await userEvent.clear(domain);
  await userEvent.type(domain, "https://own.example.org");
  expect((await finish(h)).domainUrl).toBe("https://own.example.org");
});

for (const failing of ["settings", "modules"] as const) {
  it(`a failed ${failing} read blocks Finish until a retry succeeds and keeps the enabled module`, async () => {
    const h = hub({ [failing]: "fail" });
    page(h);
    await waitFor(() => expect(h.reads()).toBeGreaterThanOrEqual(2));
    await waitFor(() => expect([setupStore.get().hydrated, screen.queryByRole("button", { name: "Weiter" }) === null]).toEqual([false, true]));
    await waitFor(() => expect(screen.queryByText(/konnten nicht geladen werden/) !== null).toBe(true));
    h.heal();
    await userEvent.click(screen.getByRole("button", { name: "Erneut versuchen" }));
    await screen.findByLabelText("Domain");
    expect((await finish(h)).moduleProxyEnabled).toBe("true");
  });
}

it("an upgraded hub with blank addresses keeps its saved name, projects and modules", async () => {
  const h = hub({ blank: true });
  page(h);
  await userEvent.type(await screen.findByLabelText("IP-Adresse"), "192.0.2.25:3552");
  const sent = await finish(h);
  expect([sent.hubName, sent.agentProjectsDirectory, sent.agentInstallDirectory, sent.moduleProxyEnabled, sent.hubAddress]).toEqual(["Saved Hub", "/srv/projects", "/srv/agent", "true", "192.0.2.25:3552"]);
});

it("late discovery fills the untouched domain beside a typed IP but never a cleared one", async () => {
  for (const clearDomain of [false, true]) {
    let release!: (d: unknown) => void;
    const h = hub({ blank: true, discovery: new Promise<unknown>((r) => { release = r; }) });
    const view = page(h);
    const ip = await screen.findByLabelText("IP-Adresse") as HTMLInputElement;
    await userEvent.type(ip, "192.0.2.25:3552");
    const domain = screen.getByLabelText("Domain") as HTMLInputElement;
    if (clearDomain) { await userEvent.type(domain, "x"); await userEvent.clear(domain); }
    release(domainOnly);
    await screen.findByRole("group", { name: "Erkannte Adressen" });
    expect([ip.value, domain.value]).toEqual(["192.0.2.25:3552", clearDomain ? "" : "https://hub.example.com"]);
    view.unmount();
    restartSetup();
  }
});
