// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original hub reachability classifier and gate retry backoff.


import { useEffect, useRef, useState } from "react";
import { isCancelledError } from "@tanstack/react-query";
import { AdapterOutcomeError } from "@/data/capability";

export function isHubUnreachable(error: unknown): boolean {
  if (isCancelledError(error) || !(error instanceof AdapterOutcomeError) || error.outcome.kind !== "error") return false;
  const { code, status } = error.outcome;
  return code === "http.error" && (status === undefined || [502, 503, 504].includes(status));
}
interface GateQuery { error: unknown; isFetching: boolean; refetch: () => Promise<unknown> }
export function useHubRetry(queries: GateQuery[]) {
  const unreachable = queries.some(query => isHubUnreachable(query.error));
  const latest = useRef(queries), pending = useRef(false), [retrying, setRetrying] = useState(false);
  useEffect(() => { latest.current = queries; });
  const retry = async () => {
    if (pending.current || latest.current.some(q => q.isFetching && isHubUnreachable(q.error))) return;
    pending.current = true; setRetrying(true);
    try { await Promise.allSettled(latest.current.filter(q => isHubUnreachable(q.error)).map(q => q.refetch())); }
    finally { pending.current = false; setRetrying(false); }
  };
  useEffect(() => {
    if (!unreachable) return;
    let active = true, attempt = 0, timer: ReturnType<typeof setTimeout>;
    const schedule = () => { timer = setTimeout(() => { void retry().finally(() => { if (active) schedule(); }); }, [5000, 10_000, 20_000, 30_000][Math.min(attempt++, 3)]); };
    schedule();
    return () => { active = false; clearTimeout(timer); };
    // Read the current query set on each retry, without restarting the backoff on a render.
  }, [unreachable]);
  return { unreachable, retry, retrying };
}
