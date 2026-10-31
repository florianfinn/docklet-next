// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original identity-fenced origin assignment and reset transport.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { DockletAdapter } from "../adapter";
import { unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";
import { runtimeResponseError } from "./runtime-mapping";

export function createContainerOrigin(options: {
  baseUrl: string; client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number; changed: () => void;
}): Pick<DockletAdapter, "setContainerOrigin"> {
  return {
    async setContainerOrigin(id, origin) {
      const generation = options.generation();
      const check = () => { if (generation !== options.generation()) throw new CancelledError(); };
      const [host, dockerId] = splitInventoryId(id);
      const ref: ResourceRef = { kind: "container", id };
      const descriptor = await options.capability("setContainerOrigin", ref);
      check();
      if (!descriptor.available) throw unavailable("setContainerOrigin", ref, descriptor.disabledReasonCode);
      const reset = origin === "unknown";
      if (!reset && origin !== "unraid" && origin !== "standalone") throw runtimeResponseError({ code: "container.origin-locked" }, 409);
      const body: components["schemas"]["ContainerOriginAssignment"] = { origin: origin as "unraid" | "standalone", dockerId };
      const response = await options.client.fetch(`${options.baseUrl}/environments/${encodeURIComponent(host)}/containers/${encodeURIComponent(dockerId)}/origin`, {
        method: reset ? "DELETE" : "PUT", headers: { Accept: "application/json", ...(!reset ? { "Content-Type": "application/json" } : {}) },
        ...(!reset ? { body: JSON.stringify(body) } : {}),
      });
      check();
      const result: unknown = await response.json();
      check();
      if (!response.ok) throw runtimeResponseError(result, response.status);
      options.changed();
    },
  };
}
