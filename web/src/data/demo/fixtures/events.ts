import type { ActionRun, ActionStep, OpsEvent, RestorePoint, ScheduledJob } from "@/domain/model";

import { COMPOSE } from "./compose-files";
import { img } from "./stacks";
import { daysAgo, hoursAgo, minutesAgo, minutesAhead } from "./time";

export const EVENTS: OpsEvent[] = [
  { id: "e1", at: minutesAgo(9), kind: "health", level: "error", title: "minecraft ist unhealthy (Healthcheck 3× fehlgeschlagen)", hostId: "boreal", stackId: "craft" },
  { id: "e2", at: minutesAgo(38), kind: "restart", level: "warn", title: "minecraft automatisch neu gestartet (Auto Heal 3/3)", hostId: "boreal", stackId: "craft" },
  { id: "e3", at: hoursAgo(2.2), kind: "host", level: "warn", title: "Cedar hat die Verbindung verloren", hostId: "cedar" },
  { id: "e4", at: hoursAgo(1.5), kind: "update", level: "info", title: "Update-Prüfung: 8 Images mit neuem Stand", hostId: "atlas" },
  { id: "e5", at: hoursAgo(7), kind: "scan", level: "info", title: "Schwachstellenscan abgeschlossen (13 Images)", hostId: "boreal" },
  { id: "e6", at: hoursAgo(9.5), kind: "backup", level: "ok", title: "Backup cloud_data abgeschlossen (NFS)", hostId: "boreal", stackId: "cloud" },
  { id: "e7", at: hoursAgo(10), kind: "backup", level: "ok", title: "Backup media_config abgeschlossen (lokal)", hostId: "atlas", stackId: "media" },
  { id: "e8", at: hoursAgo(13), kind: "deploy", level: "error", title: "Update Craft fehlgeschlagen: Zustandsprüfung", hostId: "boreal", stackId: "craft" },
  { id: "e9", at: hoursAgo(16), kind: "config", level: "info", title: "compose.yaml von Monitoring gespeichert", hostId: "boreal", stackId: "monitoring" },
  { id: "e10", at: hoursAgo(19), kind: "deploy", level: "ok", title: "Redeploy DNS abgeschlossen", hostId: "delta", stackId: "dns" },
  { id: "e11", at: hoursAgo(21), kind: "restart", level: "info", title: "paperless webserver neu gestartet", hostId: "atlas", stackId: "paperless" },
  { id: "e12", at: hoursAgo(23), kind: "backup", level: "ok", title: "Backup craft_world abgeschlossen (SMB)", hostId: "boreal", stackId: "craft" },
];

export const SCHEDULE: ScheduledJob[] = [
  { id: "j1", kind: "update-check", title: "Update-Prüfung aller Hosts", nextRunAt: minutesAhead(52), cadence: "alle 6 Stunden" },
  { id: "j2", kind: "backup", title: "Backup media_config", hostId: "atlas", stackId: "media", volumeId: "vol-atlas-media_media_config", nextRunAt: minutesAhead(4 * 60 + 10), cadence: "täglich 02:00" },
  { id: "j3", kind: "scan", title: "Schwachstellenscan", nextRunAt: minutesAhead(5 * 60 + 10), cadence: "täglich 03:00" },
  { id: "j4", kind: "auto-update", title: "Automatische Updates · Paperless", hostId: "atlas", stackId: "paperless", nextRunAt: minutesAhead(5 * 60 + 40), cadence: "Mi 03:30" },
  { id: "j5", kind: "backup", title: "Backup cloud_data", hostId: "boreal", stackId: "cloud", volumeId: "vol-boreal-cloud_cloud_data", nextRunAt: minutesAhead(4 * 60 + 25), cadence: "täglich 02:15" },
  { id: "j6", kind: "auto-update", title: "Automatische Updates · Cloud", hostId: "boreal", stackId: "cloud", nextRunAt: minutesAhead(3 * 1440 + 200), cadence: "So 04:00" },
];

