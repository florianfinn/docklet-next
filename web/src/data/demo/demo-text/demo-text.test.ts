import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { sampleRun } from "@/features/activity/sample-run";
import { MINECRAFT, PLEX_STREAMS } from "@/features/dashboard/services/service-data";

import { createDemoAdapter } from "../demo-adapter";
import { renderEvent, renderRun } from "../../message-code";
import { queueDemoFailure } from "../options";
import { STANDALONE } from "../fixtures/containers";
import { EVENTS, SCHEDULE, SEED_RESTORE_POINTS, SEED_RUNS } from "../fixtures/events";
import { HOSTS } from "../fixtures/hosts";
import { SECURITY } from "../fixtures/security";
import { SETTINGS } from "../fixtures/settings";
import { STACKS } from "../fixtures/stacks";
import { agentContainer } from "../fixtures/system";
import { simulateImport } from "../import-simulator";
import { SIMULATED_NOTE } from "../run-output";
import { simulationStore } from "../simulation";
import { localizeDemoText, localizeEvents, localizeRun } from ".";

const en = (s: string) => localizeDemoText(s, "en-US");

// User-given names stay as stored; everything else must read as English.
const NAMES = SETTINGS.backups.targets.map((t) => t.name);
const GERMAN = /[äöüÄÖÜß„]|\b(und|oder|nicht|der|die|das|den|dem|mit|von|für|auf|ist|sind|nach|bei|aus|zum|zur|noch|alle|ohne|unter|wird|wurde|keine?|prüfen|abgeschlossen|fehlgeschlagen|gestartet|gestoppt|entfernt|angelegt|verbunden|gesichert|gelöscht|neu|Fehler|Prüfung|Simulierte?r?|Stand|Ziel|Vorlage|Einstellungen|bleibt|meldet|Neustart|Wiederherstellung|lokal|täglich|Stunden|synthetisch|Beispiel|Austausch|Stopp)\b/;
const germanIn = (texts: (string | undefined)[]) =>
  texts.filter((t): t is string => !!t).map((t) => NAMES.reduce((s, n) => s.replaceAll(n, ""), en(t))).filter((t) => GERMAN.test(t));

const runTexts = (r: { title: string; target: string; error?: string; steps: { label: string; detail?: string }[]; log: string[] }) => [r.title, r.target, r.error, ...r.steps.flatMap((s) => [s.label, s.detail]), ...r.log];

describe("demo texts in American English", () => {
  it("leave German untouched and pass unknown texts through", () => {
    for (const s of ["Prüfung", "Update · Craft", "Backup cloud_data abgeschlossen (NFS)"]) expect(localizeDemoText(s, "de")).toBe(s);
    expect(en("nginx:1.27, redis:7")).toBe("nginx:1.27, redis:7");
    expect(en("Atlas")).toBe("Atlas");
    expect(localizeDemoText(undefined, "en-US")).toBeUndefined();
  });

  it("translate steps, titles, events and errors with their values", () => {
    expect(en("Zustandsprüfung")).toBe("Health check");
    expect(en("Update · Craft")).toBe("Update · Craft");
    expect(en("Vor Update · Craft")).toBe("Before update · Craft");
    expect(en("Update-Prüfung · Alle Hosts")).toBe("Update check · All hosts");
    expect(en("3 Container ersetzt")).toBe("3 containers replaced");
    expect(en("1 Container ersetzt")).toBe("1 container replaced");
    expect(en("Fehler bei Zustandsprüfung: web bleibt unhealthy (Healthcheck: timeout)")).toBe("Error at Health check: web stays unhealthy (healthcheck: timeout)");
    expect(en("Image-Pull – nginx:1.27")).toBe("Image pull – nginx:1.27");
    expect(en("Stand „Vor Update · Cloud“ vorhanden")).toBe("State “Before update · Cloud” available");
    expect(en("Update Craft fehlgeschlagen: Zustandsprüfung")).toBe("Update Craft failed: Health check");
    expect(en("web mit cloud_default verbunden (172.20.0.9)")).toBe("web connected to cloud_default (172.20.0.9)");
    expect(en("Netzwerk lan (macvlan) angelegt")).toBe("Network lan (macvlan) created");
    expect(en("Backup von media_config abgeschlossen · 2 ältere(s) Archiv(e) gemäß Aufbewahrung entfernt")).toBe("Backup of media_config completed · 2 older archives removed per retention");
    expect(en("Mi 03:30")).toBe("Wed 03:30");
    expect(en("Schreibbar, 3,1 TB frei")).toBe("Writable, 3.1 TB free");
  });

  it("cover every user-visible fixture text", () => {
    const texts = [
      ...EVENTS.map((e) => e.title),
      ...SCHEDULE.flatMap((j) => [j.title, j.cadence]),
      ...SEED_RUNS.flatMap(runTexts),
      ...SEED_RESTORE_POINTS.map((p) => p.reason),
      ...SECURITY.findings.map((f) => f.title),
      ...STACKS.flatMap((s) => [s.description, ...s.services.map((x) => (x.update.state === "error" ? x.update.message : undefined))]),
      ...STANDALONE.map((c) => c.originNote),
      ...HOSTS.flatMap((h) => [h.address, agentContainer(h).originNote]),
      ...PLEX_STREAMS.flatMap((s) => [s.subtitle, s.device]),
      ...runTexts(sampleRun(new Date(0).toISOString())),
      SIMULATED_NOTE,
      ...["example/notes-app", "example/other"].flatMap((ref) => {
        const r = simulateImport({ kind: "github", ref });
        return [r.title, ...r.found, ...r.missing];
      }),
      ...["traefik/whoami", "linuxserver/syncthing", "library/redis"].flatMap((ref) => {
        const r = simulateImport({ kind: "dockerhub", ref });
        return [r.title, ...r.found, ...r.missing];
      }),
    ];
    expect(texts.length).toBeGreaterThan(100);
    expect(germanIn(texts)).toEqual([]);
    // Content names (media titles, the server's message of the day) are data, not interface text.
    expect(en(MINECRAFT.motd)).toBe(MINECRAFT.motd);
  });

  it("keep one localized object per run so live selectors stay stable", () => {
    const run = { ...SEED_RUNS[0]!, output: [{ at: "2026-10-12T00:00:00Z", stream: "note" as const, text: SIMULATED_NOTE }, { at: "2026-10-12T00:00:01Z", stream: "stdout" as const, text: "Compose-Datei gültig" }] };
    const a = localizeRun(run, "en-US");
    expect(localizeRun(run, "en-US")).toBe(a);
    expect(localizeRun(run, "de")).toBe(run);
    expect(a.output.map((l) => l.text)).toEqual(["Simulated output – no real Docker call, values are made up.", "Compose-Datei gültig"]);
  });
});

