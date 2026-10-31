// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original live host credential lifecycle and discovery mapping. M14 host maintenance and availability.

import { discardResponse } from "./identity-reads";
import { runtimeResponseError } from "./runtime-mapping";
import { CancelledError } from "@tanstack/react-query";
import type { CapabilityDescriptor, ResourceRef } from "../capability";
import type { components } from "@/api/generated/schema";
import type { HostsApi, HubApi } from "@/data/api";
import { AdapterOutcomeError } from "@/data/capability";
import type { CsrfClient } from "./csrf-client";
import { mapHost } from "./hosts";

type Environment = components["schemas"]["EnvironmentEnvironment"];
type Discovery = components["schemas"]["EnvironmentHubDiscovery"];
const PINNED_IMAGE = /^[a-zA-Z0-9./:_-]+:\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:@sha256:[a-f0-9]{64})?$/;

export function createHostCommands(baseUrl: string, client: CsrfClient, requireCapability: (operation: string) => Promise<void>, changed: () => void, capability?: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>, generation: () => number = () => 0): Pick<HostsApi, "registerHost" | "issueAgentToken" | "removeHost" | "updateHost"> & HubApi {
  async function request<T>(path: string, method = "GET", body?: object): Promise<T> {
    const owner = generation();
    const response = await client.fetch(`${baseUrl}${path}`, {
      method, credentials: "same-origin", cache: "no-store",
      headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) },
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (owner !== generation()) throw new CancelledError();
    if (!response.ok && !/\bjson\b/i.test(response.headers.get("Content-Type") ?? "")) {
      discardResponse(response);
      throw runtimeResponseError(undefined, response.status);
    }
    const payload = await response.json();
    if (owner !== generation()) throw new CancelledError();
    if (!response.ok) throw runtimeResponseError(payload, response.status);
    if (payload.success !== true || !payload.data) throw new Error("hosts.invalid-response");
    return payload.data;
  }
  async function access(operation: string) {
    const owner = generation();
    await requireCapability(operation);
    if (owner !== generation()) throw new CancelledError();
  }
  function token(value: unknown): string {
    if (typeof value !== "string" || !value) throw new Error("hosts.missing-token");
    return value;
  }
  return {
    async updateHost(hostId, input) {
      const owner = generation();
      const ref: ResourceRef = { kind: "host", id: hostId };
      const cap = await capability?.("updateHost", ref);
      if (owner !== generation()) throw new CancelledError();
      if (!cap?.available) throw new AdapterOutcomeError({ kind: "not-available", operationId: "updateHost", resourceRef: ref, reasonCode: cap?.disabledReasonCode ?? "capability.unavailable" });
      const row = await request<Environment>(`/environments/${encodeURIComponent(hostId)}`, "PUT", { name: input.name.trim(), color: input.color?.toLowerCase() ?? null });
      changed();
      return mapHost(row);
    },
    async registerHost(input) {
      await access("registerHost");
      const isEdge = input.connection === "edge";
      const apiUrl = isEdge ? "edge://pending" : new URL(input.address.includes("://") ? input.address : `http://${input.address}`).toString().replace(/\/$/, "");
      const row = await request<Environment>("/environments", "POST", { name: input.name, apiUrl, isEdge, useApiKey: true });
      const result = { host: mapHost(row), token: token(row.apiKey) };
      changed();
      return result;
    },
    async issueAgentToken(hostId) {
      await access("issueAgentToken");
      if (hostId === "0" || !hostId) throw new Error("hosts.invalid-host");
      const row = await request<Environment>(`/environments/${encodeURIComponent(hostId)}`, "PUT", { regenerateApiKey: true });
      const issued = token(row.apiKey);
      changed();
      return issued;
    },
    async removeHost(hostId) {
      await access("removeHost");
      if (hostId === "0" || !hostId) throw new Error("hosts.invalid-host");
      await request(`/environments/${encodeURIComponent(hostId)}`, "DELETE");
      changed();
    },
    async discoverHub() {
      await access("discoverHub");
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
