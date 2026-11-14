// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regressions for bind rows, overview identities and size scans (#327, #328).

import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterOutcomeError, type CapabilityDescriptor } from "@/data/capability";
import { bindKey, bindPath, bindSize, stackStorage, storageOf, storageUsable } from "@/domain/volumes";
import { mapBackupOverview } from "./backup-mapping";
import type { CsrfClient } from "./csrf-client";
import { mapContainer } from "./inventory-mapping";
import { resourceFixture } from "./resources.test-support";
import { createVolumeCommands } from "./volume-commands";

const source = "/srv/example~data";

it("encodes bind keys like the backend and rejects malformed keys", () => {
  expect(bindKey("/srv/app")).toBe("bind:2f7372762f617070");
  expect([bindPath("bind:2f7372762f617070"), bindPath("bind:2F73"), bindPath("bind:2f7"), bindPath("data"), bindPath("bind:ff")]).toEqual(["/srv/app", undefined, undefined, undefined, undefined]);
});

it("merges the agent verdict, kind and newest size into bind rows", async () => {
  const f = resourceFixture();
  f.state.bind = true;
  f.state.binds = [{ key: bindKey(source), source, kind: "file", eligible: true, consumers: [], lastSnapshotSize: 100, lastSnapshotAt: "2026-11-01T00:00:00Z", measuredSize: 200, measuredAt: "2026-11-02T00:00:00Z", measuredPartial: true }];
  const rows = (await f.adapter.listVolumes()).filter((v) => v.kind === "bind" && v.hostId === "first");
  expect(rows.map((v) => ({ id: v.id, fileBind: v.fileBind, eligible: v.bind?.eligible, size: v.sizeBytes, unknown: v.sizeUnavailable, partial: bindSize(v.bind)?.partial }))).toEqual([
    { id: `first~${bindKey(source)}`, fileBind: true, eligible: true, size: 200, unknown: false, partial: true },
  ]);
});

it("keeps unchecked and refused binds visible without files or backups", async () => {
  const f = resourceFixture();
  f.state.bind = true;
  const unchecked = (await f.adapter.listVolumes()).find((v) => v.kind === "bind")!;
  expect([unchecked.bind === undefined, storageUsable(unchecked)]).toEqual([true, false]);
  f.state.binds = [{ key: bindKey(source), source, kind: "unknown", eligible: false, reason: "volume.bind.path-denied", consumers: [] }];
  const refused = (await f.adapter.listVolumes()).find((v) => v.kind === "bind")!;
  expect([refused.bind?.reason, storageUsable(refused)]).toEqual(["volume.bind.path-denied", false]);
});

it("shows backup overview binds by their source path while keeping the key as identity", () => {
  const overview = mapBackupOverview({ timeZone: "UTC", backupTargetAuthority: "hub", backupTargetSetVersion: 1, repository: {} as never, targets: [], volumes: [{ name: bindKey("/srv/app"), volumeExists: true } as never] }, "first");
  expect(overview.volumes.map((v) => [v.id, v.name, v.kind, v.source])).toEqual([[`first~${bindKey("/srv/app")}`, "/srv/app", "bind", "/srv/app"]]);
});

it("links container and stack mounts through the daemon-reported consumers", () => {
  const row = { id: "web", names: ["/web"], image: "example:1", imageId: "x", command: "", created: 0, ports: [], labels: {}, state: "running", status: "", hostConfig: {}, networkSettings: { networks: {} }, autoUpdateEnabled: false, origin: "standalone", originSource: "detected",
    mounts: [{ type: "bind", name: "", source: "/srv/app/config", destination: "/config", rw: false }, { type: "volume", name: "data", source: "/var/lib/docker/volumes/data/_data", destination: "/data", rw: true }, { type: "tmpfs", destination: "/tmp" }] } as never;
  expect(mapContainer(row, "first", [], { stale: false }).mounts).toEqual(["/srv/app/config:/config:ro", "data:/data", "/tmp"]);
  const bind = { id: `first~${bindKey("/srv/app/config")}`, kind: "bind" as const, consumers: [{ containerId: "first~web", containerName: "web", stackId: "first~app", service: "web", target: "/config", readOnly: true, running: true }] };
  const volumes = [bind] as never[];
  expect([storageOf(volumes, "first~web", "/config")?.id, storageOf(volumes, "first~web", "/data")?.id]).toEqual([bind.id, undefined]);
  // A relative Compose source resolves through the deployed consumer, never through a guessed host path.
  expect([stackStorage(volumes, "first~app", { source: "./config", target: "/config" }, "web")?.id, stackStorage(volumes, "first~app", { source: "config", target: "/config" }, "web")?.id]).toEqual([bind.id, undefined]);
});

function measure(status: number, body: object) {
  const calls: [string, string][] = [];
  const client = { token: async () => "", fetch: async (url: string, init: RequestInit) => { calls.push([url, String(init.body)]); return new Response(JSON.stringify(body), { status }); } } as unknown as CsrfClient;
  const commands = createVolumeCommands({ baseUrl: "/api", client, capability: async () => ({ available: true }) as CapabilityDescriptor, changed: () => {} });
  return { calls, commands };
}

it("posts the bind key for a size scan and maps the measurement", async () => {
  const key = bindKey("/srv/app");
  const { calls, commands } = measure(200, { success: true, data: { key, measuredSize: 42, measuredAt: "2026-11-02T00:00:00Z", measuredPartial: false } });
  expect(await commands.measureBind!(`first~${key}`)).toEqual({ sizeBytes: 42, measuredAt: "2026-11-02T00:00:00Z", partial: false });
  expect(calls).toEqual([["/api/environments/first/volumes/_collection/binds/size", JSON.stringify({ key })]]);
  await expect(commands.measureBind!("first~data")).rejects.toThrow();
});

it("keeps the typed size-scan timeout for the timeout hint", async () => {
  const { commands } = measure(504, { code: "volume.bind.size-timeout" });
  const error = await commands.measureBind!(`first~${bindKey("/srv/app")}`).catch((cause: unknown) => cause);
  expect(error instanceof AdapterOutcomeError && error.outcome.kind === "error" && [error.outcome.code, error.outcome.status]).toEqual(["volume.bind.size-timeout", 504]);
  expect(catalogs["en-US"].volumes.backupCodes["volume_bind_size-timeout"]).toContain("snapshot size");
});