describe("texts the demo backend writes while running actions", () => {
  beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 20 })));
  afterEach(() => {
    simulationStore.reset();
    localStorage.clear();
  });

  it("all read as English after localization", async () => {
    const adapter = createDemoAdapter();
    const settings = await adapter.getSettings();
    const started = await Promise.allSettled([
      adapter.runStackCommand("media", "restart"),
      adapter.runStackCommand("paperless", "redeploy"),
      adapter.runStackCommand("monitoring", "stop"),
      (() => { queueDemoFailure(adapter, { operation: "runStackCommand", at: "pull" }); return adapter.runStackCommand("dns", "start"); })(),
      adapter.installStackUpdates("cloud"),
      adapter.checkUpdates({}),
      adapter.restartService("craft", "minecraft"),
      adapter.runScan(),
      adapter.reconnectHost("cedar"),
      adapter.restartAgent("cedar"),
      adapter.pullImage({ hostId: "atlas", reference: "nginx:1.27" }),
      (() => { queueDemoFailure(adapter, { operation: "pullImage" }); return adapter.pullImage({ hostId: "boreal", reference: "example/missing" }); })(),
      adapter.pruneImages({ hostIds: ["atlas"], mode: "unused" }),
      adapter.runBackup("vol-atlas-paperless_pg_data"),
      adapter.runContainerCommand(STANDALONE[0]!.id, "restart"),
      adapter.saveSettings("general", settings.general),
      adapter.testBackupTarget(settings.backups.targets[0]!.id, "atlas"),
      adapter.saveNotificationTarget({ provider: "gotify", enabled: true, config: { host: "gotify.example.com", token: "example", disableTls: false, insecureSkipVerify: false, useHeader: false }, events: ["deployFailed"] }),
    ]);
    expect(started.filter((s) => s.status === "fulfilled").length).toBeGreaterThan(14);
    await waitFor(async () => expect((await adapter.listActions()).filter((r) => r.status === "running")).toEqual([]), { timeout: 15000 });

    const codedRuns = await adapter.listActions();
    const codedEvents = await adapter.listEvents();
    const runs = codedRuns.map((run) => renderRun(run, "de"));
    const events = codedEvents.map((event) => renderEvent(event, "de"));
    const results = started.flatMap((s) => (s.status === "fulfilled" && s.value && typeof s.value === "object" && "message" in s.value && typeof s.value.message === "string" ? [s.value.message] : []));
    expect(runs.length).toBeGreaterThan(12);
    expect(germanIn([...runs.flatMap(runTexts), ...events.map((e) => e.title), ...results])).toEqual([]);
    expect(codedRuns.map((run) => renderRun(run, "en-US"))).toEqual(runs.map((run) => localizeRun(run, "en-US")));
    expect(codedEvents.map((event) => renderEvent(event, "en-US"))).toEqual(localizeEvents(events, "en-US"));
  });
});
