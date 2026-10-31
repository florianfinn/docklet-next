// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original per-host component metadata from existing reports, version and image hints.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import { isHostAvailable } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import type { SystemComponent } from "@/domain/system";
import { AdapterOutcomeError, type CapabilityDescriptor, type ResourceRef } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { runtimeResponseError } from "./runtime-mapping";
interface Options { baseUrl: string; client: CsrfClient; hosts: () => Promise<Host[]>; capability: (op: string, ref: ResourceRef) => Promise<CapabilityDescriptor>; generation: () => number }
export function createSystemComponents({ baseUrl, client, hosts, capability, generation }: Options) {
  return async (): Promise<SystemComponent[]> => {
    const owner = generation(), current = () => { if (owner !== generation()) throw new CancelledError(); };
    const items = await hosts(); current();
    return Promise.all(items.map(async host => {
      let info = host.componentVersion;
      let toolsImage = info?.toolsImage;
      let errorCode: string | undefined;
      try {
        const cap = await capability("listSystemComponents", { kind: "host", id: host.id }); current();
        if (!cap.available) errorCode = cap.disabledReasonCode ?? "capability.unavailable";
        else if (host.id === "0") {
          const response = await client.fetch(`${baseUrl}/app-version`, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } });
          current(); const body = await response.json(); current();
          if (!response.ok) throw runtimeResponseError(body, response.status);
          info = body as components["schemas"]["VersionInfo"];
          if (!info || typeof info.currentVersion !== "string" || !Number.isInteger(info.agentContractVersion)) throw new Error("hosts.invalid-component");
        }
        if (!errorCode && isHostAvailable(host)) {
          const response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host.id)}/updater/system-image-hints`, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } });
          current(); const body = await response.json(); current();
          if (!response.ok) throw runtimeResponseError(body, response.status);
          const hints = body.data as components["schemas"]["UpdaterSystemImageHints"];
          if (body.success !== true || !Array.isArray(hints?.images)) throw new Error("hosts.invalid-component");
          toolsImage = hints.images.find(image => image.kind === "tools")?.reference ?? info?.toolsImage;
        }
        if (!errorCode) errorCode = host.status === "update-required" ? "agent.update-required" : host.status === "token-rejected" ? "agent.invalid-token" : host.status === "offline" ? "host.unreachable" : undefined;
      } catch (error) {
        current(); errorCode = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : "agent.unavailable";
      }
      return {
        hostId: host.id, role: "agent", metadataOnly: true,
        agentVersion: info?.currentVersion || host.agentVersion, contractVersion: info?.agentContractVersion,
        toolsImage, updateRequired: host.status === "update-required", errorCode,
        containerName: "", composeProject: "", image: { repository: info?.currentImageRef ?? "", tag: "", version: info?.currentVersion ?? "", published: "" },
        state: "unknown", installDir: host.installDir, insideProjectDir: false, labels: {}, ports: [], mounts: [], networks: [], restartPolicy: "", memLimitMb: 0, recentLog: [],
      };
    }));
  };
}
