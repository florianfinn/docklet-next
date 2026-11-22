// Source: docklet-next/issues/572
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Edge hub address guidance and effect-synchronised acceptance coverage.; M36 #572 scoped baseline, storage naming or address guidance.

import type { HubDiscovery } from "@/domain/hub-discovery";

/** Only bindings of the hub's listen port establish a missing-port warning. */
export function hubAddressPortWarning(address: string, discovery?: HubDiscovery): number | undefined {
  const authority = address.trim().replace(/^https?:\/\//i, "").split("/")[0] ?? "";
  if (!authority || /^(?:\[[^\]]+\]|[^:]+):\d+$/.test(authority)) return undefined;
  return discovery?.container?.publishedPorts.find((p) => p.containerPort === discovery.container!.listenPort && p.hostPort !== 80)?.hostPort;
}
