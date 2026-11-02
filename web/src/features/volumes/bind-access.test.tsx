// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 24b4de71db45699547c23815b5c17bc8ca547758
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regressions for bind identity, access and schedule findings of the S2 review (#287, #328, #291).

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { mapBackupOverview, mapBackupPolicy } from "@/data/http/backup-mapping";
import { BACKUP_OPERATIONS } from "@/data/http/backups";
import { backupFixture, entry, overview, policy } from "@/data/http/backups.test-support";
import { mapContainer } from "@/data/http/inventory-mapping";
import { resourceFixture } from "@/data/http/resources.test-support";
import { parseCompose } from "@/domain/compose/model";
import { bindKey, type Volume } from "@/domain/volumes";
import { ContainerFacts } from "@/features/containers/ContainerFacts";
import { ComposeFileTree } from "@/features/stacks/editor/ComposeFileTree";
import { renderWithApp } from "@/test/render";
import { BackupPolicyCard } from "./backups/BackupPolicyCard";
import { VolumeBackupsTab } from "./backups/VolumeBackupsTab";
import { BackupBadge } from "./storage-visuals";

afterEach(() => {
  simulationStore.reset();
  vi.restoreAllMocks();
});
const key = bindKey("/srv/example~data");
const consumer = (service: string) => ({ containerId: `first~${service}`, containerName: service, stackId: "first~app", service, target: "/data", readOnly: false, running: true });
const allowAll = async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });

it("links each service's mount at a shared target to its own storage entry", async () => {
  const adapter = createDemoAdapter();
  const volumes = ["db", "cache"].map((service) => ({ id: `first~${bindKey(`/srv/${service}`)}`, hostId: "first", name: `/srv/${service}`, source: `/srv/${service}`, kind: "bind", consumers: [consumer(service)] })) as Volume[];
  adapter.listVolumes = async () => volumes;
  const parsed = parseCompose('services:\n  db:\n    image: example:1\n    volumes: ["./db:/data"]\n  cache:\n    image: example:1\n    volumes: ["./cache:/data"]\n');
  if (!parsed.ok) throw new Error("invalid fixture");
  renderWithApp(<ComposeFileTree projectName="app" projectDir="/srv/app" hostId="first" stackId="first~app" active="compose" composeDirty={false} envDirty={false} model={parsed.model} onOpen={() => {}} />, adapter);
  await waitFor(() => expect(screen.getAllByRole("link").length).toBe(2));
  const href = (name: RegExp) => screen.getByRole("link", { name }).getAttribute("href") ?? "";
  expect([href(/\.\/db/).includes(encodeURIComponent(volumes[0]!.id)), href(/\.\/cache/).includes(encodeURIComponent(volumes[1]!.id))]).toEqual([true, true]);
});

it("pauses an active custom plan as manual and resumes it with the same cron", async () => {
  const adapter = createDemoAdapter();
  const volume = await adapter.getVolume("vol-atlas-home_ha_config");
  volume.backup = mapBackupPolicy({ ...policy, schedule: "0 30 1 * * 1-5", enabled: true });
  adapter.setBackupPolicy = vi.fn(async () => {});
  renderWithApp(<BackupPolicyCard volume={volume} targets={[{ id: "local", name: "Local", kind: "local" }]} hostOnline />, adapter);
  await userEvent.click(screen.getByRole("radio", { name: "Manuell" }));
  await userEvent.click(screen.getByRole("button", { name: "Plan speichern" }));
  await userEvent.click(screen.getByRole("radio", { name: "Benutzerdefinierter Zeitplan" }));
  await userEvent.click(screen.getByRole("button", { name: "Plan speichern" }));
  await waitFor(() => expect(vi.mocked(adapter.setBackupPolicy).mock.calls.length).toBe(2));
  expect(vi.mocked(adapter.setBackupPolicy).mock.calls.map(([, sent]) => [sent?.enabled, sent?.schedule])).toEqual([
    [false, { mode: "manual", cron: "0 30 1 * * 1-5" }],
    [true, { mode: "custom", cron: "0 30 1 * * 1-5" }],
  ]);
});

it("runs and lists backups of eligible demo binds like the agent", async () => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const adapter = createDemoAdapter();
  const hosts = await adapter.listHosts();
  const bind = (await adapter.listVolumes()).find((v) => v.kind === "bind" && v.bind?.eligible && v.bind.kind === "directory" && hosts.some((h) => h.id === v.hostId && h.status === "online"))!;
  expect(typeof (await adapter.runBackup(bind.id))).toBe("string");
  const rows = (await adapter.getBackupOverview(bind.hostId)).volumes;
  expect(rows.some((v) => v.id === bind.id)).toBe(true);
});

