// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original narrow host defaults and component producer seam tests. M14 host maintenance and availability.

import { expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { mapHost } from "./hosts";
import { createSettingsCore } from "./settings-core";
import { createSystemComponents } from "./system-components";
const version = { currentVersion: "1.2.3", agentContractVersion: 11, toolsImage: "registry.example.com/tools:1.2.3" } as components["schemas"]["VersionInfo"];
const defaults = { projectDir: "/srv/projects", installDir: "/srv/agent", connection: "edge" as const, puid: "1000", pgid: "1000", detectSocketGroup: true };
it("H4 saves only host-default keys and preserves the typed settings refusal", async () => {
  const fetcher = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    expect(String(url)).toBe("/api/environments/0/settings"); expect(init?.method).toBe("PUT");
    expect(new Headers(init?.headers).has("Idempotency-Key")).toBe(false);
    expect(JSON.parse(String(init?.body))).toEqual({ agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/agent", defaultHostConnection: "edge", defaultPuid: "1000", defaultPgid: "1000", detectSocketGroup: "true" });
    return new Response(JSON.stringify({ code: "settings.ui-configuration-disabled", status: 403 }), { status: 403, headers: { "Content-Type": "application/problem+json" } });
  });
  const api = createSettingsCore("/api", { fetch: fetcher as typeof fetch, token: async () => "example" });
  await expect(api.saveSettings("hostDefaults", { ...defaults, unexpected: "must-not-be-sent" } as typeof defaults)).rejects.toMatchObject({ outcome: { code: "settings.ui-configuration-disabled", status: 403 } });
});
it("H4 resolves component version, contract and tools data with isolated host errors", async () => {
  const hosts = ["0", "example", "old"].map(id => mapHost({ id, name: id, color: null, apiUrl: "http://agent.example.com", enabled: true, isEdge: id === "example", status: id === "old" ? "update-required" : "online", ...(id !== "0" ? { agentReport: { reportedAt: "2026-10-31T00:00:00Z", version: { ...version, ...(id === "old" ? { agentContractVersion: 10 } : {}) } } } : {}) }));
  const fetcher = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
    expect(init?.method).toBeUndefined();
    if (String(url) === "/api/app-version") return new Response(JSON.stringify(version));
    if (String(url) === "/api/environments/0/updater/system-image-hints") return new Response(JSON.stringify({ success: true, data: { hubVersion: "1.2.3", images: [{ kind: "tools", reference: version.toolsImage, manualOnly: true, requiresHubUpdate: false, state: "pinned" }] } }));
    expect(String(url)).toBe("/api/environments/example/updater/system-image-hints");
    return new Response(JSON.stringify({ code: "agent.ui-configuration-disabled", status: 502 }), { status: 502 });
  });
  const read = createSystemComponents({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, hosts: async () => hosts, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), generation: () => 0 });
  const rows = await read();
  expect(rows[0]).toMatchObject({ hostId: "0", contractVersion: 11, agentVersion: "1.2.3", toolsImage: version.toolsImage, metadataOnly: true });
  expect(rows[1]).toMatchObject({ hostId: "example", contractVersion: 11, errorCode: "agent.ui-configuration-disabled" });
  expect(rows[2]).toMatchObject({ hostId: "old", contractVersion: 10, updateRequired: true, errorCode: "agent.update-required" });
  expect(fetcher).toHaveBeenCalledTimes(3);
});
it("H4 reads only the settings needed by Hosts and installation controls", async () => {
  const values = { agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/agent", defaultHostConnection: "edge", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true", hubAddress: "https://hub.example.com", effectiveTimeZone: "UTC", actionPresentation: "invalid", stackStopMode: "invalid", linkBase: "invalid" };
  const api = createSettingsCore("/api", { fetch: (async () => new Response(JSON.stringify(Object.entries(values).map(([key, value]) => ({ key, value }))))) as typeof fetch, token: async () => "example" });
  expect(await api.getConnectedSettings("hostDefaults")).toEqual({ hostDefaults: { ...defaults, puid: "", pgid: "" } });
  expect(await api.getConnectedSettings("installation")).toMatchObject({ hostDefaults: { connection: "edge" }, general: { hubAddress: values.hubAddress, timeZone: "UTC" } });
  await expect(api.getConnectedSettings()).rejects.toThrow("settings.invalid.linkBase");
});
