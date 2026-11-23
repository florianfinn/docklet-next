// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original per-host detail query for M37 #579; answers and failures stamped with their source epoch; failures retried at a fixed interval.

import { CancelledError, useQuery } from "@tanstack/react-query";
import { useEffect } from "react";

import { hostDetailsEpoch } from "@/data/host-details-source";
import { freshFrom } from "@/data/refresh-owner";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { isHostAvailable } from "@/domain/hosts";
import type { Host, HostDetails } from "@/domain/model";

type Stamped = { details: HostDetails; epoch: number };

/** A failed read carries the source epoch it ran under, so a failure overtaken by a change still gets a current read. */
class StampedFailure extends Error {
  readonly epoch: number;
  constructor(epoch: number, cause: unknown) { super("host-details.read-failed", { cause }); this.epoch = epoch; }
}

/** An agent that has just restarted can fail its first reads while the hub already reports it available. */
export const FAILED_READ_RETRY_MS = 15_000;

/** Answers seen as current when they were rendered; such a value may stay visible while its follow-up read runs. */
const proven = new WeakSet<Stamped>();

/**
 * Details belong to one host identity and show only while the host is available; a cached read never outlives either.
 * Every answer carries the source epoch it was read under. An answer that is no longer current when it is rendered is
 * hidden unless it was current before (then it stays until the follow-up arrives), and it always gets a follow-up read:
 * this covers a change that lands after the read's last check but before the query publishes it. A settled failure is
 * read again at a fixed interval while the card is open and the host available.
 */
export function useHostDetails(host: Host): HostDetails | undefined {
  const adapter = useAdapter();
  const available = isHostAvailable(host);
  const query = useQuery({
    queryKey: [...queryKeys.hostDetails, host.id, host.identity ?? ""],
    // A hosts or settings change during a read supersedes it: the read repeats until one ran without a change. Each repeat
    // needs a change notification during the read, which this read does not cause; a burst of changes ends the loop.
    queryFn: async ({ signal }): Promise<Stamped> => {
      for (;;) {
        // Each read starts from the current state, so the refresh owner does not repeat it once more for changes before it.
        freshFrom(signal);
        const epoch = hostDetailsEpoch();
        let details: HostDetails;
        try { details = await adapter.getHostDetails!(host.id, signal); }
        catch (error) {
          if (signal.aborted) throw new CancelledError();
          // A failure overtaken by a change says nothing about the current state: read again, like a superseded answer.
          if (epoch !== hostDetailsEpoch()) continue;
          throw new StampedFailure(epoch, error);
        }
        if (epoch === hostDetailsEpoch()) return { details, epoch };
        if (signal.aborted) throw new CancelledError();
      }
    },
    enabled: available && !!adapter.getHostDetails,
    retry: false,
    refetchInterval: (q) => q.state.status === "error" ? FAILED_READ_RETRY_MS : false,
  });
  const data = available && query.isSuccess ? query.data : undefined;
  const fresh = !!data && data.epoch === hostDetailsEpoch();
  if (data && fresh) proven.add(data);
  // A failure settled after a change (between the read's last check and publication) is as superseded as an answer.
  const failedBefore = available && query.error instanceof StampedFailure && query.error.epoch !== hostDetailsEpoch();
  const stale = (!!data && !fresh) || failedBefore;
  const { isFetching, refetch } = query;
  useEffect(() => { if (stale && !isFetching) void refetch(); }, [stale, isFetching, refetch]);
  return data && (fresh || proven.has(data)) ? data.details : undefined;
}
