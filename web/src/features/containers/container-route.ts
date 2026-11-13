// Source: docklet-next/issues/477
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original container page address resolution.

import type { ContainerInfo } from "@/domain/model";

/**
 * The container a page address names: its inventory ID first, else the exact container name on
 * the address's host (`<host>~<name>`). Docker names are unique per daemon, so a name link stays
 * valid when a recreate gives the container a new ID.
 */
export function findRouteContainer(containers: readonly ContainerInfo[] | undefined, ref: string): ContainerInfo | undefined {
  const byId = containers?.find((x) => x.id === ref);
  if (byId || !containers) return byId;
  const split = ref.indexOf("~");
  if (split <= 0) return undefined;
  const hostId = ref.slice(0, split), name = ref.slice(split + 1);
  return containers.find((x) => x.hostId === hostId && x.name === name);
}
