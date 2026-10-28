// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original live backup gestures, host failure isolation and key lifetime regressions.

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { backupFixture, entry, ok } from "@/data/http/backups.test-support";
import { mapBackup } from "@/data/http/backup-mapping";
import { renderRoutes, renderWithApp } from "@/test/render";
import { BackupHistory } from "./BackupHistory";
import { VolumeBackupsTab } from "./VolumeBackupsTab";
import { RestoreDialog } from "./RestoreDialog";
import { RestoreFilesSheet } from "./RestoreFilesSheet";
import { BackupBadge } from "../storage-visuals";
import BackupsPage from "../BackupsPage";
import { RecoveryKeyDialog } from "@/features/settings/backups/RecoveryKeyDialog";
import { BackupTargetsSection } from "@/features/settings/sections/BackupTargetsSection";
import { TargetDialog } from "@/features/settings/backups/TargetDialog";

function setup() {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const f = backupFixture();
  const adapter = { ...createDemoAdapter(), ...f.api, listHosts: async () => f.hosts, getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true }), observeActionDetail: vi.fn(() => () => {}) };
  return { ...f, adapter };
}
afterEach(() => { simulationStore.reset(); localStorage.clear(); vi.restoreAllMocks(); });

it("shows partial and safety, restricts unavailable rows to deletion and translates history errors", async () => {
  const f = setup(), volume = (await f.api.getBackupOverview("first")).volumes[0]!;
  const backups = [mapBackup(entry, "first"), { ...mapBackup(entry, "first"), id: "missing", status: "unavailable" as const }];
  renderWithApp(<BackupHistory volume={{ ...volume, backups }} targets={[]} hostOnline />, f.adapter);
  const partial = screen.getByTestId("backup-snapshot"), missing = screen.getByTestId("backup-missing");
  expect(partial.textContent).toContain("Teilweise"); expect(partial.textContent).toContain("Sicherheitskopie");
  expect(within(partial).getByRole("button", { name: "Dateien wiederherstellen" }) !== null).toBe(true);
  expect(within(missing).queryAllByRole("button").map((b) => b.textContent)).toEqual(["Snapshot löschen"]);
});
it("defaults to replace, prefills new name and displays existing-name refusal", async () => {
  const f = setup(), volume = (await f.api.getBackupOverview("first")).volumes[0]!;
  f.state.fail = "/restore"; f.state.code = "volume.exists";
  renderWithApp(<RestoreDialog volume={volume} runId="snapshot" onClose={() => {}} accepted={() => {}} />, f.adapter);
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByRole("radio", { name: /Daten ersetzen/ }).getAttribute("aria-checked")).toBe("true");
  await userEvent.click(within(dialog).getByRole("radio", { name: /Als neues Volume/ }));
  expect((screen.getByLabelText("Name des neuen Volumes") as HTMLInputElement).value).toMatch(/^data-restore-\d{8}$/);
  await userEvent.click(within(dialog).getByRole("button", { name: "Wiederherstellen" }));
  expect((await screen.findByRole("alert")).textContent).toContain("Ein Volume mit diesem Namen ist bereits vorhanden");
  expect(document.body.textContent).not.toContain("PRIVATE");
});
it("paginates the snapshot browser, keeps selected paths and confirms restore", async () => {
  const f = setup(); const accepted = vi.fn();
  renderWithApp(<RestoreFilesSheet volumeId="first~data" runId="snapshot" onClose={() => {}} accepted={accepted} />, f.adapter);
  await userEvent.click(await screen.findByRole("checkbox", { name: "folder" }));
  await userEvent.click(screen.getByRole("button", { name: "Nächste Seite" }));
  await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("start=100"))).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Dateien wiederherstellen" }));
  await userEvent.click(screen.getByRole("button", { name: "Wiederherstellen" }));
  await waitFor(() => expect(accepted).toHaveBeenCalledWith("accepted"));
  expect(f.state.requests.find((r) => r.path.endsWith("restore-files"))?.body).toEqual({ paths: ["/folder"] });
});
it("shows a typed refusal for a directory beyond the listing budget", async () => {
  const f = setup(); f.state.fail = "/files/browse"; f.state.code = "volume.backup.listing-too-large";
  renderWithApp(<RestoreFilesSheet volumeId="first~data" runId="snapshot" onClose={() => {}} accepted={() => {}} />, f.adapter);
  expect((await screen.findByRole("alert")).textContent).toContain("Ergebnis überschreitet");
});
it("rediscovers running restores on reload without a POST and shows pending restart safety context", async () => {
  const f = setup(); f.state.overview.volumes![0]!.running = { activityId: "rediscovered", kind: "restore" };
  f.state.overview.volumes![0]!.lastRestore = { restoreId: "failed", activityId: "old", kind: "full", mode: "replace", status: "failed", phase: "writing", at: "2026-10-28T02:00:00Z", pendingRestart: true, safetyBackupId: "safety", stoppedContainers: [{ id: "consumer", name: "database" }] };
  const volume = (await f.api.getBackupOverview("first")).volumes[0]!;
  renderWithApp(<VolumeBackupsTab volume={{ ...volume, backups: [] }} hostOnline />, f.adapter);
  await waitFor(() => expect(f.adapter.observeActionDetail).toHaveBeenCalledWith("first", "rediscovered", expect.any(Function)));
  expect(screen.queryByRole("alert")?.textContent).toContain("database"); expect(screen.queryByRole("alert")?.textContent).toContain("safety");
  expect(f.state.requests.some((r) => r.method === "POST")).toBe(false);
  expect((screen.getByRole("button", { name: "Jetzt sichern" }) as HTMLButtonElement).disabled).toBe(true);
});
it("keeps another host populated when one overview is unavailable", async () => {
  const f = setup(); f.intercept(async (url) => url.pathname.includes("/second/") ? Response.json({ code: "agent.unavailable" }, { status: 503 }) : undefined);
  renderRoutes([{ path: "/backups", element: <BackupsPage /> }], ["/backups"], f.adapter);
  await screen.findByRole("link", { name: "data" });
  expect(await screen.findByText(/Backupinformationen für diesen Host sind nicht verfügbar/) !== null).toBe(true);
  expect(screen.queryByText("Kein Backup") === null).toBe(true);
});
it("requires key confirmation and discards a late key after close", async () => {
  const f = setup(); let finish!: (response: Response) => void;
  const close = vi.fn(); f.intercept(async (url) => url.pathname.endsWith("recovery-key") ? new Promise<Response>((done) => { finish = done; }) : undefined);
  let client!: QueryClient;
  function CacheProbe() { client = useQueryClient(); return null; }
  const { unmount } = renderWithApp(<><CacheProbe /><RecoveryKeyDialog id="target" name="Archive" onClose={close} /></>, f.adapter);
  expect(f.state.requests.some((r) => r.path.endsWith("recovery-key"))).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "Bestätigen und Schlüssel anzeigen" }));
  await waitFor(() => expect(typeof finish).toBe("function"));
  await userEvent.click(screen.getByRole("button", { name: "Schließen" }));
  expect(close).toHaveBeenCalled();
  await act(async () => { finish(ok({ recoveryKey: f.state.key })); });
  expect(document.body.textContent).not.toContain(f.state.key); expect(JSON.stringify(client.getQueryCache().getAll().map((q) => q.state.data))).not.toContain(f.state.key); unmount();
});
it("shows earlier-settings checks and locks existing target locations", async () => {
  const f = setup(); renderWithApp(<BackupTargetsSection />, f.adapter);
  await screen.findByText("Archive");
  expect(document.body.textContent).toContain("Mit früheren Einstellungen getestet");
  await userEvent.click(screen.getByRole("button", { name: "Bearbeiten" }));
  const dialog = await screen.findByRole("dialog");
  expect((within(dialog).getByLabelText("Server") as HTMLInputElement).disabled || (within(dialog).getByLabelText("Server").closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
});
it("passes entered credentials and optional existing recovery key only to target creation", async () => {
  const f = setup(), saved = vi.fn(); renderWithApp(<TargetDialog onClose={() => {}} saved={saved} />, f.adapter);
  await userEvent.type(screen.getByLabelText("Name"), "Archive"); await userEvent.type(screen.getByLabelText("Server"), "nas.example.com"); await userEvent.type(screen.getByLabelText("Freigabe"), "backups");
  await userEvent.type(screen.getByLabelText("Passwort"), "test-only"); await userEvent.type(screen.getByLabelText("Vorhandener Wiederherstellungsschlüssel (optional)"), "synthetic-key");
  await userEvent.click(screen.getByRole("button", { name: "Übernehmen" }));
  await waitFor(() => expect(saved).toHaveBeenCalled());
  expect(f.state.requests.find((r) => r.method === "POST")?.body).toMatchObject({ secret: "test-only", recoveryKey: "synthetic-key" });
  expect(JSON.stringify(await f.api.listBackupTargets())).not.toContain("test-only");
});
it("shows the named-volume boundary for bind mounts", async () => {
  const f = setup(), volume = (await f.api.getBackupOverview("first")).volumes[0]!;
  renderWithApp(<VolumeBackupsTab volume={{ ...volume, kind: "bind", backups: [] }} hostOnline />, f.adapter);
  expect(screen.getByRole("status").textContent).toBe("Backups umfassen nur benannte Volumes.");
});
it("keeps typed target retirement refusal and host references inside the confirmation", async () => {
  const f = setup(); f.state.fail = "/backup-targets/target"; f.state.code = "backup.target.hosts-unknown";
  renderWithApp(<BackupTargetsSection />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Entfernen" }));
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Entfernen" }));
  await waitFor(() => expect(within(dialog).queryByRole("alert")?.textContent).toContain("second"));
  expect(within(dialog).getByRole("alert").textContent).not.toContain("PRIVATE");
});
it("uses the backend next-run instant in the owning host time zone", async () => {
  const f = setup(); const volume = (await f.api.getBackupOverview("first")).volumes[0]!;
  renderWithApp(<VolumeBackupsTab volume={{ ...volume, backups: [] }} hostOnline />, f.adapter);
  await screen.findByRole("button", { name: "Plan speichern" });
  expect(screen.getByTestId("backup-next").textContent).toContain("Europe/Berlin");
  expect(screen.getByTestId("backup-next").textContent).toContain("02:00");
});

it("sorts overview targets using the host overview target names", async () => {
  const f = setup();
  f.state.overview.targets = [{ id: "local", kind: "local", name: "Zeta" }, { id: "alpha", kind: "smb", name: "Alpha" }];
  f.state.overview.volumes!.push({ ...f.state.overview.volumes![0]!, name: "second", policy: { ...f.state.overview.volumes![0]!.policy!, targetId: "alpha" } });
  renderWithApp(<BackupsPage />, f.adapter);
  await screen.findByRole("button", { name: "Nach Ziel sortieren" });
  await userEvent.click(screen.getByRole("button", { name: "Nach Ziel sortieren" }));
  expect(screen.getAllByRole("link", { name: /^(data|second)$/ })[0]?.textContent).toBe("second");
});
it("shows a backup reader the status without requiring policy write permission", async () => {
  const f = setup(), volume = (await f.api.getBackupOverview("first")).volumes[0]!;
  f.adapter.getCapability = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: operationId !== "setBackupPolicy", disabledReasonCode: "capability.unavailable" });
  renderWithApp(<BackupBadge volume={volume} />, { ...f.adapter, kind: "http" });
  await waitFor(() => expect(screen.queryByText("Teilweise") !== null).toBe(true));
});
