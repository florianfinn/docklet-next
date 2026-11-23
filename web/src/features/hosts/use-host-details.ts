// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original per-host detail query for M37 #579.

import { CancelledError, useQuery } from "@tanstack/react-query";

import { hostDetailsEpoch } from "@/data/host-details-source";
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
    // A hosts or settings change during the read fences it; the refresh owner then reads again instead of publishing it.
    queryFn: async ({ signal }) => {
      const epoch = hostDetailsEpoch();
      const details = await adapter.getHostDetails!(host.id, signal);
      if (epoch !== hostDetailsEpoch()) throw new CancelledError({ silent: true });
      return details;
    },
    enabled: available && !!adapter.getHostDetails,
    retry: false,
  });
  return available && query.isSuccess ? query.data : undefined;
}
