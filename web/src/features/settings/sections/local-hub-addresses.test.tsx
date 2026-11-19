// Source: docklet-next/issues/529
// Revision: 6f21fc09c6e94a47d611e7371b27e8a9e5c000c7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original legacy address gestures and real HTTP settings save/reload coverage.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { createHttpAdapter } from "@/data/http/http-adapter";
import { useConnectedSettings } from "@/data/queries";
import { agentEnv } from "@/domain/agent-install";
import type { Host } from "@/domain/model";
import { agentHubUrl, notificationBaseUrl } from "@/domain/settings";
import { existingHostPlan } from "@/features/hosts/existing-host-plan";
import SetupPage from "@/features/setup/SetupPage";
import { restartSetup, setupStore } from "@/features/setup/setup-draft";
import { renderRoutes } from "@/test/render";

import { GeneralSection } from "./GeneralSection";

afterEach(() => { cleanup(); restartSetup(); localStorage.clear(); });

const discovery = { detectedAt: "2026-11-19T12:00:00Z", agentImage: "registry.example.com/docklet/agent:1.2.3", containerDetection: "unknown", candidates: [
  { id: "domain", url: "https://hub.example.com", kind: "domain", source: "configured", origins: ["settingsDomain", "appUrl"], reach: "unknown" },
  { id: "ip", url: "http://192.0.2.10:3552", kind: "ip", source: "configured", origins: ["settingsAddress", "published", "interface", "request"], reach: "unknown" },
] };

function hub(domainUrl = "", hubAddress = "hub.example.com", delays: { discovery?: Promise<void>; write?: Promise<void> } = {}) {
  const writes: Record<string, string>[] = [];
  const values: Record<string, string> = { setupCompleted: "false", hubName: "Saved Hub", hubAddress, domainUrl, linkBase: "auto", effectiveTimeZone: "UTC", timeZone: "",
    defaultHostConnection: "edge", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true",
    keepActionOutput: "true", actionPresentation: "dialog", moduleProxyEnabled: "true" };
  const adapter = createHttpAdapter({ fetch: async (input, init) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const headers = { "X-CSRF-Token": "example".padEnd(43, "x"), "Content-Type": "application/json" };
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers });
    if (path === "/api/auth/mode") return json({ mode: "none", setupPending: false });
    if (path === "/api/auth/me") return json({ id: "admin", username: "owner", isAdmin: true, requiresPasswordChange: false, assignments: [] });
    if (path === "/api/capabilities") return json(["discoverHub", "saveSettings"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })));
    if (path === "/api/hub/discovery") { await delays.discovery; return json(discovery); }
    if (path === "/api/environments/0/settings" || path === "/api/environments/0/settings/public") {
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as Record<string, string>;
        writes.push(body); await delays.write; Object.assign(values, body); return json([]);
      }
      return new Response(JSON.stringify(Object.entries(values).map(([key, value]) => ({ key, value }))), { headers });
    }
    return new Response("{}", { status: 503 });
  } });
  return { adapter, writes, values };
}

function General() {
  const saved = useConnectedSettings(true);
  return saved.data ? <GeneralSection settings={saved.data} /> : null;
}
const open = (h: ReturnType<typeof hub>) => renderRoutes([{ path: "/settings/general", element: <General /> }], ["/settings/general"], h.adapter);
const input = (label: string) => screen.getByLabelText(label) as HTMLInputElement;
const save = async () => { await userEvent.click(screen.getByRole("button", { name: "Speichern" })); };

it("B16 loads a legacy value without writes and keeps it on an unrelated save", async () => {
  const h = hub(); open(h);
  await screen.findByText("Keine IP-Adresse. Agents brauchen die IP des Hosts.");
  expect(input("IP-Adresse").value).toBe("hub.example.com");
  expect(h.writes).toHaveLength(0);
  await userEvent.type(input("Name des Hubs"), " changed");
  await save();
  await waitFor(() => expect(h.writes).toHaveLength(1));
  expect(h.writes[0]?.hubAddress).toBe("hub.example.com");
});

