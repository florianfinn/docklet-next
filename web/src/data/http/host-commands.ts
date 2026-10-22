// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original live host credential lifecycle and discovery mapping.

import { discardResponse } from "./identity-reads";
import type { components } from "@/api/generated/schema";
import type { HostsApi, HubApi } from "@/data/api";
import { AdapterOutcomeError } from "@/data/capability";
import type { CsrfClient } from "./csrf-client";
import { mapHost } from "./hosts";

type Environment = components["schemas"]["EnvironmentEnvironment"];
type Discovery = components["schemas"]["EnvironmentHubDiscovery"];
const PINNED_IMAGE = /^[a-zA-Z0-9./:_-]+:\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:@sha256:[a-f0-9]{64})?$/;

export function createHostCommands(baseUrl: string, client: CsrfClient, requireCapability: (operation: string) => Promise<void>, changed: () => void): Pick<HostsApi, "registerHost" | "issueAgentToken" | "removeHost"> & HubApi {
  async function request<T>(path: string, method = "GET", body?: object): Promise<T> {
    const response = await client.fetch(`${baseUrl}${path}`, {
      method, credentials: "same-origin", cache: "no-store",
      headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) },
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (!response.ok) { discardResponse(response); throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status }); }
    const payload = await response.json() as { success?: boolean; data?: T };
    if (payload.success !== true || !payload.data) throw new Error("hosts.invalid-response");
    return payload.data;
  }
  function token(value: unknown): string {
    if (typeof value !== "string" || !value) throw new Error("hosts.missing-token");
    return value;
  }
  return {
    async registerHost(input) {
      await requireCapability("registerHost");
      const isEdge = input.connection === "edge";
      const apiUrl = isEdge ? "edge://pending" : new URL(input.address.includes("://") ? input.address : `http://${input.address}`).toString().replace(/\/$/, "");
      const row = await request<Environment>("/environments", "POST", { name: input.name, apiUrl, isEdge, useApiKey: true });
      const result = { host: mapHost(row), token: token(row.apiKey) };
      changed();
      return result;
    },
    async issueAgentToken(hostId) {
      await requireCapability("issueAgentToken");
      if (hostId === "0" || !hostId) throw new Error("hosts.invalid-host");
      const row = await request<Environment>(`/environments/${encodeURIComponent(hostId)}`, "PUT", { regenerateApiKey: true });
      const issued = token(row.apiKey);
      changed();
      return issued;
    },
    async removeHost(hostId) {
      await requireCapability("removeHost");
      if (hostId === "0" || !hostId) throw new Error("hosts.invalid-host");
      await request(`/environments/${encodeURIComponent(hostId)}`, "DELETE");
      changed();
    },
    async discoverHub() {
      await requireCapability("discoverHub");
      const discovery = await request<Discovery>("/hub/discovery");
      if (!Number.isFinite(Date.parse(discovery.detectedAt)) || !PINNED_IMAGE.test(discovery.agentImage) || !Array.isArray(discovery.candidates)) throw new Error("hosts.invalid-discovery");
      for (const candidate of discovery.candidates) {
        const url = new URL(candidate.url);
        if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash || !["", "/"].includes(url.pathname) || !["unknown", "same-host"].includes(candidate.reach) || !["configured", "interface"].includes(candidate.source)) throw new Error("hosts.invalid-candidate");
      }
      return { ...discovery, candidates: discovery.candidates, interfaces: [], hostAddresses: [], simulated: false };
    },
  };
}
