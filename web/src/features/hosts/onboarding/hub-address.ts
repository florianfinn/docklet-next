// Source: docklet-next/issues/572
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Edge hub address guidance and effect-synchronised acceptance coverage.; M36 #572: Edge hub address guidance.

import { preferredIpCandidate, type HubDiscovery } from "@/domain/hub-discovery";
import { agentHubUrl, type GeneralSettings } from "@/domain/settings";

/** A saved portless address is usable only when discovery confirms host port 80. */
export function defaultAgentHubUrl(general: GeneralSettings, discovery?: HubDiscovery): string {
  const authority = general.hubAddress.trim().replace(/^https?:\/\//i, "").split("/")[0] ?? "";
  const explicitPort = /^(?:\[[^\]]+\]|[^:]+):\d+$/.test(authority);
  const publishedOn80 = discovery?.container?.publishedPorts.some((p) => p.containerPort === discovery.container!.listenPort && p.hostPort === 80);
  if (authority && (explicitPort || publishedOn80)) return agentHubUrl(general);
  const candidates = discovery?.candidates;
  const preferred = preferredIpCandidate(candidates?.filter((c) => !!new URL(c.url).port));
  return (preferred ?? candidates?.[0])?.url ?? agentHubUrl(general);
}
