// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 3d2d4f27b524b3422d010d6a69a22fe57225a78f
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original production boundary and regression coverage for issue #27.

import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeAll, expect, it, vi } from "vitest";

vi.mock("@/runtime", () => vi.importActual("./runtime"));

let routes: typeof import("@/app/router").routes;
let renderRoutes: typeof import("@/test/render").renderRoutes;
let renderWithApp: typeof import("@/test/render").renderWithApp;
let createAdapter: typeof import("./runtime").createAdapter;
let fixture: typeof import("./production-connected.test-support").connectedFixture;
let BackupPolicyCard: typeof import("@/features/volumes/backups/BackupPolicyCard").BackupPolicyCard;
let BackupHistory: typeof import("@/features/volumes/backups/BackupHistory").BackupHistory;
let AppsWidget: typeof import("@/features/dashboard/widgets/SmallWidgets").AppsWidget;
let QueryError: typeof import("@/shared/components/page").QueryError;
let unavailable: typeof import("@/data/capability").unavailable;

// Cold router imports use a generous hang timeout so shared CI load does not fail setup.
beforeAll(async () => {
  vi.stubEnv("DEV", false);
  vi.resetModules();
  ({ routes } = await import("@/app/router"));
  ({ renderRoutes, renderWithApp } = await import("@/test/render"));
  ({ createAdapter } = await import("./runtime"));
  ({ connectedFixture: fixture } = await import("./production-connected.test-support"));
  ({ BackupPolicyCard } = await import("@/features/volumes/backups/BackupPolicyCard"));
  ({ BackupHistory } = await import("@/features/volumes/backups/BackupHistory"));
  ({ AppsWidget } = await import("@/features/dashboard/widgets/SmallWidgets"));
  ({ QueryError } = await import("@/shared/components/page"));
  ({ unavailable } = await import("@/data/capability"));
}, 60_000);
afterEach(() => { cleanup(); localStorage.clear(); });
afterAll(() => vi.unstubAllEnvs());

