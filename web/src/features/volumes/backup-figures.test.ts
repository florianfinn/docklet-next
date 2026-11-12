// Source: docklet-next/docs/design/backup-completion.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regression for the /backups overview population (BC16, #329).

import { expect, it } from "vitest";
import type { BackupOverview, Volume } from "@/domain/volumes";
import { backupFigures, targetAddress } from "./backup-figures";

const consumer = { containerId: "c", containerName: "c", target: "/data", readOnly: false, running: true };
const entry = (id: string, extra: Partial<Volume> = {}): Volume => ({ id, name: id, hostId: id.split("~")[0]!, kind: "volume", source: "", sizeBytes: 0, createdAt: "", consumers: [consumer], labels: {}, ...extra });
const run = (status: "succeeded" | "failed" | "partial") => ({ id: "r", volumeId: "", hostId: "", startedAt: "", status, trigger: "scheduled" }) as unknown as Volume["lastBackup"];
const overview = (hostId: string, volumes: Volume[], unavailable = false): BackupOverview => ({ hostId, unavailable, timeZone: "UTC", authority: "a", setVersion: 1, targets: [], volumes });
const plan = { enabled: false, targetId: "local", schedule: { mode: "manual" as const }, retentionCount: 7, stopContainers: false };

it("counts plans, results and unprotected entries over available hosts only", () => {
  const overviews = [
    overview("a", [entry("a~ok", { backup: { ...plan, enabled: true, schedule: { mode: "daily", time: "03:00" } }, lastBackup: run("succeeded") }), entry("a~manual", { backup: plan, lastBackup: run("partial") }), entry("a~failed", { backup: plan, lastBackup: run("failed") })]),
    overview("b", [entry("b~hidden", { backup: plan, lastBackup: run("failed") })], true),
  ];
  const inventory = [
    entry("a~ok"), entry("a~idle"), entry("a~unused", { consumers: [] }), entry("a~gone", { volumeExists: false }),
    entry("a~bind:2f73", { kind: "bind", bind: { kind: "directory", eligible: true } }),
    entry("a~bind:2f66", { kind: "bind", bind: { kind: "file", eligible: true } }),
    entry("a~bind:2f65", { kind: "bind", bind: { kind: "directory", eligible: false, reason: "volume.bind.path-denied" } }),
    entry("a~bind:2f75", { kind: "bind", bind: { kind: "unknown", eligible: true } }),
    entry("a~bind:2f6e", { kind: "bind" }),
    entry("b~idle"), entry("c~loading"),
  ];
  const figures = backupFigures(overviews, inventory);
  expect({ ...figures, unprotected: figures.unprotected.map((v) => v.id) }).toEqual({ plans: 3, succeeded: 1, failed: 2, unprotected: ["a~idle", "a~bind:2f73", "a~bind:2f66"] });
});

it("shows remote addresses with share and subfolder and local ones with their host", () => {
  expect([
    targetAddress({ kind: "smb", server: "nas.example.com", share: "backups", path: "docklet" }),
    targetAddress({ kind: "nfs", server: "192.0.2.10", share: "/export/", path: "" }),
    targetAddress({ kind: "local", path: "/srv/backups", hostId: "0" }, "Hub"),
  ]).toEqual(["//nas.example.com/backups/docklet", "192.0.2.10:/export", "Hub · /srv/backups"]);
});
