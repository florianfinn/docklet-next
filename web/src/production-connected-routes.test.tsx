// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 3d2d4f27b524b3422d010d6a69a22fe57225a78f
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original production boundary and regression coverage for issue #27.

import { act, cleanup, screen, waitFor } from "@testing-library/react";
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
});
afterEach(() => { cleanup(); localStorage.clear(); });
afterAll(() => vi.unstubAllEnvs());

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
    expect(schedule?.textContent).toContain("Diese Funktion ist nicht verfügbar");
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
  expect(await screen.findByText("Noch keine Deployments aufgezeichnet.")).toBeTruthy();
  await userEvent.click(screen.getByRole("link", { name: "Volumes" }));
  await userEvent.click(await screen.findByRole("link", { name: "data öffnen" }));
  await screen.findByRole("heading", { name: "data", level: 1 });
  await userEvent.click(screen.getByRole("tab", { name: "Dateien" }));
  await userEvent.click(await screen.findByRole("button", { name: "hello.txt öffnen" }));
  await waitFor(() => expect(document.querySelector(".cm-content")?.textContent).toBe("hello.txt"));
  expect(f.state.calls).toContain("/api/environments/first/volumes/data/workspace/file");
  expect(screen.getByRole("tab", { name: "Backups" }).hasAttribute("disabled")).toBe(true);
  expect(screen.queryByText("Kein Backup") === null).toBe(true);
  expect(screen.getByText("Backup-Status nicht verfügbar") !== null).toBe(true);
  await act(() => router.navigate("/volumes/first~data?tab=backups"));
  expect(await screen.findByText("Diese Funktion ist nicht verfügbar")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Plan speichern" }) === null).toBe(true);
  await act(() => router.navigate("/backups"));
  await screen.findByText("Diese Funktion ist nicht verfügbar");
  expect(screen.queryByText(/ungeschützt/)).toBeNull();
  await act(() => router.navigate("/networks?bench=100"));
  await screen.findByRole("heading", { name: "Netzwerke", level: 1 });
  await waitFor(() => expect(document.body.textContent).toContain("front"));
  expect(document.body.textContent).not.toMatch(/Bench 0|bench_net_|svc-99/);
  await act(() => router.navigate("/preview/unknown"));
  await screen.findByText("Diese Adresse ist nicht verfügbar.");
  expect(document.querySelector('[data-layer="design-lab"]') === null).toBe(true);
});

it.each(["policy", "history"] as const)("keeps unsupported backup %s controls unmounted despite an advertised server capability", async (kind) => {
  const f = fixture();
  const adapter = createAdapter({ fetch: f.fetcher as typeof fetch });
  const volume = await adapter.getVolume("first~data");
  const write = vi.spyOn(adapter, "setBackupPolicy");
  const backup = vi.spyOn(adapter, "runBackup");
  const restore = vi.spyOn(adapter, "restoreBackup");
  renderWithApp(kind === "policy" ? <BackupPolicyCard volume={volume} targets={[]} defaultRetention={{ maxCount: 7, maxSizeGb: null }} hostOnline /> : <BackupHistory volume={volume} targets={[]} hostOnline />, adapter);
  expect(screen.queryAllByRole("button").length).toBe(0);
  await screen.findByText("Diese Funktion ist nicht verfügbar");
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
