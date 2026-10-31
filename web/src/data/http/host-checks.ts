// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original scoped typed checks using the backend activity-producing route.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import { unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { runtimeError, runtimeResponseError } from "./runtime-mapping";

import type { HostCheckResult } from "../api";
interface Options {
  baseUrl: string; client: CsrfClient;
  capability: (op: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number; changed: () => void;
}
export function createHostChecks({ baseUrl, client, capability, generation, changed }: Options) {
  async function check(operation: string, hostId: string): Promise<HostCheckResult> {
    const owner = generation(), current = () => { if (owner !== generation()) throw new CancelledError(); };
    const ref: ResourceRef = { kind: "host", id: hostId };
    const cap = await capability(operation, ref); current();
    if (!cap.available) throw unavailable(operation, ref, cap.disabledReasonCode);
    try {
      const response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(hostId)}/test`, { method: "POST", credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } });
      current();
      const body = await response.json(); current();
      if (!response.ok) throw runtimeResponseError(body, response.status);
      const data = body.data as components["schemas"]["EnvironmentTest"];
      if (body.success !== true || (data?.status !== "online" && data?.status !== "standby-opened")) throw runtimeError("hosts.invalid-response");
      return { status: data.status };
    } finally { if (owner === generation()) changed(); }
  }
  return { connectHost: (id: string) => check("connectHost", id), reconnectHost: (id: string) => check("reconnectHost", id) };
}