it("B16 moves explicitly, requires a new IP, then saves and reloads both consumer URLs", async () => {
  const h = hub(); const view = open(h);
  await screen.findByText("Keine IP-Adresse. Agents brauchen die IP des Hosts.");
  // Wait for the actual discovery effect: the bare name must inherit APP_URL's scheme.
  await waitFor(() => expect((screen.getByRole("button", { name: "Zur Domain verschieben" }) as HTMLButtonElement).disabled).toBe(false));
  expect(h.writes).toHaveLength(0);
  await userEvent.click(screen.getByRole("button", { name: "Zur Domain verschieben" }));
  expect([input("IP-Adresse").value, input("Domain").value]).toEqual(["", "https://hub.example.com"]);
  await save();
  expect(h.writes).toHaveLength(0);
  await userEvent.type(input("IP-Adresse"), "2001:db8::10");
  await save();
  await waitFor(() => expect(h.writes).toHaveLength(1));
  const saved = await h.adapter.getConnectedSettings();
  expect([saved.general.hubAddress, saved.general.domainUrl]).toEqual(["2001:db8::10", "https://hub.example.com"]);
  expect(agentHubUrl(saved.general)).toBe("http://[2001:db8::10]");
  expect(notificationBaseUrl(saved.general)).toEqual({ source: "domain", url: "https://hub.example.com" });
  // Both installation modes consume the actual saved General settings through the real plan builder.
  for (const connection of ["direct", "edge"] as const) {
    const host = { name: "Example", connection, address: "192.0.2.20:3552" } as Host;
    expect(agentEnv(existingHostPlan(host, saved.hostDefaults, saved.general))).toContain("MANAGER_API_URL='http://[2001:db8::10]'");
  }
  view.unmount(); open(h);
  await waitFor(() => expect(input("IP-Adresse").value).toBe("2001:db8::10"));
  expect(input("Domain").value).toBe("https://hub.example.com");
  expect(h.writes).toHaveLength(1);
});

