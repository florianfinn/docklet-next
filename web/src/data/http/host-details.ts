// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original host detail read (Docker info and the hub's own project directory) for M37 #579.

import type { DockletAdapter } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import { readSettingsList } from "./settings-core";
import type { CsrfClient } from "./csrf-client";

const text = (value: unknown) => (typeof value === "string" && value.trim() ? value.trim() : undefined);

/** Docker info is a flat body: OperatingSystem names the host system, ServerVersion the daemon; the hub's own Docker ("0") adds its project directory. */
export function createHostDetails(baseUrl: string, client: CsrfClient): Pick<DockletAdapter, "getHostDetails"> {
  return {
    async getHostDetails(hostId, signal) {
      const local = hostId === "0";
      const [info, settings] = await Promise.all([
        client.fetch(`${baseUrl}/environments/${encodeURIComponent(hostId)}/system/docker/info`, { headers: { Accept: "application/json" }, cache: "no-store", signal }).then(async (response) => {
          if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status });
          const body: unknown = await response.json();
          if (!body || typeof body !== "object" || Array.isArray(body)) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
          return body as Record<string, unknown>;
        }),
        local ? readSettingsList(baseUrl, client).catch(() => undefined) : undefined,
      ]);
      signal?.throwIfAborted();
      return { os: text(info.OperatingSystem), dockerVersion: text(info.ServerVersion), ...(local ? { projectDir: text(settings?.projectsDirectory) } : {}) };
    },
  };
}
