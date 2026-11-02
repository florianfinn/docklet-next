// Source: docklet-next/docs/design/backup-completion.md
// Revision: 57866390a8785e38ef98ab15ab31a19880f78c19
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regressions for stored bind access, identity and backup completion findings of the web review (#287, #329).

import { act, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ActionRunner } from "@/data/demo/action-runner";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { DemoDb } from "@/data/demo/demo-db";
import { createVolumesApi } from "@/data/demo/demo-volumes";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import { mapBackupOverview } from "@/data/http/backup-mapping";
import { BACKUP_OPERATIONS } from "@/data/http/backups";
import { entry, overview, policy } from "@/data/http/backups.test-support";
import { createHttpHubBackups } from "@/data/http/hub-backups";
import { resourceFixture } from "@/data/http/resources.test-support";
import type { Host } from "@/domain/model";
import { backupAccess, bindKey, collectMountUsage } from "@/domain/volumes";
import { renderWithApp } from "@/test/render";
import { backupFigures } from "./backup-figures";
import BackupsPage from "./BackupsPage";
import { VolumeBackupsTab } from "./backups/VolumeBackupsTab";

afterEach(() => {
  simulationStore.reset();
  vi.restoreAllMocks();
});
const allowAll = async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
const key = bindKey("/srv/data");
const hosts = ["0", "first"].map((id) => ({ ...HOSTS[0]!, id, name: id, status: "online" }) as Host);
const base = () => ({ ...createDemoAdapter(), listHosts: async () => hosts, getCapability: allowAll });
function demo() {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const db = new DemoDb();
  return { db, api: createVolumesApi({ db, runner: new ActionRunner([]), forcedFailure: () => false, demoFailure: () => undefined }) };
}

it("saves the hub policy's copy target over HTTP", async () => {
  const fetcher = vi.fn(async () => Response.json({ policies: [] }));
  const api = createHttpHubBackups("/api", { fetch: fetcher as typeof fetch, token: async () => "synthetic" });
  await api.saveHubBackupPolicies([{ id: "p", enabled: true, schedule: "0 0 3 * * *", retentionCount: 7, targetId: "copy-target" }]);
  const init = fetcher.mock.calls[0] as unknown as [string, RequestInit];
  expect(JSON.parse(String(init[1].body)).policies[0].targetId).toBe("copy-target");
});

for (const refused of [false, true])
  it(`offers "back up now" on /backups by the live bind verdict (refused ${refused})`, async () => {
    const ov = mapBackupOverview({ ...overview, volumes: [{ name: key, volumeExists: false, count: 1, policy: { ...policy, volumeName: key }, lastBackup: { ...entry, volumeName: key } }] }, "first");
    const inventory = { ...ov.volumes[0]!, bind: { kind: "directory" as const, eligible: !refused, ...(refused ? { reason: "volume.bind.path-denied" } : {}) }, consumers: [{ containerId: "first~c", containerName: "c", target: "/data", readOnly: false, running: true }] };
    renderWithApp(<BackupsPage />, { ...base(), listBackupOverviews: async () => [ov], listVolumes: async () => [inventory], listBackupTargets: async () => [] });
    const button = (await screen.findByRole("button", { name: "Jetzt sichern" })) as HTMLButtonElement;
    await screen.findByRole("link", { name: "/srv/data" });
    expect(button.disabled).toBe(refused);
  });

it("keeps the latest finished outcome in the figures while a newer capture runs", async () => {
  const { db, api } = demo();
  const volume = (await api.listVolumes()).find((v) => v.kind === "volume" && v.backup)!;
  db.backupRuns = db.backupRuns.filter((r) => r.volumeId !== volume.id);
  db.backupRuns.push({ id: "done", volumeId: volume.id, targetId: "local", startedAt: "2026-11-01T00:00:00Z", status: "succeeded", trigger: "manual", stoppedContainers: false });
  const count = backupFigures([await api.getBackupOverview(volume.hostId)], []).succeeded;
  db.backupRuns.push({ id: "active", volumeId: volume.id, targetId: "local", startedAt: "2026-11-02T00:00:00Z", status: "running", trigger: "manual", stoppedContainers: false, actionId: "capturing" });
  const after = await api.getBackupOverview(volume.hostId);
  expect([count > 0, backupFigures([after], []).succeeded, after.volumes.find((v) => v.id === volume.id)?.running]).toEqual([true, count, { kind: "backup", activityId: "capturing" }]);
});

it("keeps a stored demo bind readable after its last consumer is gone", async () => {
  const { db, api } = demo();
  const volume = (await api.listVolumes()).find((v) => v.kind === "bind" && v.bind?.eligible)!;
  db.backupRuns.push({ id: "saved", volumeId: volume.id, targetId: "local", startedAt: "2026-11-02T00:00:00Z", status: "succeeded", trigger: "manual", stoppedContainers: false });
  vi.spyOn(db, "containers").mockReturnValue([]);
  const ov = await api.getBackupOverview(volume.hostId);
  expect(ov.volumes.some((v) => v.id === volume.id)).toBe(true);
  await expect(api.getVolume(volume.id)).resolves.toMatchObject({ backups: [{ id: "saved" }] });
  await expect(api.deleteBackup(volume.id, "saved")).resolves.toBeUndefined();
});

it("keeps backup access for a bind key retained only by a restore row", async () => {
  const f = resourceFixture(),
    original = f.fetcher.getMockImplementation()!;
  const restore = { restoreId: "r", activityId: "a", at: "2026-11-02T00:00:00Z", kind: "full", mode: "replace", status: "failed", phase: "preparing", pendingRestart: false, stoppedContainers: [] };
  f.fetcher.mockImplementation(async (input) => {
    const u = new URL(String(input), "https://hub.example.com");
    if (u.pathname === "/api/capabilities") return Response.json({ success: true, data: [...BACKUP_OPERATIONS, "listHosts", "listVolumes", "getVolume"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: u.searchParams.get("kind") ?? "hub", id: u.searchParams.get("id") ?? "0" } })) });
    if (u.pathname.endsWith("/overview")) return Response.json({ success: true, data: { ...overview, volumes: [{ name: key, volumeExists: false, count: 0, lastRestore: restore, pendingRestores: [] }] } });
    if (u.pathname.endsWith("/backups")) return Response.json({ success: true, data: [], pagination: { totalItems: 0 } });
    return original(input);
  });
  expect(backupAccess(await f.adapter.getVolume(`first~${key}`))).toBe("full");
});

