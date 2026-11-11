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
/** The epoch at a fetch's last check before Query publishes its outcome. */
const checked = new WeakMap<object, number>();
const tails = new WeakSet<object>();

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
        const current = () => {
          const done = epoch === epochOf(query) || repeat >= MAX_SUPERSEDED_REPEATS || context.signal.aborted;
          if (done) checked.set(query, epochOf(query));
          return done;
        };
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

/** Whether a partial result of the attempt that called this is still current; superseded attempts publish nothing. */
export function attemptCurrent(client: QueryClient, queryKey: QueryKey): () => boolean {
  const query = client.getQueryCache().find({ queryKey, exact: true });
  if (!query) return () => true;
  const epoch = epochOf(query);
  return () => epoch === epochOf(query);
}

/**
 * A notification between a fetch's last check and Query's publication cannot repeat that fetch any more: once the
 * outcome is published, the query is refreshed once more.
 */
function tail(client: QueryClient, query: Query): void {
  if (tails.has(query)) return;
  tails.add(query);
  const off = client.getQueryCache().subscribe((event) => {
    if (event.query !== query || query.state.fetchStatus !== "idle") return;
    off(); tails.delete(query);
    if (checked.get(query) !== undefined && checked.get(query) !== epochOf(query)) {
      checked.set(query, epochOf(query));
      void client.invalidateQueries({ queryKey: query.queryKey, exact: true }, { cancelRefetch: false });
    }
  });
}

/** Refreshes the queries of one change notification: each once, without cancelling a read in flight. */
export function refresh(client: QueryClient, keys: readonly QueryKey[], exactKeys: readonly QueryKey[] = []): void {
  const queries = new Set<Query>();
  for (const queryKey of keys) for (const query of client.getQueryCache().findAll({ queryKey })) queries.add(query);
  for (const queryKey of exactKeys) for (const query of client.getQueryCache().findAll({ queryKey, exact: true })) queries.add(query);
  for (const query of queries) if (query.state.fetchStatus === "fetching") { epochs.set(query, epochOf(query) + 1); tail(client, query); }
  for (const query of queries) void client.invalidateQueries({ queryKey: query.queryKey, exact: true }, { cancelRefetch: false });
}
