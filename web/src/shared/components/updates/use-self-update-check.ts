// Source: docklet-next/issues/616
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original shared self-update check read for Settings › Updates and host cards.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import type { Host } from "@/domain/model";

type HostOwner = Pick<Host, "identity" | "agentVersion">;
/**
 * Agent checks belong to the host's current registration, session and reported version: a replacement or
 * reconnect selects a new entry, so a superseded answer is never shown for it. The hub keeps its key,
 * which the hub-update session invalidates.
 */
export const selfUpdateQueryKey = (hostId: string, host?: HostOwner) =>
  hostId === "0" ? ["settings", "self-update", "0"] : ["settings", "self-update", hostId, host?.identity ?? "", host?.agentVersion ?? ""];

export function useSelfUpdateCheck(hostId: string, { host, enabled }: { host?: HostOwner; enabled: boolean }) {
  const adapter = useAdapter(), client = useQueryClient(), queryKey = selfUpdateQueryKey(hostId, host);
  const query = useQuery({ queryKey, queryFn: () => adapter.getSelfUpdate(hostId), enabled,
    // A live check reaches the agent; repeat it only while an operation is unsettled.
    refetchInterval: (q) => q.state.data?.operation && q.state.data.operation.phase !== "settled" ? 10_000 : false });
  // An explicit recheck asks the hub for a fresh release list; the query state shows a failure.
  const recheck = () => client.fetchQuery({ queryKey, queryFn: () => adapter.getSelfUpdate(hostId, { refresh: true }), staleTime: 0 });
  return { query, recheck };
}
