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
/** Whether a query's running fetch started without loaded data; its own partial writes do not make it a refetch. */
const initial = new WeakMap<object, boolean>();
/** Per fetch signal: marks the attempt current from here on (see `freshFrom`). */
const boundaries = new WeakMap<AbortSignal, () => void>();

/**
 * Called by a reader when its attempt takes the state it reads from (the inventory's host catalogue): a change notified
 * before that point is already part of what the attempt reads, so it does not make the attempt superseded.
 */
export function freshFrom(signal: AbortSignal | undefined): void {
  if (signal) boundaries.get(signal)?.();
}

/**
 * A fetch whose query was superseded while it ran discards that outcome and runs the query function again, so its
 * physical read is never aborted by a change notification and a value read before the change is never published.
 */
export const refreshBehavior: QueryBehavior = {
  onFetch(context, query) {
    const original = context.fetchFn, first = query.state.data === undefined;
    initial.set(query, first);
    context.fetchFn = async () => {
      for (let repeat = 0; ; repeat++) {
        let epoch = epochOf(query);
        boundaries.set(context.signal, () => { epoch = epochOf(query); });
        const current = () => {
          const done = epoch === epochOf(query) || repeat >= MAX_SUPERSEDED_REPEATS || context.signal.aborted;
          if (done) checked.set(query, epochOf(query));
          return done;
        };
        try {
          const value = await original();
          // A first load joins a change as before; only a refetch of loaded data repeats.
          if (first || current()) return value;
        } catch (error) {
          // A fence of the read's own source (a version bump) counts as superseded, also for a first load;
          // cancelling this fetch does not.
          if (!isCancelledError(error) || current()) throw error;
        }
      }
    };
  },
};

/**
 * A notification between a fetch's last check and Query's publication cannot repeat that fetch any more: once that
 * fetch publishes its value or error, the query is refreshed once more. Cancellation, a reset or removal of the query
 * retires the tail without a read.
 */
function tail(client: QueryClient, query: Query): void {
  if (tails.has(query)) return;
  tails.add(query);
  const off = client.getQueryCache().subscribe((event) => {
    if (event.query !== query) return;
    if (event.type === "removed") { off(); tails.delete(query); return; }
    // A manual cache write (a partial publisher) is not this fetch's publication.
    if (event.type !== "updated" || ["fetch", "failed", "pause", "continue", "invalidate"].includes(event.action.type)
      || (event.action.type === "success" && event.action.manual)) return;
    off(); tails.delete(query);
    const published = event.action.type === "success" || (event.action.type === "error" && !isCancelledError(event.action.error));
    if (published && checked.get(query) !== epochOf(query)) {
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
  // Every running fetch learns of the change: a refetch of loaded data repeats (and gets a tail for a late change), a
  // first load repeats only when its source fenced it; its value joins the change as before.
  for (const query of queries) {
    if (query.state.fetchStatus !== "fetching") continue;
    epochs.set(query, epochOf(query) + 1);
    if (initial.get(query) === false) tail(client, query);
  }
  for (const query of queries) void client.invalidateQueries({ queryKey: query.queryKey, exact: true }, { cancelRefetch: false });
}
