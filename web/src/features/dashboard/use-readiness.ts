// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original Dashboard readiness markers for load measurement (hooks).

import { createContext, useCallback, useContext, useEffect, useState } from "react";

export type ReadyReport = (id: string) => void;
export const ReadyContext = createContext<ReadyReport | null>(null);

/** A query's first complete answer or its error; an inventory seed keeps the query fetching, so it does not count. */
export function firstResult(query: { isError: boolean; isSuccess: boolean; fetchStatus: string }): boolean {
  return query.isError || (query.isSuccess && query.fetchStatus === "idle");
}

/** A widget reports once that its first complete result or error is shown. */
export function useWidgetReady(id: string, ready: boolean): void {
  const report = useContext(ReadyContext);
  useEffect(() => { if (ready) report?.(id); }, [report, id, ready]);
}

/**
 * `core` once hosts, stacks and containers have their first complete results; `settled` once the shown widgets are
 * known and every one reported too. Both stay set for this page visit (later refetches do not clear them).
 */
export function useDashboardReadiness(visible: readonly string[], core: boolean, known = true) {
  const [reported, setReported] = useState<ReadonlySet<string>>(() => new Set());
  const [coreReady, setCoreReady] = useState(false);
  const [settled, setSettled] = useState(false);
  const report = useCallback<ReadyReport>((id) => setReported((old) => (old.has(id) ? old : new Set(old).add(id))), []);
  // Sticky flags derived while rendering.
  if (core && !coreReady) setCoreReady(true);
  // `known`: the set of shown widgets is final (service summaries answered).
  const all = (coreReady || core) && known && visible.every((id) => reported.has(id));
  if (all && !settled) setSettled(true);
  return { coreReady: coreReady || core, settled: settled || all, report };
}
