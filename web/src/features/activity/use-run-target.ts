// Source: docklet-next/issues/584
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import { useHosts } from "@/data/queries";
import type { ActionRun } from "@/domain/model";

/**
 * Host load, rename and removal are reflected in every run surface. Every run names its host; a resource run also names
 * its resource, while a scheduler job's target would only repeat its title.
 */
export function useRunTarget(run: ActionRun): string {
  const hosts = useHosts();
  const host = run.hostId === undefined ? undefined : hosts.data?.find((h) => h.id === run.hostId)?.name;
  if (run.hostScoped) return host ?? "";
  return [run.operationTarget ? "" : run.target, host].filter(Boolean).join(" · ");
}
