// Source: docklet-next/issues/438
// Revision: cc93f43359147e8e128b2ac00bcd410c055eb423
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original candidate proof consumer and verified update integration. M21 R1-S12W B1: no topic re-reads.

import { useEffect, useReducer, useRef, useState, useSyncExternalStore } from "react";
import { isCancelledError } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import type { CandidateResults } from "@/data/vulnerability-candidates";

/** Advisory, host and signed-in identity own every candidate read and activity follow. */
export function useCandidates(host: string, advisory: string) {
  const adapter = useAdapter(), lifetime = identityLifetime(adapter);
  const identity = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version), active = lifetime.active();
  const owner = JSON.stringify([host, advisory, identity, active]);
  const [revision, reload] = useReducer((v: number) => v + 1, 0);
  const [state, setState] = useState<{ owner: string; data?: CandidateResults; error?: unknown; revision: number }>();
  const [activity, setActivity] = useState<{ owner: string; id: string }>();
  const generation = useRef(0);
  // Candidate reads observe the registry, so stream topics never re-read; reads
  // follow opening, finished runs, explicit gestures and proof expiry only.
  useEffect(() => {
    const sequence = ++generation.current, controller = new AbortController();
    if (active && advisory) void adapter.getVulnerabilityCandidates(host, { advisory }, controller.signal).then(
      (data) => { if (sequence === generation.current && !controller.signal.aborted && lifetime.version() === identity && lifetime.active()) setState({ owner, data, revision }); },
      (error: unknown) => { if (sequence === generation.current && !controller.signal.aborted && !isCancelledError(error)) setState((old) => ({ owner, data: old?.owner === owner ? old.data : undefined, error, revision })); },
    );
    return () => { controller.abort(); };
  }, [adapter, host, advisory, owner, identity, lifetime, active, revision]);
  const current = active && state?.owner === owner ? state : undefined;
  const activityId = active && activity?.owner === owner ? activity.id : undefined;
  const following = JSON.stringify([...new Set([...(current?.data?.runs.filter((r) => r.status === "running").map((r) => r.activityId) ?? []), ...(activityId ? [activityId] : [])])]);
  useEffect(() => {
    let live = true;
    const finished = new Set<string>();
    const stops = (JSON.parse(following) as string[]).map((id) => adapter.observeActionDetail?.(host, id, (detail) => {
      if (!live || lifetime.version() !== identity || !lifetime.active() || detail.status !== "ready" || ["running", "queued"].includes(detail.run.status) || finished.has(id)) return;
      finished.add(id); reload();
      setActivity((old) => old?.id === id ? undefined : old);
    }));
    return () => { live = false; stops.forEach((stop) => stop?.()); };
  }, [adapter, following, host, identity, lifetime]);
  // Expiry also invalidates a proof while the detail remains open without events.
  const expires = Math.min(...(current?.data?.results.flatMap((r) => r.proof ? [Date.parse(r.proof.completedAt) + 86400000] : []) ?? []));
  useEffect(() => {
    if (!Number.isFinite(expires) || expires <= Date.now()) return;
    const timer = setTimeout(reload, expires - Date.now() + 1);
    return () => clearTimeout(timer);
  }, [expires]);
  return { refreshing: current?.revision !== revision, data: current?.data, error: current?.error, reload, activityId, follow: (id: string) => { setActivity({ owner, id }); reload(); }, active, identity };
}
