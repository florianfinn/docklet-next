// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original per-host detail query for M37 #579.

import { CancelledError, useQuery } from "@tanstack/react-query";

import { hostDetailsEpoch } from "@/data/host-details-source";
import { freshFrom } from "@/data/refresh-owner";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { isHostAvailable } from "@/domain/hosts";
import type { Host, HostDetails } from "@/domain/model";

/** Details belong to one host identity and show only while the host is available; a cached read never outlives either. */
export function useHostDetails(host: Host): HostDetails | undefined {
  const adapter = useAdapter();
  const available = isHostAvailable(host);
  const query = useQuery({
    queryKey: [...queryKeys.hostDetails, host.id, host.identity ?? ""],
    // A hosts or settings change during a read supersedes it: the read repeats until one ran without a change, so a
    // superseded answer is never published and a fetch never ends without a current one. Each repeat needs a change
    // notification during the read, which this read does not cause; a burst of changes therefore ends the loop.
    queryFn: async ({ signal }) => {
      for (;;) {
        // Each read starts from the current state, so the refresh owner does not repeat it once more for changes before it.
        freshFrom(signal);
        const epoch = hostDetailsEpoch();
        const details = await adapter.getHostDetails!(host.id, signal);
        if (epoch === hostDetailsEpoch()) return details;
        if (signal.aborted) throw new CancelledError();
      }
    },
    enabled: available && !!adapter.getHostDetails,
    retry: false,
  });
  return available && query.isSuccess ? query.data : undefined;
}
