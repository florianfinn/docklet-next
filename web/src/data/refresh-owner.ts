// Source: docklet-next/issues/391
// Revision: a331ec9df0c18670fd14d06d043cf58a4ef0fc80
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original change-driven refresh owner: superseded fetches repeat instead of being cancelled (#391, #389).

import { isCancelledError, type Query, type QueryClient, type QueryKey, type QueryOptions } from "@tanstack/react-query";

type QueryBehavior = NonNullable<QueryOptions["behavior"]>;

/** Repeats of one fetch after changes arrived during it; the last outcome stands. */
export const MAX_SUPERSEDED_REPEATS = 3;

const epochs = new WeakMap<object, number>();
const epochOf = (query: object) => epochs.get(query) ?? 0;

/**
 * A fetch whose query was superseded while it ran discards that outcome and runs the query function again, so its
 * physical read is never aborted by a change notification and a value read before the change is never published.
 */
export const refreshBehavior: QueryBehavior = {
  onFetch(context, query) {
    const original = context.fetchFn;
    context.fetchFn = async () => {
      for (let repeat = 0; ; repeat++) {
        const epoch = epochOf(query);
        const current = () => epoch === epochOf(query) || repeat >= MAX_SUPERSEDED_REPEATS || context.signal.aborted;
        try {
          const value = await original();
          if (current()) return value;
        } catch (error) {
          // A fence of the read's own source (a version bump) counts as superseded; cancelling this fetch does not.
          if (!isCancelledError(error) || current()) throw error;
        }
      }
    };
  },
};

/** Refreshes the queries of one change notification: each once, without cancelling a read in flight. */
export function refresh(client: QueryClient, keys: readonly QueryKey[], exactKeys: readonly QueryKey[] = []): void {
  const queries = new Set<Query>();
  for (const queryKey of keys) for (const query of client.getQueryCache().findAll({ queryKey })) queries.add(query);
  for (const queryKey of exactKeys) for (const query of client.getQueryCache().findAll({ queryKey, exact: true })) queries.add(query);
  for (const query of queries) if (query.state.fetchStatus === "fetching") epochs.set(query, epochOf(query) + 1);
  for (const query of queries) void client.invalidateQueries({ queryKey: query.queryKey, exact: true }, { cancelRefetch: false });
}