function steps(defs: [ActionStep["key"], string, ActionStep["state"], string?][], start: string): ActionStep[] {
  const t0 = new Date(start).getTime();
  return defs.map(([key, label, state, detail], i) => ({
    key,
    label,
    state,
    detail,
    startedAt: new Date(t0 + i * 4000).toISOString(),
    finishedAt: state === "pending" ? undefined : new Date(t0 + i * 4000 + 3500).toISOString(),
  }));
}

const craftFailStart = hoursAgo(13);
const dnsStart = hoursAgo(19);
const cloudStart = daysAgo(2);

/** Finished runs that make up deploy histories. */
export const SEED_RUNS: Omit<ActionRun, "output">[] = [
  {
    id: "run-craft-1",
    kind: "update",
    title: "Update · Craft",
    target: "Craft · Boreal",
    hostId: "boreal",
    stackId: "craft",
    services: ["minecraft"],
    status: "failed",
    progress: 80,
    startedAt: craftFailStart,
    finishedAt: new Date(new Date(craftFailStart).getTime() + 19500).toISOString(),
    error: "minecraft wurde nach 3 Versuchen nicht healthy (mc-health: timeout).",
    restorePointId: "rp-craft-1",
    steps: steps(
      [
        ["validate", "Prüfung", "done", "Compose gültig, Boreal erreichbar"],
        ["pull", "Image-Pull", "done", "itzg/minecraft-server 2025.9.0"],
        ["create", "Austausch", "done", "1 Container ersetzt"],
        ["start", "Start", "done", "minecraft"],
        ["health", "Zustandsprüfung", "error", "mc-health: timeout"],
      ],
      craftFailStart,
    ),
    log: ["Vorheriger Stand gesichert (Image 2025.8.1, compose.yaml)", "Pull abgeschlossen", "Container ersetzt", "Gestartet", "Fehler: Healthcheck nicht bestanden"],
  },
  {
    id: "run-dns-1",
    kind: "redeploy",
    title: "Redeploy · DNS",
    target: "DNS · Delta",
    hostId: "delta",
    stackId: "dns",
    services: ["adguard", "unbound"],
    status: "succeeded",
    progress: 100,
    startedAt: dnsStart,
    finishedAt: new Date(new Date(dnsStart).getTime() + 19800).toISOString(),
    steps: steps(
      [
        ["validate", "Prüfung", "done"],
        ["pull", "Image-Pull", "done"],
        ["create", "Austausch", "done"],
        ["start", "Start", "done"],
        ["health", "Zustandsprüfung", "done"],
      ],
      dnsStart,
    ),
    log: ["Abgeschlossen"],
  },
  {
    id: "run-cloud-1",
    kind: "update",
    title: "Update · Cloud",
    target: "Cloud · Boreal",
    hostId: "boreal",
    stackId: "cloud",
    services: ["nextcloud"],
    status: "succeeded",
    progress: 100,
    startedAt: cloudStart,
    finishedAt: new Date(new Date(cloudStart).getTime() + 21000).toISOString(),
    restorePointId: "rp-cloud-1",
    steps: steps(
      [
        ["validate", "Prüfung", "done"],
        ["pull", "Image-Pull", "done", "nextcloud 31.0.9"],
        ["create", "Austausch", "done"],
        ["start", "Start", "done"],
        ["health", "Zustandsprüfung", "done"],
      ],
      cloudStart,
    ),
    log: ["Abgeschlossen"],
  },
];

export const SEED_RESTORE_POINTS: RestorePoint[] = [
  {
    id: "rp-craft-1",
    stackId: "craft",
    createdAt: craftFailStart,
    reason: "Vor Update · Craft",
    images: [{ service: "minecraft", image: img("itzg/minecraft-server", "2025.8.1", "2025.8.1", 60) }],
    composeSnapshot: (COMPOSE.craft ?? "").replace("2025.9.0", "2025.8.1"),
  },
  {
    id: "rp-cloud-1",
    stackId: "cloud",
    createdAt: cloudStart,
    reason: "Vor Update · Cloud",
    images: [{ service: "nextcloud", image: img("nextcloud", "31.0.8-apache", "31.0.8", 50) }],
    composeSnapshot: (COMPOSE.cloud ?? "").replace("31.0.9-apache", "31.0.8-apache"),
  },
];