it("B16 cancels or confirms replacement without writing until save", async () => {
  const h = hub("http://other.example.com"); open(h);
  await screen.findByText("Keine IP-Adresse. Agents brauchen die IP des Hosts.");
  await userEvent.click(screen.getByRole("button", { name: "Zur Domain verschieben" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Abbrechen" }));
  expect([input("IP-Adresse").value, input("Domain").value, h.writes.length]).toEqual(["hub.example.com", "http://other.example.com", 0]);
  await userEvent.click(screen.getByRole("button", { name: "Zur Domain verschieben" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Ersetzen" }));
  expect([input("IP-Adresse").value, input("Domain").value, h.writes.length]).toEqual(["", "https://hub.example.com", 0]);
  await save(); expect(h.writes).toHaveLength(0);
});

it("B18 rejects an edited legacy name even when restored, and rejects paths on save", async () => {
  const h = hub(); open(h);
  await screen.findByLabelText("IP-Adresse");
  await userEvent.clear(input("IP-Adresse"));
  await userEvent.type(input("IP-Adresse"), "hub.example.com");
  await userEvent.type(input("Name des Hubs"), " changed");
  await save(); expect(h.writes).toHaveLength(0);
  await userEvent.clear(input("IP-Adresse"));
  await userEvent.type(input("IP-Adresse"), "[[2001:db8::10]:3552");
  await userEvent.type(input("Domain"), "https://hub.example.com/path");
  await save(); expect(h.writes).toHaveLength(0);
  await userEvent.clear(input("Domain"));
  await userEvent.type(input("Domain"), "hub.example.com:3552");
  await save(); await waitFor(() => expect(h.writes).toHaveLength(1));
  expect(h.writes[0]?.domainUrl).toBe("https://hub.example.com:3552");
});

it("B16/B17 hydrates setup without writes, separates candidate fields and shows every merged origin once", async () => {
  const h = hub();
  renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"], h.adapter);
  await screen.findByText("Keine IP-Adresse. Agents brauchen die IP des Hosts.");
  const candidates = await screen.findByRole("group", { name: "Erkannte Adressen" });
  const domains = within(candidates).getByRole("list", { name: "Erkannte Adressen: Domain" });
  const ips = within(candidates).getByRole("list", { name: "Erkannte Adressen: IP-Adresse" });
  expect(within(domains).getAllByRole("listitem")).toHaveLength(1);
  expect(within(ips).getAllByRole("listitem")).toHaveLength(1);
  expect(domains.textContent).toContain("https://hub.example.com");
  for (const label of ["Domain in den Einstellungen", "APP_URL"]) expect(within(domains).getAllByText(label)).toHaveLength(1);
  for (const label of ["IP-Adresse in den Einstellungen", "Veröffentlichter Port", "Hub-Schnittstelle", "Adresse dieser Sitzung"]) expect(within(ips).getAllByText(label)).toHaveLength(1);
  expect(input("IP-Adresse").value).toBe("hub.example.com");
  expect(h.writes).toHaveLength(0);
  await userEvent.click(within(domains).getByRole("button", { name: "https://hub.example.com übernehmen" }));
  expect([input("Domain").value, input("IP-Adresse").value]).toEqual(["https://hub.example.com", "hub.example.com"]);
  await userEvent.click(screen.getByRole("button", { name: "Zur Domain verschieben" }));
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  expect(screen.queryByLabelText("IP-Adresse") !== null).toBe(true);
  expect(h.writes).toHaveLength(0);
  await userEvent.click(within(ips).getByRole("button", { name: "192.0.2.10:3552 übernehmen" }));
  await userEvent.clear(input("Domain"));
  await userEvent.type(input("Domain"), "hub.example.com:3552");
  for (let i = 0; i < 3; i++) await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.click(screen.getByRole("button", { name: "Einrichtung abschließen" }));
  await waitFor(() => expect(h.writes).toHaveLength(1));
  expect([h.writes[0]?.hubAddress, h.writes[0]?.domainUrl]).toEqual(["192.0.2.10:3552", "https://hub.example.com:3552"]);
});

it("waits for late APP_URL discovery before moving a bare name", async () => {
  let release!: () => void;
  const h = hub("", "hub.example.com", { discovery: new Promise<void>((r) => { release = r; }) });
  open(h);
  const move = await screen.findByRole("button", { name: "Zur Domain verschieben" }) as HTMLButtonElement;
  expect(move.disabled).toBe(true);
  expect(h.writes).toHaveLength(0);
  release();
  await waitFor(() => expect(move.disabled).toBe(false));
  await userEvent.click(move);
  expect(input("Domain").value).toBe("https://hub.example.com");
});

it("keeps an IP edit made during an unrelated save subject to validation", async () => {
  let release!: () => void;
  const h = hub("", "hub.example.com", { write: new Promise<void>((r) => { release = r; }) });
  open(h); await screen.findByLabelText("IP-Adresse");
  await userEvent.type(input("Name des Hubs"), " changed");
  await save();
  await waitFor(() => expect(h.writes).toHaveLength(1));
  await userEvent.clear(input("IP-Adresse"));
  await userEvent.type(input("IP-Adresse"), "hub.example.com");
  release();
  await waitFor(() => expect((screen.getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true));
  await userEvent.type(input("Name des Hubs"), " again");
  await save();
  expect(h.writes).toHaveLength(1);
  expect(screen.queryByText(/IPv4- oder IPv6-Adresse/) !== null).toBe(true);
});

it("revalidates the IP when finishing directly from the setup review", async () => {
  const h = hub();
  setupStore.set((d) => ({ ...d, reached: 4 }));
  renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup?step=review"], h.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Einrichtung abschließen" }));
  await screen.findByLabelText("IP-Adresse");
  expect(h.writes).toHaveLength(0);
});

it("AUTO-2 a legacy http:// IP value offers the bare IP instead of a domain move", async () => {
  const h = hub("", "http://192.0.2.10:8080"); open(h);
  await screen.findByText("Die IP-Adresse wird ohne http:// oder https:// gespeichert.");
  expect([screen.queryByRole("button", { name: "Zur Domain verschieben" }) === null, h.writes.length]).toEqual([true, 0]);
  await userEvent.click(screen.getByRole("button", { name: "192.0.2.10:8080 übernehmen" }));
  expect([input("IP-Adresse").value, input("Domain").value]).toEqual(["192.0.2.10:8080", ""]);
  await save();
  await waitFor(() => expect(h.writes).toHaveLength(1));
  expect(h.writes[0]?.hubAddress).toBe("192.0.2.10:8080");
});

it.each([["http://[2001:db8::10]", "2001:db8::10"], ["http://[::ffff:192.0.2.10]", "::ffff:192.0.2.10"], ["http://[2001:db8::10]:80", "[2001:db8::10]:80"], ["http://192.0.2.10:80", "192.0.2.10:80"], ["http://[2001:db8::10]:3552", "[2001:db8::10]:3552"]])("C-B1 setup prefills discovered %s in the stored form %s and advances", async (url, stored) => {
  const ip = discovery.candidates[1]!, original = ip.url;
  ip.url = url;
  try {
    const h = hub("", "");
    renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup"], h.adapter);
    await waitFor(() => expect(input("IP-Adresse").value).toBe(stored));
    await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
    await waitFor(() => expect(screen.queryByLabelText("IP-Adresse") === null).toBe(true));
    expect(h.writes).toHaveLength(0);
  } finally { ip.url = original; }
});
