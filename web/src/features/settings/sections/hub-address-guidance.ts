// Source: docklet-next/issues/572
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Edge hub address guidance and effect-synchronised acceptance coverage.; M36 #572: Edge hub address guidance.

import type { HubDiscovery } from "@/domain/hub-discovery";

/**
 * Only bindings of the hub's listen port establish a missing-port warning; a portless address is valid once the
 * listen port is published on 80 (the onboarding default applies the same rule).
 */
export function hubAddressPortWarning(address: string, discovery?: HubDiscovery): number | undefined {
  const authority = address.trim().replace(/^https?:\/\//i, "").split("/")[0] ?? "";
  if (!authority || /^(?:\[[^\]]+\]|[^:]+):\d+$/.test(authority)) return undefined;
  const listen = discovery?.container?.publishedPorts.filter((p) => p.containerPort === discovery.container!.listenPort) ?? [];
  return listen.some((p) => p.hostPort === 80) ? undefined : listen[0]?.hostPort;
}