// This full production navigation flow uses a hang timeout to tolerate shared CI load.
it("connects login, dashboard, hosts, containers, stacks, volumes and files with the production HTTP runtime", async () => {
  expect(import.meta.env.DEV).toBe(false);
  const f = fixture();
  const adapter = createAdapter({ fetch: f.fetcher as typeof fetch, webSocket: f.webSocket, eventSource: () => f.source });
  expect(adapter.kind).toBe("http");
  const { router } = renderRoutes(routes, ["/dashboard"], adapter);
  await screen.findByLabelText("Benutzername");
  await userEvent.type(screen.getByLabelText("Benutzername"), "operator");
  await userEvent.type(screen.getByLabelText("Passwort"), "example-password");
  await userEvent.click(screen.getByRole("button", { name: "Anmelden" }));
  await screen.findByRole("heading", { name: "Dashboard", level: 1 });
  await waitFor(() => {
    const schedule = document.querySelector('[data-widget="schedule"]');
    expect(schedule?.textContent).toContain("Zeitplan nicht verfügbar");
    expect(schedule?.textContent).toContain("first");
    expect(schedule?.textContent).not.toContain("Daten konnten nicht geladen werden");
  });
  await userEvent.click(screen.getByRole("link", { name: "Hosts" }));
  await screen.findByRole("heading", { name: "first", level: 2 });
  expect(screen.getByRole("button", { name: "Verbindung prüfen" }).hasAttribute("disabled")).toBe(true);
  await userEvent.click(screen.getByRole("link", { name: "Container" }));
  await screen.findByRole("button", { name: "Details zu web öffnen" });
  await userEvent.click(screen.getByRole("link", { name: "Stacks" }));
  await userEvent.click(await screen.findByRole("link", { name: "Stack app öffnen" }));
  await screen.findByRole("heading", { name: "app", level: 1 });
  await userEvent.click(screen.getByRole("tab", { name: /Deployments/ }));
  expect((await screen.findByText("Deploy-Verlauf ist nicht verfügbar.")) !== null).toBe(true);
  await userEvent.click(screen.getByRole("link", { name: "Volumes" }));
  await screen.findByRole("link", { name: "data öffnen" });
  await userEvent.click(screen.getByRole("button", { name: "Aktionen für data" }));
  const files = await screen.findByRole("menuitem", { name: "Dateien öffnen" });
  await waitFor(() => expect(files.getAttribute("aria-disabled")).not.toBe("true"));
  await userEvent.click(files);
  await screen.findByRole("heading", { name: "data", level: 1 });
  expect(screen.getByRole("tab", { name: "Dateien" }).getAttribute("aria-selected")).toBe("true");
  await userEvent.click(await screen.findByRole("button", { name: "hello.txt öffnen" }));
  await waitFor(() => expect(document.querySelector(".cm-content")?.textContent).toBe("hello.txt"));
  expect(f.state.calls).toContain("/api/environments/first/volumes/data/workspace/file");
  await userEvent.click(screen.getByRole("tab", { name: "Übersicht" }));
  const consumer = await screen.findByRole("row", { name: /web/ });
  expect(consumer.textContent).toContain("/data");
  expect(within(consumer).getByRole("link", { name: "Container öffnen" }).getAttribute("href")).toContain("first~web");
  expect(f.state.calls.some((path) => path.endsWith("/volumes/data/usage"))).toBe(false);
  expect(screen.getByRole("tab", { name: "Backups" }).hasAttribute("disabled")).toBe(false);
  expect(screen.queryByText("Kein Backup") === null).toBe(true);
  expect(screen.queryByText("Backup-Status nicht verfügbar") === null).toBe(true);
  await act(() => router.navigate("/volumes/first~data?tab=backups"));
  expect(await screen.findByRole("button", { name: "Plan speichern" }) !== null).toBe(true);
  expect(await screen.findByText(/Sicherheitskopie ·/) !== null).toBe(true);
  await act(() => router.navigate("/backups"));
  await screen.findByRole("link", { name: "data" });
  expect(screen.queryByText(/ungeschützt/)).toBeNull();
  await act(() => router.navigate("/networks?bench=100"));
  await screen.findByRole("heading", { name: "Netzwerke", level: 1 });
  await waitFor(() => expect(document.body.textContent).toContain("front"));
  expect(document.body.textContent).not.toMatch(/Bench 0|bench_net_|svc-99/);
  await act(() => router.navigate("/preview/unknown"));
  await screen.findByText("Diese Adresse ist nicht verfügbar.");
  expect(document.querySelector('[data-layer="design-lab"]') === null).toBe(true);
}, 120_000);

it.each(["policy", "history"] as const)("renders live backup %s controls when the server advertises the mapping", async (kind) => {
  const f = fixture(); f.state.signedIn = true;
  const adapter = createAdapter({ fetch: f.fetcher as typeof fetch });
  const volume = await adapter.getVolume("first~data");
  const write = vi.spyOn(adapter, "setBackupPolicy");
  const backup = vi.spyOn(adapter, "runBackup");
  const restore = vi.spyOn(adapter, "restoreBackup");
  renderWithApp(kind === "policy" ? <BackupPolicyCard volume={volume} targets={[]} defaultRetentionCount={7} hostOnline /> : <BackupHistory volume={volume} targets={[]} hostOnline />, adapter);
  if (kind === "policy") await screen.findByRole("button", { name: "Plan speichern" });
  else await screen.findByRole("button", { name: "Wiederherstellen" });
  expect(screen.queryByText("Diese Funktion ist nicht verfügbar") === null).toBe(true);
  expect(write).not.toHaveBeenCalled(); expect(backup).not.toHaveBeenCalled(); expect(restore).not.toHaveBeenCalled();
});

it("distinguishes unavailable reads from failed loads", async () => {
  const f = fixture(); const adapter = createAdapter({ fetch: f.fetcher as typeof fetch });
  const { rerender } = renderWithApp(<QueryError error={unavailable("listSchedule", { kind: "hub", id: "0" }, "capability.mapping-unavailable")} />, adapter);
  await screen.findByText("Diese Funktion ist nicht verfügbar");
  expect(screen.queryByRole("alert") === null).toBe(true);
  // Use a second mounted app so providers and translated error rendering stay real.
  rerender(<></>);
  renderWithApp(<QueryError error={new Error("offline")} />, adapter);
  expect(screen.getByRole("alert").textContent).toContain("Daten konnten nicht geladen werden");
});

