// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original core settings mapping for first-run setup.

import type { HubSettings } from "@/domain/settings";
import type { CsrfClient } from "./csrf-client";

export type CoreSettings = Record<string, string>;

export function createSettingsCore(baseUrl: string, client: CsrfClient) {
  async function read(): Promise<CoreSettings> {
    const response = await client.fetch(`${baseUrl}/environments/0/settings`, { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`settings.read.${response.status}`);
    const items = await response.json() as { key?: unknown; value?: unknown }[];
    if (!Array.isArray(items) || !items.every((item) => typeof item.key === "string" && typeof item.value === "string")) throw new Error("settings.invalid-response");
    return Object.fromEntries(items.map((item) => [item.key, item.value])) as CoreSettings;
  }

  async function write(values: CoreSettings): Promise<void> {
    const response = await client.fetch(`${baseUrl}/environments/0/settings`, {
      method: "PUT", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify(values),
    });
    if (!response.ok) throw new Error(`settings.write.${response.status}`);
  }

  return {
    read,
    write,
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
        timeZone: input.general.timeZone,
        projectsDirectory: input.hostDefaults.projectDir,
        agentInstallDirectory: input.hostDefaults.installDir,
        defaultHostConnection: input.hostDefaults.connection,
        defaultPuid: input.hostDefaults.puid,
        defaultPgid: input.hostDefaults.pgid,
        detectSocketGroup: String(input.hostDefaults.detectSocketGroup),
        setupCompleted: "true",
      });
    },
  };
}