it("gives distinct demo identities to bind paths that differ only in punctuation", () => {
  const containers = ["/srv/app.data", "/srv/app-data"].map((source, i) => ({ id: `c${i}`, name: `c${i}`, hostId: "h", state: "running", mounts: [`${source}:/data`] }));
  expect(collectMountUsage(containers, () => undefined).map((v) => v.name)).toEqual(["/srv/app.data", "/srv/app-data"]);
});

it("does not treat a bind as a missing volume from the Docker volume list", async () => {
  const mapped = mapBackupOverview({ ...overview, volumes: [{ name: key, volumeExists: false, count: 1, lastBackup: { ...entry, volumeName: key } }] }, "first").volumes[0]!;
  const volume = { ...mapped, bind: { kind: "directory" as const, eligible: true }, backups: [{ id: "r", volumeId: `first~${key}`, hostId: "first", startedAt: "2026-11-02T00:00:00Z", status: "succeeded" as const, trigger: "manual" as const, targetId: "local", archive: "r", stoppedContainers: false }] };
  renderWithApp(<VolumeBackupsTab volume={volume} hostOnline />, { ...base(), getBackupOverview: async () => ({ ...overview, hostId: "first", timeZone: "UTC", authority: "a", setVersion: 1, targets: [], volumes: [volume] }) });
  const button = (await screen.findByRole("button", { name: "Dateien wiederherstellen" })) as HTMLButtonElement;
  expect([screen.queryByText("Volume existiert nicht mehr. Seine Snapshots können weiterhin wiederhergestellt werden.") !== null, button.disabled]).toEqual([false, false]);
});