it("shows the recorded outcome of an overview bind", () => {
  const volume = mapBackupOverview({ ...overview, volumes: [{ name: key, volumeExists: true, count: 1, lastBackup: { ...entry, volumeName: key } }] }, "first").volumes[0]!;
  renderWithApp(<BackupBadge volume={volume} />);
  expect(screen.queryByText("Teilweise") !== null).toBe(true);
});

it("offers no directory-only restore for an eligible bind of unknown kind", async () => {
  const f = backupFixture();
  f.state.overview.volumes![0]!.name = key;
  const adapter = { ...createDemoAdapter(), ...f.api, getCapability: allowAll };
  const volume = { ...(await f.api.getBackupOverview("first")).volumes[0]!, bind: { kind: "unknown" as const, eligible: true }, fileBind: false, backups: [] };
  renderWithApp(<VolumeBackupsTab volume={volume} hostOnline />, adapter);
  await screen.findByRole("button", { name: "Jetzt sichern" });
  expect(screen.queryByLabelText("Archiv zur Wiederherstellung hochladen") === null).toBe(true);
});

function storedBindFixture(volumeExists = false) {
  const f = resourceFixture();
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname === "/api/capabilities") return Response.json({ success: true, data: [...BACKUP_OPERATIONS, "listHosts", "listVolumes", "getVolume"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })) });
    if (url.pathname.endsWith("/overview")) return Response.json({ success: true, data: { ...overview, volumes: [{ name: key, volumeExists, count: 1, lastBackup: { ...entry, volumeName: key } }] } });
    if (url.pathname.endsWith("/backups")) return Response.json({ success: true, data: [{ ...entry, volumeName: key }], pagination: { totalItems: 1 } });
    return original(input);
  });
  return f;
}

for (const exists of [false, true])
  it(`keeps the stored history of an unmounted bind (overview volumeExists ${exists})`, async () => {
    const volume = await storedBindFixture(exists).adapter.getVolume(`first~${key}`);
    expect([volume.id, volume.backups.map((v) => v.id)]).toEqual([`first~${key}`, ["snapshot"]]);
  });

it("keeps the stored history of a now protected bind downloadable, without restore", async () => {
  const f = storedBindFixture(true);
  f.state.bind = true;
  f.state.binds = [{ key, source: "/srv/example~data", kind: "directory", eligible: false, reason: "volume.bind.path-denied", consumers: [] }];
  const volume = await f.adapter.getVolume(`first~${key}`);
  expect([volume.bind?.eligible, volume.backups.map((v) => v.id)]).toEqual([false, ["snapshot"]]);
  renderWithApp(<VolumeBackupsTab volume={volume} hostOnline />, { ...createDemoAdapter(), ...f.adapter, getCapability: allowAll });
  await screen.findByRole("button", { name: "Herunterladen" });
  expect([screen.queryByRole("button", { name: "Wiederherstellen" }), screen.queryByRole("button", { name: "Jetzt sichern" })]).toEqual([null, null]);
});

it("links a daemon mount by its structured target, also with a colon in the source", async () => {
  const c = mapContainer({ id: "web", names: ["/web"], image: "example:1", imageId: "x", command: "", created: 0, ports: [], labels: {}, state: "running", status: "", hostConfig: {}, networkSettings: { networks: {} }, autoUpdateEnabled: false, origin: "standalone", originSource: "detected", mounts: [{ type: "bind", source: "/srv/app:data", destination: "/config", rw: true }] } as never, "first", [], { stale: false });
  const bind = { id: `first~${bindKey("/srv/app:data")}`, hostId: "first", name: "/srv/app:data", source: "/srv/app:data", kind: "bind", consumers: [{ containerId: c.id, containerName: "web", target: "/config", readOnly: false, running: true }] } as Volume;
  const adapter = createDemoAdapter();
  adapter.listVolumes = async () => [bind];
  renderWithApp(<ContainerFacts c={c} host={undefined} />, adapter);
  const link = await screen.findByRole("link", { name: /\/srv\/app:data:\/config/ });
  expect(link.getAttribute("href")?.includes(encodeURIComponent(bind.id))).toBe(true);
});

it("rediscovers a running bind operation by its key after an ambiguous admission", async () => {
  const f = backupFixture();
  f.state.overview.volumes![0]!.name = key;
  f.state.overview.volumes![0]!.running = { activityId: "rediscovered", kind: "backup" };
  f.intercept(async (url, init) => (url.pathname.endsWith("/backups") && init?.method === "POST" ? Response.json({ code: "unclassified" }, { status: 502 }) : undefined));
  await expect(f.api.runBackup(`first~${key}`)).resolves.toBe("rediscovered");
  expect(f.accepted).toHaveBeenCalledWith("first", "rediscovered");
});
