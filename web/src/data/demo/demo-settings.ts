import { pruneEvents, validateHistoryLimit } from "@/domain/activity-history";
import type { HubSettings, SettingsSection } from "@/domain/settings";

import type { SettingsApi } from "../api";
import { DemoError, wait, type DemoContext } from "./demo-context";
import { simulateRead } from "./simulation";

const SECTION_LABEL: Record<SettingsSection, string> = {
  general: "Allgemein",
  auth: "Anmeldung",
  users: "Benutzer",
  hostDefaults: "Host-Standards",
  updates: "Updates",
  backups: "Backupziele",
  security: "Sicherheitsscans",
  activity: "Aktivität",
};

export function createSettingsApi({ db, runner }: DemoContext): SettingsApi {
  const snapshot = (): HubSettings => ({ ...structuredClone(db.settings), security: { scanEnabled: db.security.enabled, schedule: db.security.schedule } });

  return {
    getSetupStatus: async () => db.settings.setupCompleted,
    getConnectedSettings: async () => ({ general: structuredClone(db.settings.general), activity: structuredClone(db.settings.activity), hostDefaults: structuredClone(db.settings.hostDefaults) }),
    getSettings: () => simulateRead("settings", snapshot),

    async saveSettings(section, value) {
      await wait(450);
      if (section === "users") {
        const users = value as HubSettings["users"];
        if (!users.some((u) => u.admin && u.access === "full")) throw new DemoError("last-admin");
      }
      if (section === "backups") {
        const ids = new Set((value as HubSettings["backups"]).targets.map((t) => t.id));
        if ([...db.backupPolicies.values()].some((p) => !ids.has(p.targetId))) throw new DemoError("target-in-use");
        if (!Number.isInteger((value as HubSettings["backups"]).defaultRetentionCount)) throw new DemoError("invalid-plan");
      }
      if (section === "security") {
        const sec = value as HubSettings["security"];
        db.security.enabled = sec.scanEnabled;
        db.security.schedule = sec.schedule;
        db.changed("security");
      }
      if (section === "activity") {
        const a = value as HubSettings["activity"];
        if (validateHistoryLimit("events", a.maxEvents) || validateHistoryLimit("actions", a.maxFinishedActions)) throw new DemoError("invalid-history");
      }
      db.settings = { ...db.settings, [section]: structuredClone(value) };
      db.pushEvent({ kind: "config", level: "info", title: `Hub-Einstellungen gespeichert: ${SECTION_LABEL[section]}` });
      if (section === "activity") {
        // Smaller limits apply at once: the oldest entries leave, running actions stay.
        db.events = pruneEvents(db.events, db.settings.activity.maxEvents);
        runner.prune();
        db.changed("events");
      }
      db.changed("settings");
    },

    async completeSetup(input) {
      await wait(600);
      db.settings = { ...db.settings, ...structuredClone(input), setupCompleted: true };
      db.pushEvent({ kind: "config", level: "ok", title: `Ersteinrichtung abgeschlossen: ${input.general.hubName}` });
      db.changed("settings");
    },

    async setContainerAutoUpdate(containerId, policy) {
      await wait(200);
      const c = db.standalone.find((x) => x.id === containerId);
      if (!c) throw new DemoError("not-found");
      db.patchContainer(containerId, (x) => ({ ...x, autoUpdate: policy }));
      db.changed("containers", "schedule");
    },
  };
}
