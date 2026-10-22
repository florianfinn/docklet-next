// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original core settings mapping for setup and connected hub sections.

import { discardResponse } from "./identity-reads";
import type { SettingsApi } from "@/data/api";
import { HISTORY_LIMITS } from "@/domain/activity-history";
import type { HubSettings, SettingsSection } from "@/domain/settings";
import { unavailable } from "../capability";
import type { CsrfClient } from "./csrf-client";

export type CoreSettings = Record<string, string>;

function field(values: CoreSettings, key: string): string {
  const value = values[key];
  if (value === undefined) throw new Error(`settings.missing.${key}`);
  return value;
}

function boolean(values: CoreSettings, key: string): boolean {
  const value = field(values, key);
  if (value !== "true" && value !== "false") throw new Error(`settings.invalid.${key}`);
  return value === "true";
}

export function createSettingsCore(baseUrl: string, client: CsrfClient): Pick<SettingsApi, "getSetupStatus" | "completeSetup" | "getConnectedSettings" | "saveSettings"> {
  async function read(): Promise<CoreSettings> {
    const response = await client.fetch(`${baseUrl}/environments/0/settings`, { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!response.ok) { discardResponse(response); throw new Error(`settings.read.${response.status}`); }
    const items = await response.json() as { key?: unknown; value?: unknown }[];
    if (!Array.isArray(items) || !items.every((item) => typeof item.key === "string" && typeof item.value === "string")) throw new Error("settings.invalid-response");
    return Object.fromEntries(items.map((item) => [item.key, item.value])) as CoreSettings;
  }

  async function write(values: CoreSettings): Promise<void> {
    const response = await client.fetch(`${baseUrl}/environments/0/settings`, {
      method: "PUT", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify(values),
    });
    discardResponse(response);
    if (!response.ok) throw new Error(`settings.write.${response.status}`);
  }

  return {
    async getSetupStatus() {
      const settings = await read();
      if (settings.setupCompleted !== "true" && settings.setupCompleted !== "false") throw new Error("settings.setup-status-missing");
      return settings.setupCompleted === "true";
    },
    async completeSetup(input: Pick<HubSettings, "general" | "auth" | "hostDefaults">) {
      await write({
        hubName: input.general.hubName,
        hubAddress: input.general.hubAddress,
        domainUrl: input.general.domainUrl,
        linkBase: input.general.linkBase,
        agentProjectsDirectory: input.hostDefaults.projectDir,
        agentInstallDirectory: input.hostDefaults.installDir,
        defaultHostConnection: input.hostDefaults.connection,
        defaultPuid: input.hostDefaults.puid,
        defaultPgid: input.hostDefaults.pgid,
        detectSocketGroup: String(input.hostDefaults.detectSocketGroup),
        setupCompleted: "true",
      });
    },
    async getConnectedSettings() {
      const values = await read();
      const linkBase = field(values, "linkBase");
      const actionPresentation = field(values, "actionPresentation");
      if (linkBase !== "auto" && linkBase !== "internal" && linkBase !== "domain") throw new Error("settings.invalid.linkBase");
      if (actionPresentation !== "dialog" && actionPresentation !== "toast") throw new Error("settings.invalid.actionPresentation");
      const connection = field(values, "defaultHostConnection");
      if (connection !== "direct" && connection !== "edge") throw new Error("settings.invalid.defaultHostConnection");
      const timeZone = field(values, "effectiveTimeZone");
      try { new Intl.DateTimeFormat("en", { timeZone }); } catch { throw new Error("settings.invalid.timeZone"); }
      return {
        general: { hubName: field(values, "hubName"), hubAddress: field(values, "hubAddress"), domainUrl: field(values, "domainUrl"), linkBase, timeZone },
        hostDefaults: { connection, projectDir: field(values, "agentProjectsDirectory"), installDir: field(values, "agentInstallDirectory"), puid: field(values, "defaultPuid"), pgid: field(values, "defaultPgid"), detectSocketGroup: boolean(values, "detectSocketGroup") },
        activity: { maxEvents: HISTORY_LIMITS.events.default, maxFinishedActions: HISTORY_LIMITS.actions.default, keepActionOutput: boolean(values, "keepActionOutput"), actionPresentation },
      };
    },
    async saveSettings<K extends SettingsSection>(section: K, value: HubSettings[K]) {
      if (section === "general") {
        const general = value as HubSettings["general"];
        await write({ hubName: general.hubName, hubAddress: general.hubAddress, domainUrl: general.domainUrl, linkBase: general.linkBase });
        return;
      }
      if (section === "activity") {
        const activity = value as HubSettings["activity"];
        await write({ keepActionOutput: String(activity.keepActionOutput), actionPresentation: activity.actionPresentation });
        return;
      }
      throw unavailable("saveSettings", undefined, "capability.mapping-unavailable");
    },
  };
}