it("tests an edited demo target with its stored secret when none is entered", async () => {
  const adapter = createDemoAdapter();
  // The demo refuses this simulated SMB password like a real permission error.
  const refusedByDemo = "wrong";
  const saved = await adapter.createBackupTarget({ name: "Stored", kind: "smb", server: "nas.example.com", share: "backups", path: "docklet", secret: refusedByDemo });
  const probeHostId = (await adapter.listHosts()).find((h) => h.status === "online")!.id;
  const result = await adapter.testBackupTargetDraft!({ target: { name: saved.name, kind: saved.kind, server: saved.server, share: saved.share, path: saved.path }, probeHostId, targetId: saved.id });
  expect(result).toEqual({ ok: false, code: "backup.target.permission-denied" });
});

it("offers a new plan only once the configured default retention has loaded", async () => {
  let release!: (count: number) => void;
  const held = new Promise<number>((r) => (release = r));
  const ov = mapBackupOverview(overview, "first");
  const volume = { ...ov.volumes[0]!, id: "first~fresh", name: "fresh", backup: undefined, lastBackup: undefined, backups: [] };
  renderWithApp(<VolumeBackupsTab volume={volume} hostOnline />, { ...base(), getBackupOverview: async () => ov, getBackupDefaultRetention: () => held, setBackupPolicy: vi.fn(async () => {}) });
  await screen.findByRole("button", { name: "Jetzt sichern" });
  expect(screen.queryByRole("button", { name: "Plan speichern" })).toBeNull();
  await act(async () => release(30));
  await screen.findByRole("button", { name: "Plan speichern" });
  expect((document.getElementById("bk-retention") as HTMLInputElement).value).toBe("30");
});

/** Runs a demo operation through the real ActionRunner and returns its terminal status. */
async function settle(runner: ActionRunner, activity: Promise<string>) {
  const id = await activity;
  await vi.advanceTimersByTimeAsync(5000);
  return runner.list().find((r) => r.id === id)?.status;
}
function timed() {
  vi.useFakeTimers();
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 3 }));
  const db = new DemoDb(),
    runner = new ActionRunner([]);
  return { db, runner, api: createVolumesApi({ db, runner, forcedFailure: () => false, demoFailure: () => undefined }) };
}

it("keeps a demo bind key retained only by its restore row in overview and detail", async () => {
  const { db, runner, api } = timed();
  const volume = (await api.listVolumes()).find((v) => v.kind === "bind" && v.bind?.eligible)!;
  expect(await settle(runner, api.runBackup(volume.id))).toBe("succeeded");
  const snapshot = (await api.getVolume(volume.id)).backups[0]!;
  expect(await settle(runner, api.restoreBackup(volume.id, snapshot.id, "replace"))).toBe("succeeded");
  for (const run of (await api.getVolume(volume.id)).backups) await api.deleteBackup(volume.id, run.id);
  await api.setBackupPolicy(volume.id, undefined);
  vi.spyOn(db, "containers").mockReturnValue([]);
  const ov = await api.getBackupOverview(volume.hostId);
  const detail = await api.getVolume(volume.id);
  expect([ov.volumes.some((v) => v.id === volume.id), detail.lastRestore?.status, backupAccess(detail)]).toEqual([true, "succeeded", "full"]);
  vi.useRealTimers();
});

for (const mounted of [true, false])
  it(`refuses demo source work once a bind path becomes a backup target (mounted ${mounted})`, async () => {
    const { db, runner, api } = timed();
    const volume = (await api.listVolumes()).find((v) => v.kind === "bind" && v.bind?.eligible && !v.fileBind)!;
    expect(await settle(runner, api.runBackup(volume.id))).toBe("succeeded");
    const snapshot = (await api.getVolume(volume.id)).backups[0]!;
    await api.createBackupTarget({ name: "Protected source", kind: "local", hostId: volume.hostId, path: volume.name });
    if (!mounted) vi.spyOn(db, "containers").mockReturnValue([]);
    if (mounted) expect((await api.listVolumes()).find((v) => v.id === volume.id)?.bind?.reason).toBe("volume.bind.path-denied");
    await expect(api.downloadBackup(volume.id, snapshot.id)).resolves.toBeInstanceOf(Blob);
    await expect(api.runBackup(volume.id)).rejects.toMatchObject({ problem: { code: "volume.bind.path-denied" } });
    vi.useRealTimers();
  });