it("opens product app links and omits unsafe protocols", async () => {
  const f = fixture(); const adapter = createAdapter({ fetch: f.fetcher as typeof fetch });
  const [stack] = await adapter.listStacks();
  renderWithApp(<AppsWidget stacks={[{ ...stack!, url: "https://app.example.com" }, { ...stack!, id: "unsafe", url: "javascript:void(0)" }]} hosts={new Map()} />, adapter);
  const links = screen.getAllByRole("link");
  expect(links).toHaveLength(1);
  expect(links[0]?.getAttribute("href")).toBe("https://app.example.com");
  expect(links[0]?.getAttribute("rel")).toBe("noopener noreferrer");
});

it("connects the production security route and Scans settings to HTTP producers", async () => {
  const { securityFixture } = await import("./data/http/vulnerabilities.test-support");
  const f = securityFixture();
  f.state.settings.vulnerabilityScanSchedule = '{"mode":"daily","time":"03:00"}';
  const adapter = createAdapter({ fetch: f.fetcher as typeof fetch, eventSource: () => f.source });
  const { router } = renderRoutes(routes, ["/security?tab=images"], adapter);
  await screen.findByRole("table", { name: "Images" });
  expect(screen.getAllByText("example:1.0.0")).toHaveLength(2);
  expect(document.body.textContent).not.toMatch(/synthetic|synthetisch|Befunde sind/);
  await act(() => router.navigate("/settings/security"));
  await screen.findByRole("switch", { name: "Automatische Scans" });
  expect(screen.getByText(/Nächste Ausführung:/) !== null).toBe(true);
  expect(f.fetcher.mock.calls.some(([url]) => String(url).endsWith("/environments/0/jobs"))).toBe(true);
});

it("connects notification targets and per-host Auto Heal to HTTP producers without demo text", async () => {
  const { securityFixture } = await import("./data/http/vulnerabilities.test-support");
  const { default: targets } = await import("./data/http/testdata/notification-targets.json");
  const f = securityFixture();
  const base = f.fetcher.getMockImplementation()!;
  const heal = { enabled: true, maxRestarts: 4, windowMinutes: 45, excludedStacks: ["web"], excludedContainers: [], managedByEnvironment: false };
  const json = (body: unknown) => new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
  f.fetcher.mockImplementation(async (input, init) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname === "/api/capabilities") {
      const body = await (await base(input, init)).json();
      const ref = { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" };
      body.data.push(...["listNotificationTargets", "saveNotificationTarget", "deleteNotificationTarget", "testNotificationTarget", "getAutoHealSettings", "saveAutoHealSettings"].map((operationId) => ({ operationId, available: true, resourceRef: ref })));
      return json(body);
    }
    if (url.pathname === "/api/environments/0/notifications/settings") return json(targets);
    if (url.pathname.endsWith("/auto-heal")) return json(heal);
    return base(input, init);
  });
  const adapter = createAdapter({ fetch: f.fetcher as typeof fetch, eventSource: () => f.source });
  const { router } = renderRoutes(routes, ["/settings/notifications"], adapter);
  await screen.findByText("reader@example.com · mail.example.com");
  expect(screen.getByRole("switch", { name: "Aktiv: ntfy" }).getAttribute("aria-checked")).toBe("false");
  expect(screen.queryByText(/nicht bearbeiten kann|simuliert|alex@example\.com/) === null).toBe(true);
  await act(() => router.navigate("/settings/autoHeal"));
  const rows = await screen.findAllByText(/Höchstens 4 Neustarts je 45 min · 1 Stack und keine Container ausgenommen/);
  expect(rows.length).toBeGreaterThan(0);
  expect(document.body.textContent).not.toMatch(/Atlas|Delta|craft/);
});
